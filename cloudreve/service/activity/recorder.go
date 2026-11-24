package activity

import (
	"context"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/gin-gonic/gin"
)

// Action identifiers stored in the Activity entity. Keep them stable: they are
// part of the admin/user API contract and of the frontend filters.
const (
	ActionLoginSuccess = "login_success"
	ActionLoginFailed  = "login_failed"

	ActionFileCreate   = "file_create"
	ActionFileUpload   = "file_upload"
	ActionFileUpdate   = "file_update"
	ActionFileRename   = "file_rename"
	ActionFileMove     = "file_move"
	ActionFileCopy     = "file_copy"
	ActionFileDelete   = "file_delete"
	ActionFileRestore  = "file_restore"
	ActionFileDownload = "file_download"

	ActionShareCreate = "share_create"
	ActionShareDelete = "share_delete"
)

// Target categories used to group activity entries in the UI.
const (
	TargetTypeFile   = "file"
	TargetTypeShare  = "share"
	TargetTypeUser   = "user"
	TargetTypeSystem = "system"
)

// Entry describes an activity to be recorded.
type Entry struct {
	// Action is one of the Action* constants.
	Action string
	// TargetType is one of the TargetType* constants.
	TargetType string
	// Target is a human readable target (file URI, share ID, email...).
	Target string
	// FileID is the related file ID when known.
	FileID int
	// UserID explicitly overrides the actor from the request context.
	UserID int
	// Metadata carries optional structured context.
	Metadata map[string]any
}

// Record persists an activity entry on a best-effort basis. Failures are
// logged and never propagated to the caller, so auditing cannot break the
// operation being audited.
func Record(c *gin.Context, entry Entry) {
	if c == nil {
		return
	}

	dep := dependency.FromContext(c)
	if dep == nil {
		return
	}

	ctx := context.Context(c)
	if !dep.SettingProvider().ActivityLogEnabled(ctx) {
		return
	}

	userID := entry.UserID
	if userID <= 0 {
		userID = inventory.UserIDFromContext(c)
	}

	action := entry.Action
	if action == "" {
		return
	}
	targetType := entry.TargetType
	if targetType == "" {
		targetType = TargetTypeSystem
	}

	create := dep.DBClient().Activity.Create().
		SetAction(action).
		SetTargetType(targetType)

	if userID > 0 {
		create.SetUserID(userID)
	}
	if entry.Target != "" {
		create.SetTarget(entry.Target)
	}
	if entry.FileID > 0 {
		create.SetFileID(entry.FileID)
	}
	if ip := c.ClientIP(); ip != "" {
		create.SetIP(ip)
	}
	if c.Request != nil {
		if ua := c.Request.UserAgent(); ua != "" {
			create.SetUserAgent(ua)
		}
	}
	if len(entry.Metadata) > 0 {
		create.SetMetadata(entry.Metadata)
	}

	if _, err := create.Save(ctx); err != nil {
		dep.Logger().Warning("Failed to record activity %q: %s", action, err)
	}
}

// RecordFile records a file related activity.
func RecordFile(c *gin.Context, action, uri string, metadata map[string]any) {
	Record(c, Entry{
		Action:     action,
		TargetType: TargetTypeFile,
		Target:     uri,
		Metadata:   metadata,
	})
}
