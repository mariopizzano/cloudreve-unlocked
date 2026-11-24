package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/task"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/dbfs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/queue"
	"github.com/samber/lo"
	"golang.org/x/tools/container/intsets"
)

type (
	RelocateTask struct {
		*queue.DBTask

		l        logging.Logger
		fm       manager.FileManager
		state    *RelocateTaskState
		progress queue.Progresses
	}

	RelocateTaskState struct {
		Src          []string          `json:"src"`
		DstPolicyID  int               `json:"dst_policy_id"`
		Phase        string            `json:"phase"`
		Total        int               `json:"total"`
		Done         int               `json:"done"`
		Failed       int               `json:"failed"`
		Skipped      int               `json:"skipped"`
		Unsupported  int               `json:"unsupported"`
		Versions     int               `json:"versions"`
		Thumbnails   int               `json:"thumbnails"`
		Shared       int               `json:"shared"`
		DeleteSource bool              `json:"delete_source"`
		Failures     []RelocateFailure `json:"failures,omitempty"`
	}

	// RelocateFailure is a single entity-level failure kept for the task audit.
	RelocateFailure struct {
		File   int    `json:"file"`
		Entity int    `json:"entity"`
		Name   string `json:"name"`
		Error  string `json:"error"`
	}

	// RelocatePlan is a read-only preview of a relocation operation.
	RelocatePlan struct {
		Files       int   `json:"files"`
		Folders     int   `json:"folders"`
		Entities    int   `json:"entities"`
		Versions    int   `json:"versions"`
		Thumbnails  int   `json:"thumbnails"`
		Shared      int   `json:"shared"`
		Skipped     int   `json:"skipped"`
		Unsupported int   `json:"unsupported"`
		Encrypted   int   `json:"encrypted"`
		Size        int64 `json:"size"`
	}
)

const (
	RelocatePhaseIndex    = "index"
	RelocatePhaseTransfer = "transfer"
	RelocatePhaseFinish   = "finish"

	SummaryKeyRelocateEntities   = "entities"
	SummaryKeyRelocateDone       = "done"
	SummaryKeyRelocateVersions   = "versions"
	SummaryKeyRelocateThumbnails = "thumbnails"
	SummaryKeyRelocateShared     = "shared"
	SummaryKeyRelocateSkipped    = "skipped"
	SummaryKeyRelocateFailures   = "failures"
)

func init() {
	queue.RegisterResumableTaskFactory(queue.RelocateTaskType, NewRelocateTaskFromModel)
}

func NewRelocateTask(ctx context.Context, u *ent.User, src []string, dstPolicyID int, deleteSource bool) (queue.Task, error) {
	state := &RelocateTaskState{
		Src:          src,
		DstPolicyID:  dstPolicyID,
		Phase:        RelocatePhaseIndex,
		DeleteSource: deleteSource,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &RelocateTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:          queue.RelocateTaskType,
				CorrelationID: logging.CorrelationID(ctx),
				PrivateState:  string(stateBytes),
				PublicState:   &types.TaskPublicState{},
			},
			DirectOwner: u,
		},
	}, nil
}

func NewRelocateTaskFromModel(t *ent.Task) queue.Task {
	return &RelocateTask{
		DBTask: &queue.DBTask{
			Task: t,
		},
	}
}

