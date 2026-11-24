package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RetainedObject tracks a physical storage object that was intentionally left
// behind when a file entity was relocated with delete_source disabled. The
// object stays on its original storage policy until the retention window
// expires, then a periodic cleanup removes it.
type RetainedObject struct {
	ent.Schema
}

// Fields of the RetainedObject.
func (RetainedObject) Fields() []ent.Field {
	return []ent.Field{
		// Storage policy the physical object still lives on.
		field.Int("storage_policy_id"),
		// Physical object key/path inside the storage policy.
		field.Text("source"),
		// Original entity size in bytes, kept for reporting only.
		field.Int64("size").Default(0),
		// Original entity type (primary, version, thumbnail...).
		field.Int("entity_type").Default(0),
		// Origin file and entity IDs. These may reference rows that no longer
		// exist: they are kept for audit only and deliberately not encoded as
		// foreign keys.
		field.Int("file_id").Optional(),
		field.Int("entity_id").Optional(),
		// Owner of the origin file, kept for reporting only.
		field.Int("owner_id").Optional(),
		// Cleanup state. See inventory.RetainedObjectState* constants.
		field.String("state").Default("pending"),
		// Number of failed physical deletion attempts.
		field.Int("attempts").Default(0),
		// Last error encountered while processing this object.
		field.Text("last_error").Optional().Nillable(),
	}
}

// Mixin of the RetainedObject.
func (RetainedObject) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

// Indexes of the RetainedObject.
func (RetainedObject) Indexes() []ent.Index {
	return []ent.Index{
		// Cleanup picks pending rows ordered by creation time.
		index.Fields("state", "created_at"),
		// Safety check / reporting per storage policy.
		index.Fields("storage_policy_id", "state"),
	}
}
