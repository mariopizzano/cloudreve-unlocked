package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/entity"
	"github.com/cloudreve/Cloudreve/v4/ent/retainedobject"
	"github.com/cloudreve/Cloudreve/v4/ent/schema"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/crontab"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/samber/lo"
)

const (
	// RetainedObjectCleanupLastRunKey is the KV key holding the summary of the
	// last retained object cleanup run.
	RetainedObjectCleanupLastRunKey = "retained_object_cleanup_last_run"

	defaultRetainedObjectBatchSize = 100
	// maxRetainedObjectCleanupRounds bounds the number of batches processed in
	// a single run, so a huge backlog cannot keep the cron job alive forever.
	maxRetainedObjectCleanupRounds = 100
)

type (
	// RetainedCleanupOptions controls a retained object cleanup run.
	RetainedCleanupOptions struct {
		// Retention is the minimum age an object must have before deletion.
		Retention time.Duration
		// BatchSize is the number of records fetched per query.
		BatchSize int
		// Force ignores the retention window and processes every pending
		// object. The live-reference safety check is always applied.
		Force bool
	}

	// RetainedCleanupResult summarizes a cleanup run.
	RetainedCleanupResult struct {
		StartedAt      time.Time `json:"started_at"`
		FinishedAt     time.Time `json:"finished_at"`
		Candidates     int       `json:"candidates"`
		Deleted        int       `json:"deleted"`
		Skipped        int       `json:"skipped"`
		Failed         int       `json:"failed"`
		BytesReclaimed int64     `json:"bytes_reclaimed"`
	}
)

func init() {
	crontab.Register(setting.CronTypeRetainedObjectCleanup, CronCleanupRetainedObjects)
}

// RetainedObjectExpiryCutoff returns the oldest creation time an object may
// have to be eligible for cleanup. A non-positive retention means "expire now".
func RetainedObjectExpiryCutoff(now time.Time, retention time.Duration) time.Time {
	if retention <= 0 {
		return now
	}

	return now.Add(-retention)
}

// IsRetainedObjectExpired reports whether an object created at the given time
// is old enough to be cleaned up.
func IsRetainedObjectExpired(createdAt, now time.Time, retention time.Duration) bool {
	return !createdAt.After(RetainedObjectExpiryCutoff(now, retention))
}

// CronCleanupRetainedObjects is the daily cron entry point for retained object
// cleanup.
func CronCleanupRetainedObjects(ctx context.Context) {
	dep := dependency.FromContext(ctx)
	l := dep.Logger()

	cfg := dep.SettingProvider().RetainedObjectSetting(ctx)
	if !cfg.Enabled {
		l.Info("Retained object cleanup is disabled, skipping")
		return
	}

	options := &RetainedCleanupOptions{
		Retention: cfg.Retention,
		BatchSize: cfg.BatchSize,
	}
	res, err := CleanupRetainedObjects(ctx, dep, options)
	if err != nil {
		l.Error("Retained object cleanup failed: %s", err)
		return
	}

	if err := SaveLastRetainedObjectCleanup(ctx, dep, res); err != nil {
		l.Warning("Failed to persist retained object cleanup summary: %s", err)
	}

	l.Info("Retained object cleanup finished: %d candidates, %d deleted, %d skipped, %d failed, %d bytes reclaimed",
		res.Candidates, res.Deleted, res.Skipped, res.Failed, res.BytesReclaimed)
}

// CleanupRetainedObjects removes expired retained physical objects. Deletion
// is idempotent: objects that no longer exist are treated as deleted, and a
// failure on a single object does not stop the rest of the batch.
func CleanupRetainedObjects(ctx context.Context, dep dependency.Dep, options *RetainedCleanupOptions) (*RetainedCleanupResult, error) {
	if options == nil {
		options = &RetainedCleanupOptions{}
	}

	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultRetainedObjectBatchSize
	}

	now := time.Now()
	cutoff := RetainedObjectExpiryCutoff(now, options.Retention)
	if options.Force {
		cutoff = now
	}

	db := dep.DBClient()
	fm := NewFileManager(dep, inventory.UserFromContext(ctx))
	l := dep.Logger()
	result := &RetainedCleanupResult{StartedAt: now}
	handledIDs := make([]int, 0, batchSize)

	for round := 0; round < maxRetainedObjectCleanupRounds; round++ {
		query := db.RetainedObject.Query().Where(
			retainedobject.StateEQ(inventory.RetainedObjectStatePending),
			retainedobject.CreatedAtLTE(cutoff),
		)
		if len(handledIDs) > 0 {
			query = query.Where(retainedobject.IDNotIn(handledIDs...))
		}

		records, err := query.
			Order(ent.Asc(retainedobject.FieldCreatedAt)).
			Limit(batchSize).
			All(ctx)
		if err != nil {
			result.FinishedAt = time.Now()
			return result, fmt.Errorf("failed to query retained objects: %w", err)
		}
		if len(records) == 0 {
			break
		}

		for _, record := range records {
			handledIDs = append(handledIDs, record.ID)
		}
		result.Candidates += len(records)

		processRetainedObjectBatch(ctx, dep, db, fm, records, result, l)
		if len(records) < batchSize {
			break
		}
	}

	result.FinishedAt = time.Now()
	return result, nil
}