// PlanRelocate counts what would be moved without touching any data.
func PlanRelocate(ctx context.Context, dep dependency.Dep, user *ent.User, src []string, dstPolicyID int) (*RelocatePlan, error) {
	fm := manager.NewFileManager(dep, user)
	defer fm.Recycle()

	plan := &RelocatePlan{}
	if err := walkRelocateSources(ctx, fm, src, func(file fs.File) error {
		if file.IsSymbolic() {
			return nil
		}

		if file.Type() == types.FileTypeFolder {
			plan.Folders++
			return nil
		}

		if file.Type() != types.FileTypeFile {
			return nil
		}

		primary := file.PrimaryEntity()
		primaryID := 0
		if primary != nil {
			primaryID = primary.ID()
		}

		movable := 0
		for _, entity := range file.Entities() {
			// Placeholders and already-migrated entities are not relocated.
			if entity.Size() == 0 || entity.PolicyID() == dstPolicyID {
				continue
			}

			movable++
			plan.Entities++
			plan.Size += entity.Size()
			if entity.Encrypted() {
				plan.Encrypted++
			}
			if entity.ReferenceCount() > 1 {
				plan.Shared++
			}
			if entity.Type() == types.EntityTypeThumbnail {
				plan.Thumbnails++
			} else if entity.ID() != primaryID {
				plan.Versions++
			}
		}

		if movable > 0 {
			plan.Files++
		} else {
			plan.Skipped++
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return plan, nil
}

func walkRelocateSources(ctx context.Context, fm manager.FileManager, src []string, fn func(fs.File) error) error {
	for _, raw := range src {
		uri, err := fs.NewUriFromString(raw)
		if err != nil {
			return fmt.Errorf("invalid source %q: %w", raw, err)
		}

		file, err := fm.Get(ctx, uri,
			dbfs.WithFileEntities(),
			dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile),
		)
		if err != nil {
			return fmt.Errorf("failed to get source %q: %w", raw, err)
		}

		if file.Type() != types.FileTypeFolder {
			if err := fn(file); err != nil {
				return err
			}
			continue
		}

		if err := fm.Walk(ctx, uri, intsets.MaxInt, func(walked fs.File, level int) error {
			return fn(walked)
		}, dbfs.WithFileEntities()); err != nil {
			return fmt.Errorf("failed to walk %q: %w", raw, err)
		}
	}

	return nil
}

func (m *RelocateTask) Do(ctx context.Context) (task.Status, error) {
	dep := dependency.FromContext(ctx)
	m.l = dep.Logger()
	user := inventory.UserFromContext(ctx)
	m.fm = manager.NewFileManager(dep, user)
	defer m.fm.Recycle()

	m.Lock()
	if m.progress == nil {
		m.progress = make(queue.Progresses)
	}
	m.progress[ProgressTypeRelocateTransferCount] = &queue.Progress{}
	m.Unlock()

	state := &RelocateTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	m.state = state

	var (
		next task.Status
		err  error
	)
	switch state.Phase {
	case "", RelocatePhaseIndex:
		next, err = m.index(ctx, dep, user)
	case RelocatePhaseTransfer:
		next, err = m.transfer(ctx, dep)
	case RelocatePhaseFinish:
		next, err = task.StatusCompleted, nil
	default:
		next, err = task.StatusError, fmt.Errorf("unknown relocate phase %q: %w", state.Phase, queue.CriticalErr)
	}

	if m.state != nil {
		stateBytes, marshalErr := json.Marshal(m.state)
		if marshalErr != nil {
			return task.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
		}
		m.Lock()
		m.Task.PrivateState = string(stateBytes)
		m.Unlock()
	}

	return next, err
}

func (m *RelocateTask) index(ctx context.Context, dep dependency.Dep, user *ent.User) (task.Status, error) {
	plan, err := PlanRelocate(ctx, dep, user, m.state.Src, m.state.DstPolicyID)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to build relocation plan: %w", err)
	}

	m.state.Total = plan.Entities
	m.state.Skipped = plan.Skipped
	m.state.Unsupported = plan.Unsupported
	m.state.Versions = plan.Versions
	m.state.Thumbnails = plan.Thumbnails
	m.state.Shared = plan.Shared
	m.state.Phase = RelocatePhaseTransfer
	m.state.Done = 0
	m.state.Failed = 0

	m.Lock()
	m.progress[ProgressTypeRelocateTransferCount].Total = int64(plan.Entities)
	m.Unlock()

	m.l.Info("Relocation plan: %d files, %d entities (%d versions, %d thumbnails, %d shared), %d bytes, %d skipped, %d unsupported",
		plan.Files, plan.Entities, plan.Versions, plan.Thumbnails, plan.Shared, plan.Size, plan.Skipped, plan.Unsupported)
	m.ResumeAfter(0)
	return task.StatusSuspending, nil
}

func (m *RelocateTask) transfer(ctx context.Context, dep dependency.Dep) (task.Status, error) {
	dstPolicy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, m.state.DstPolicyID)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get destination policy: %w", err)
	}

	user := inventory.UserFromContext(ctx)
	failed := 0
	done := 0
	walkErr := walkRelocateSources(ctx, m.fm, m.state.Src, func(file fs.File) error {
		if file.IsSymbolic() || file.Type() != types.FileTypeFile {
			return nil
		}

		primaryID := file.PrimaryEntityID()
		for _, entity := range file.Entities() {
			if entity.Size() == 0 || entity.PolicyID() == dstPolicy.ID {
				continue
			}

			if err := m.relocateEntity(ctx, dep, user, file, entity, dstPolicy, entity.ID() == primaryID); err != nil {
				failed++
				m.l.Warning("Failed to relocate entity %d of file %d (%s): %s", entity.ID(), file.ID(), file.Name(), err)
				if len(m.state.Failures) < 20 {
					m.state.Failures = append(m.state.Failures, RelocateFailure{
						File:   file.ID(),
						Entity: entity.ID(),
						Name:   file.Name(),
						Error:  err.Error(),
					})
				}
				continue
			}

			done++
			atomic.AddInt64(&m.progress[ProgressTypeRelocateTransferCount].Current, 1)
		}

		return nil
	})

	m.state.Done = done
	m.state.Failed = failed
	m.state.Phase = RelocatePhaseFinish

	if walkErr != nil {
		return task.StatusError, fmt.Errorf("failed to walk relocation sources: %w", walkErr)
	}

	m.l.Info("Relocation finished: %d entities moved, %d failed, %d skipped files, %d unsupported",
		done, failed, m.state.Skipped, m.state.Unsupported)
	return task.StatusCompleted, nil
}

