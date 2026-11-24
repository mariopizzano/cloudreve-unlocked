package inventory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/ent/groupstoragepolicy"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/stretchr/testify/require"
)

func TestMigrateGroupStoragePoliciesBackfillIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "group-pool-migration.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	policy, err := client.StoragePolicy.Create().
		SetName("legacy").
		SetType(types.PolicyTypeLocal).
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("legacy group").
		SetMaxStorage(1 << 30).
		SetPermissions(&boolset.BooleanSet{}).
		SetSettings(&types.GroupSetting{}).
		SetStoragePolicyID(policy.ID).
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, migrateGroupStoragePolicies(client, ctx))
	require.NoError(t, migrateGroupStoragePolicies(client, ctx))

	count, err := client.GroupStoragePolicy.Query().
		Where(
			groupstoragepolicy.GroupID(group.ID),
			groupstoragepolicy.PolicyID(policy.ID),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestMigrateGroupStoragePolicyPoolSeedExpandsBackfilledPools(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "group-pool-seed.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	createPolicy := func(name string) *ent.StoragePolicy {
		p, err := client.StoragePolicy.Create().
			SetName(name).
			SetType(types.PolicyTypeLocal).
			Save(ctx)
		require.NoError(t, err)
		return p
	}

	defaultPolicy := createPolicy("default")
	otherPolicy := createPolicy("other")
	archivedPolicy := createPolicy("archived")
	_, err := archivedPolicy.Update().SetDeletedAt(time.Now()).Save(ctx)
	require.NoError(t, err)

	createGroup := func(name string) *ent.Group {
		g, err := client.Group.Create().
			SetName(name).
			SetMaxStorage(1 << 30).
			SetPermissions(&boolset.BooleanSet{}).
			SetSettings(&types.GroupSetting{}).
			SetStoragePolicyID(defaultPolicy.ID).
			Save(ctx)
		require.NoError(t, err)
		return g
	}

	auto := createGroup("auto seeded")
	curated := createGroup("curated")

	require.NoError(t, migrateGroupStoragePolicies(client, ctx))

	// Simulate an explicitly curated pool: more than the auto-seeded default.
	_, err = client.GroupStoragePolicy.Create().
		SetGroupID(curated.ID).
		SetPolicyID(otherPolicy.ID).
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, migrateGroupStoragePolicyPoolSeed(client, ctx))
	require.NoError(t, migrateGroupStoragePolicyPoolSeed(client, ctx))

	autoPool, err := client.GroupStoragePolicy.Query().
		Where(groupstoragepolicy.GroupID(auto.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, autoPool, 2, "auto-seeded pool must be expanded with other active policies")

	curatedPool, err := client.GroupStoragePolicy.Query().
		Where(groupstoragepolicy.GroupID(curated.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, curatedPool, 2, "curated pool must not be touched")

	// The marker makes the seed one-time: policies added later are not injected.
	late := createPolicy("late")
	require.NoError(t, migrateGroupStoragePolicyPoolSeed(client, ctx))
	autoPool, err = client.GroupStoragePolicy.Query().
		Where(groupstoragepolicy.GroupID(auto.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, autoPool, 2)
	for _, entry := range autoPool {
		require.NotEqual(t, late.ID, entry.PolicyID)
		require.NotEqual(t, archivedPolicy.ID, entry.PolicyID, "soft-deleted policies must be skipped")
	}
}
