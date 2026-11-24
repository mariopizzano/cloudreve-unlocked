package admin

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/retainedobject"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

const defaultRetainedObjectListLimit = 100

type (
	// RetainedObjectService lists retained objects and their cleanup stats.
	RetainedObjectService struct {
		Limit int `form:"limit" binding:"omitempty,min=1,max=500"`
	}
	RetainedObjectParamCtx struct{}

	// RetainedObjectStats is the admin view of the retained object queue.
	RetainedObjectStats struct {
		Pending     int                            `json:"pending"`
		Skipped     int                            `json:"skipped"`
		PendingSize int64                          `json:"pending_size"`
		LastRun     *manager.RetainedCleanupResult `json:"last_run,omitempty"`
		Objects     []*ent.RetainedObject          `json:"objects"`
	}

	// ForceRetainedObjectCleanupService triggers an immediate cleanup run.
	ForceRetainedObjectCleanupService struct {
		BatchSize int `form:"batch_size" binding:"omitempty,min=1,max=1000"`
	}
	ForceRetainedObjectCleanupParamCtx struct{}
)

func (s *RetainedObjectService) Get(c *gin.Context) (*RetainedObjectStats, error) {
	dep := dependency.FromContext(c)
	db := dep.DBClient()

	pending, err := db.RetainedObject.Query().
		Where(retainedobject.StateEQ(inventory.RetainedObjectStatePending)).
		Count(c)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to count pending retained objects", err)
	}

	skipped, err := db.RetainedObject.Query().
		Where(retainedobject.StateEQ(inventory.RetainedObjectStateSkipped)).
		Count(c)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to count skipped retained objects", err)
	}

	var pendingSize int64
	var sums []struct {
		Sum int64 `json:"sum"`
	}
	if err := db.RetainedObject.Query().
		Where(retainedobject.StateEQ(inventory.RetainedObjectStatePending)).
		Aggregate(ent.Sum(retainedobject.FieldSize)).
		Scan(c, &sums); err == nil && len(sums) > 0 {
		pendingSize = sums[0].Sum
	}

	limit := s.Limit
	if limit <= 0 {
		limit = defaultRetainedObjectListLimit
	}

	objects, err := db.RetainedObject.Query().
		Where(retainedobject.StateEQ(inventory.RetainedObjectStatePending)).
		Order(ent.Asc(retainedobject.FieldCreatedAt)).
		Limit(limit).
		All(c)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list retained objects", err)
	}

	return &RetainedObjectStats{
		Pending:     pending,
		Skipped:     skipped,
		PendingSize: pendingSize,
		LastRun:     manager.LastRetainedObjectCleanup(c, dep),
		Objects:     objects,
	}, nil
}

// Cleanup forces a cleanup run, ignoring the retention window. The live
// reference safety check still applies.
func (s *ForceRetainedObjectCleanupService) Cleanup(c *gin.Context) (*manager.RetainedCleanupResult, error) {
	dep := dependency.FromContext(c)
	result, err := manager.CleanupRetainedObjects(c, dep, &manager.RetainedCleanupOptions{
		BatchSize: s.BatchSize,
		Force:     true,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to clean up retained objects", err)
	}

	if err := manager.SaveLastRetainedObjectCleanup(c, dep, result); err != nil {
		dep.Logger().Warning("Failed to persist retained object cleanup summary: %s", err)
	}

	return result, nil
}