func (m *RelocateTask) relocateEntity(ctx context.Context, dep dependency.Dep, user *ent.User, file fs.File, entity fs.Entity, dstPolicy *ent.StoragePolicy, isPrimary bool) error {
	source, err := m.fm.GetEntitySource(ctx, entity.ID())
	if err != nil {
		return fmt.Errorf("failed to open source entity: %w", err)
	}

	var (
		reader     io.ReadCloser = source
		seeker     io.Seeker     = source
		storedMeta *types.EncryptMetadata
	)

	if dstPolicy.Settings != nil && dstPolicy.Settings.Encryption {
		cryptor, err := dep.EncryptorFactory(ctx)(types.CipherAES256CTR)
		if err != nil {
			_ = source.Close()
			return fmt.Errorf("failed to create encryptor: %w", err)
		}

		meta, err := cryptor.GenerateMetadata(ctx)
		if err != nil {
			_ = source.Close()
			return fmt.Errorf("failed to generate encryption metadata: %w", err)
		}

		if err := cryptor.LoadMetadata(ctx, meta); err != nil {
			_ = source.Close()
			return fmt.Errorf("failed to load encryption metadata: %w", err)
		}

		if err := cryptor.SetSource(source, source, entity.Size(), 0); err != nil {
			_ = source.Close()
			return fmt.Errorf("failed to initialize encryptor source: %w", err)
		}

		reader = cryptor
		seeker = cryptor
		storedMeta = &types.EncryptMetadata{
			Algorithm: meta.Algorithm,
			Key:       meta.Key,
			IV:        meta.IV,
		}
	}

	mimeType := dep.MimeDetector(ctx).TypeByName(file.Name())
	if entity.Type() == types.EntityTypeThumbnail {
		mimeType = "image/jpeg"
	}

	savePath := dbfs.GenerateSavePathForPolicy(dstPolicy, file.Uri(false), user)
	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:      file.Uri(false),
			Size:     entity.Size(),
			SavePath: savePath,
			MimeType: mimeType,
		},
		Mode:   fs.ModeNone,
		File:   reader,
		Seeker: seeker,
	}

	driver, err := m.fm.GetStorageDriver(ctx, dstPolicy)
	if err != nil {
		_ = reader.Close()
		return fmt.Errorf("failed to get destination driver: %w", err)
	}

	if err := driver.Put(ctx, req); err != nil {
		return fmt.Errorf("failed to write destination object: %w", err)
	}

	if _, err := dep.FileClient().RelocateEntity(ctx, file.ID(), entity.ID(), inventory.RelocateEntityParameters{
		NewSource:       savePath,
		StoragePolicyID: dstPolicy.ID,
		EncryptMetadata: storedMeta,
		DeleteSource:    m.state.DeleteSource,
	}); err != nil {
		// The physical copy is orphaned; remove it best-effort.
		if _, delErr := driver.Delete(ctx, savePath); delErr != nil {
			m.l.Warning("Failed to clean up orphaned relocated object %q: %s", savePath, delErr)
		}
		return fmt.Errorf("failed to commit relocated entity: %w", err)
	}

	if !isPrimary {
		m.l.Info("Relocated %s entity %d of file %d to policy %d", entity.Type(), entity.ID(), file.ID(), dstPolicy.ID)
	}

	oldPolicy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, entity.PolicyID())
	if err != nil {
		m.l.Warning("Failed to load old policy %d for cleanup: %s", entity.PolicyID(), err)
		return nil
	}

	if !m.state.DeleteSource {
		m.l.Info("Relocation kept source object %q on policy %d (delete_source=false)", entity.Source(), entity.PolicyID())
		return nil
	}

	if entity.ReferenceCount() > 1 {
		// Shared blob: other files still use it, never delete the source.
		m.l.Info("Relocation kept shared source object %q (reference count %d)", entity.Source(), entity.ReferenceCount())
		return nil
	}

	oldDriver, err := m.fm.GetStorageDriver(ctx, oldPolicy)
	if err != nil {
		m.l.Warning("Failed to get old storage driver for cleanup: %s", err)
		return nil
	}

	if _, err := oldDriver.Delete(ctx, entity.Source()); err != nil {
		m.l.Warning("Failed to delete old object %q: %s", entity.Source(), err)
	}

	return nil
}

