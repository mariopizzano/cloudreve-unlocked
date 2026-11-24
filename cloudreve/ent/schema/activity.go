package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Activity stores an audit trail entry for a user action. Entries are written
// best-effort and never contain secrets or file contents.
type Activity struct {
	ent.Schema
}

// Fields of the Activity.
func (Activity) Fields() []ent.Field {
	return []ent.Field{
		// Actor. Optional to support anonymous or unknown users (e.g. failed
		// login attempts with an unknown email address).
		field.Int("user_id").Optional(),
		// Stable action identifier, e.g. "login_success" or "file_upload".
		field.String("action").MaxLen(64),
		// Coarse target category: file, share, user or system.
		field.String("target_type").MaxLen(32).Default(""),
		// Human readable target, usually the file URI or object name.
		field.Text("target").Optional(),
		// Related file ID when known, kept for audit only. Deliberately not a
		// foreign key so the record survives file deletion.
		field.Int("file_id").Optional(),
		// Client IP address and user agent as seen by the server.
		field.String("ip").MaxLen(64).Optional(),
		field.Text("user_agent").Optional(),
		// Extra structured context (old/new name, share ID, byte size...).
		field.JSON("metadata", map[string]any{}).Optional(),
	}
}

// Edges of the Activity.
// Intentional: the actor is stored as a plain user_id without an edge/FK so
// audit rows survive user deletion and can be created on databases that only
// migrate the additive activities table.
func (Activity) Edges() []ent.Edge {
	return nil
}

// Mixin of the Activity.
func (Activity) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

// Indexes of the Activity.
func (Activity) Indexes() []ent.Index {
	return []ent.Index{
		// User-facing history, newest first.
		index.Fields("user_id", "created_at"),
		// Admin filtering by action.
		index.Fields("action", "created_at"),
		// Global admin listing / retention queries.
		index.Fields("created_at"),
		// Admin filtering by IP.
		index.Fields("ip", "created_at"),
	}
}
