package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GroupStoragePolicy is the join entity between a group and the storage policies
// that are available to it. The group's storage_policy_id keeps acting as the
// default policy for backward compatibility.
type GroupStoragePolicy struct {
	ent.Schema
}

func (GroupStoragePolicy) Fields() []ent.Field {
	return []ent.Field{
		field.Int("group_id"),
		field.Int("policy_id"),
	}
}

func (GroupStoragePolicy) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

func (GroupStoragePolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "policy_id").Unique(),
	}
}

func (GroupStoragePolicy) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).
			Ref("storage_policy_pool").
			Field("group_id").
			Unique().
			Required(),
		edge.From("storage_policy", StoragePolicy.Type).
			Ref("group_policy_pool").
			Field("policy_id").
			Unique().
			Required(),
	}
}