func (m *RelocateTask) Progress(ctx context.Context) queue.Progresses {
	m.Lock()
	defer m.Unlock()
	return m.progress
}

func (m *RelocateTask) Summarize(hasher hashid.Encoder) *queue.Summary {
	if m.state == nil {
		if err := json.Unmarshal([]byte(m.State()), &m.state); err != nil {
			return nil
		}
	}

	return &queue.Summary{
		Phase: m.state.Phase,
		Props: map[string]any{
			SummaryKeySrcMultiple:        lo.Ternary(len(m.state.Src) > 0, m.state.Src, nil),
			SummaryKeySrcDstPolicyID:     hashid.EncodePolicyID(hasher, m.state.DstPolicyID),
			SummaryKeyFailed:             m.state.Failed,
			SummaryKeyTotal:              m.state.Total,
			SummaryKeyRelocateEntities:   m.state.Total,
			SummaryKeyRelocateDone:       m.state.Done,
			SummaryKeyRelocateVersions:   m.state.Versions,
			SummaryKeyRelocateThumbnails: m.state.Thumbnails,
			SummaryKeyRelocateShared:     m.state.Shared,
			SummaryKeyRelocateSkipped:    m.state.Skipped,
			SummaryKeyRelocateFailures:   lo.Ternary(len(m.state.Failures) > 0, m.state.Failures, nil),
		},
	}
}