func processRetainedObjectBatch(
	ctx context.Context,
	dep dependency.Dep,
	db *ent.Client,
	fm FileManager,
	records []*ent.RetainedObject,
	result *RetainedCleanupResult,
	l logging.Logger,
) {
	groups := lo.GroupBy(records, func(record *ent.RetainedObject) int {
		return record.StoragePolicyID
	})

	for policyID, group := range groups {
		policy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, policyID)
		if err != nil {
			result.Failed += len(group)
			l.Warning("Failed to load storage policy %d for retained object cleanup: %s", policyID, err)
			for _, record := range group {
				markRetainedObjectFailed(ctx, db, record, fmt.Errorf("failed to load storage policy %d: %w", policyID, err))
			}
			continue
		}

		driver, err := fm.GetStorageDriver(ctx, policy)
		if err != nil {
			result.Failed += len(group)
			l.Warning("Failed to get storage driver for policy %d: %s", policyID, err)
			for _, record := range group {
				markRetainedObjectFailed(ctx, db, record, fmt.Errorf("failed to get storage driver: %w", err))
			}
			continue
		}

		for _, record := range group {
			referenced, err := isRetainedObjectStillReferenced(ctx, db, record)
			if err != nil {
				result.Failed++
				markRetainedObjectFailed(ctx, db, record, fmt.Errorf("failed to check live references: %w", err))
				l.Warning("Failed to check live references for retained object %d: %s", record.ID, err)
				continue
			}
			if referenced {
				// A live entity still points to the same physical object on the
				// same policy. Deleting it would corrupt that file, so never do
				// it. The entity collector owns the object from now on.
				result.Skipped++
				markRetainedObjectSkipped(ctx, db, record)
				l.Info("Retained object %d (%q, policy %d) is still referenced by a live entity, skipping",
					record.ID, record.Source, record.StoragePolicyID)
				continue
			}

			failed, deleteErr := driver.Delete(ctx, record.Source)
			if deleteErr != nil || len(failed) > 0 {
				result.Failed++
				if deleteErr == nil {
					deleteErr = fmt.Errorf("object %q was not deleted", record.Source)
				}
				markRetainedObjectFailed(ctx, db, record, deleteErr)
				l.Warning("Failed to delete retained object %d (%q, policy %d): %s",
					record.ID, record.Source, record.StoragePolicyID, deleteErr)
				continue
			}

			// Physical object is gone; drop the tracking row. If this step fails
			// the row stays pending and will be retried, which is safe because
			// deletion is idempotent.
			if err := db.RetainedObject.DeleteOneID(record.ID).Exec(schema.SkipSoftDelete(ctx)); err != nil {
				result.Failed++
				markRetainedObjectFailed(ctx, db, record, fmt.Errorf("failed to remove retained object record: %w", err))
				l.Warning("Failed to remove retained object record %d after physical deletion: %s", record.ID, err)
				continue
			}

			result.Deleted++
			result.BytesReclaimed += record.Size
		}
	}
}

// isRetainedObjectStillReferenced reports whether any live (not soft-deleted)
// entity still points to the same physical object on the same policy. In that
// case the object must never be deleted by the retention cleanup.
func isRetainedObjectStillReferenced(ctx context.Context, db *ent.Client, record *ent.RetainedObject) (bool, error) {
	return db.Entity.Query().Where(
		entity.StoragePolicyEntities(record.StoragePolicyID),
		entity.SourceEQ(record.Source),
	).Exist(ctx)
}

func markRetainedObjectFailed(ctx context.Context, db *ent.Client, record *ent.RetainedObject, cause error) {
	// Best effort: the record stays pending for the next run anyway.
	_ = db.RetainedObject.UpdateOneID(record.ID).
		SetAttempts(record.Attempts + 1).
		SetLastError(cause.Error()).
		Exec(ctx)
}

func markRetainedObjectSkipped(ctx context.Context, db *ent.Client, record *ent.RetainedObject) {
	_ = db.RetainedObject.UpdateOneID(record.ID).
		SetState(inventory.RetainedObjectStateSkipped).
		SetLastError("still referenced by a live entity").
		Exec(ctx)
}

// SaveLastRetainedObjectCleanup persists a cleanup summary in the KV store.
func SaveLastRetainedObjectCleanup(ctx context.Context, dep dependency.Dep, result *RetainedCleanupResult) error {
	bytes, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return dep.KV().Set(RetainedObjectCleanupLastRunKey, string(bytes), 0)
}

// LastRetainedObjectCleanup loads the summary of the last cleanup run, if any.
func LastRetainedObjectCleanup(ctx context.Context, dep dependency.Dep) *RetainedCleanupResult {
	value, ok := dep.KV().Get(RetainedObjectCleanupLastRunKey)
	if !ok {
		return nil
	}

	raw, ok := value.(string)
	if !ok {
		return nil
	}

	result := &RetainedCleanupResult{}
	if err := json.Unmarshal([]byte(raw), result); err != nil {
		return nil
	}

	return result
}
