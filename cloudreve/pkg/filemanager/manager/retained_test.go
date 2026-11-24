package manager

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/stretchr/testify/require"
)

func TestIsRetainedObjectExpired(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	retention := 7 * 24 * time.Hour

	tests := []struct {
		name      string
		createdAt time.Time
		retention time.Duration
		want      bool
	}{
		{name: "older than retention", createdAt: now.Add(-8 * 24 * time.Hour), retention: retention, want: true},
		{name: "newer than retention", createdAt: now.Add(-6 * 24 * time.Hour), retention: retention, want: false},
		{name: "exactly at retention cutoff", createdAt: now.Add(-retention), retention: retention, want: true},
		{name: "zero retention expires immediately", createdAt: now, retention: 0, want: true},
		{name: "negative retention expires immediately", createdAt: now, retention: -time.Hour, want: true},
		{name: "future object is never expired", createdAt: now.Add(time.Hour), retention: retention, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsRetainedObjectExpired(tt.createdAt, now, tt.retention))
		})
	}
}

func TestIsRetainedObjectStillReferenced(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "retained.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	policy, err := client.StoragePolicy.Create().
		SetName("test").
		SetType(types.PolicyTypeLocal).
		Save(ctx)
	require.NoError(t, err)

	record := &ent.RetainedObject{
		StoragePolicyID: policy.ID,
		Source:          "uploads/retained-object",
		State:           inventory.RetainedObjectStatePending,
	}

	referenced, err := isRetainedObjectStillReferenced(ctx, client, record)
	require.NoError(t, err)
	require.False(t, referenced, "an object without live entities is deletable")

	live, err := client.Entity.Create().
		SetType(int(types.EntityTypeVersion)).
		SetSource(record.Source).
		SetSize(1).
		SetStoragePolicyID(policy.ID).
		Save(ctx)
	require.NoError(t, err)

	referenced, err = isRetainedObjectStillReferenced(ctx, client, record)
	require.NoError(t, err)
	require.True(t, referenced, "a live entity must block deletion")

	// Soft-delete the live entity: cleanup must ignore it.
	require.NoError(t, client.Entity.DeleteOneID(live.ID).Exec(ctx))
	referenced, err = isRetainedObjectStillReferenced(ctx, client, record)
	require.NoError(t, err)
	require.False(t, referenced, "soft-deleted entities do not block cleanup")

	// An entity on another policy with the same key must not block cleanup.
	otherPolicy, err := client.StoragePolicy.Create().
		SetName("other").
		SetType(types.PolicyTypeLocal).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Entity.Create().
		SetType(int(types.EntityTypeVersion)).
		SetSource(record.Source).
		SetSize(1).
		SetStoragePolicyID(otherPolicy.ID).
		Save(ctx)
	require.NoError(t, err)

	referenced, err = isRetainedObjectStillReferenced(ctx, client, record)
	require.NoError(t, err)
	require.False(t, referenced, "entities on other policies do not block cleanup")
}
