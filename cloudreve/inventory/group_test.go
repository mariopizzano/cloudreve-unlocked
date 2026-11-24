package inventory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/ent/groupstoragepolicy"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/stretchr/testify/require"
)

func newTestGroup(name string, defaultID int) *ent.Group {
	group := &ent.Group{
		Name:        name,
		MaxStorage:  1 << 30,
		Permissions: &boolset.BooleanSet{},
		Settings:    &types.GroupSetting{},
	}
	if defaultID > 0 {
		group.Edges.StoragePolicies = &ent.StoragePolicy{ID: defaultID}
	}
	return group
}

func TestNormalizeGroupPolicyPool(t *testing.T) {
	pool, defaultID := NormalizeGroupPolicyPool([]int{2, 1, 2, 0, -1}, 3)
	require.Equal(t, []int{2, 1, 3}, pool)
	require.Equal(t, 3, defaultID)

	pool, defaultID = NormalizeGroupPolicyPool([]int{0}, 0)
	require.Empty(t, pool)
	require.Zero(t, defaultID)

	pool, defaultID = NormalizeGroupPolicyPool([]int{4, 5}, 0)
	require.Equal(t, []int{4, 5}, pool)
	require.Equal(t, 4, defaultID)
}

func TestGroupStoragePolicyPoolUpsertAndSync(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "group-pool.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	policies := make([]*ent.StoragePolicy, 0, 3)
	for _, name := range []string{"one", "two", "three"} {
		policy, err := client.StoragePolicy.Create().
			SetName(name).
			SetType(types.PolicyTypeLocal).
			Save(ctx)
		require.NoError(t, err)
		policies = append(policies, policy)
	}

	groupClient := NewGroupClient(client, conf.SQLiteDB, nil)
	policyClient := NewStoragePolicyClient(client, nil)

	created, err := groupClient.Upsert(ctx, newTestGroup("test", policies[1].ID), []int{policies[0].ID, policies[1].ID, policies[1].ID})
	require.NoError(t, err)
	require.Equal(t, policies[1].ID, created.StoragePolicyID)

	poolRows, err := client.GroupStoragePolicy.Query().
		Where(groupstoragepolicy.GroupID(created.ID)).
		Order(ent.Asc(groupstoragepolicy.FieldPolicyID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, poolRows, 2)
	require.Equal(t, policies[0].ID, poolRows[0].PolicyID)
	require.Equal(t, policies[1].ID, poolRows[1].PolicyID)

	available, err := policyClient.GetAvailableByGroup(ctx, created)
	require.NoError(t, err)
	require.Len(t, available, 2)

	defaultPolicy, err := policyClient.GetDefaultByGroup(ctx, created)
	require.NoError(t, err)
	require.Equal(t, policies[1].ID, defaultPolicy.ID)

	availableToGroup, err := policyClient.IsPolicyAvailableToGroup(ctx, created, policies[0].ID)
	require.NoError(t, err)
	require.True(t, availableToGroup)
	availableToGroup, err = policyClient.IsPolicyAvailableToGroup(ctx, created, policies[2].ID)
	require.NoError(t, err)
	require.False(t, availableToGroup)

	// Replace the pool and make policy three the default.
	created.Edges.StoragePolicies = &ent.StoragePolicy{ID: policies[2].ID}
	updated, err := groupClient.Upsert(ctx, created, []int{policies[0].ID, policies[2].ID})
	require.NoError(t, err)
	require.Equal(t, policies[2].ID, updated.StoragePolicyID)

	available, err = policyClient.GetAvailableByGroup(ctx, updated)
	require.NoError(t, err)
	require.Len(t, available, 2)
	require.Equal(t, []int{policies[0].ID, policies[2].ID}, []int{available[0].ID, available[1].ID})

	// A default outside the submitted pool is added to it.
	created.Edges.StoragePolicies = &ent.StoragePolicy{ID: policies[2].ID}
	created, err = groupClient.Upsert(ctx, created, []int{policies[1].ID})
	require.NoError(t, err)
	available, err = policyClient.GetAvailableByGroup(ctx, created)
	require.NoError(t, err)
	require.Len(t, available, 2)
	require.Equal(t, policies[2].ID, created.StoragePolicyID)

	// Deleting the group clears the join rows.
	usage, err := groupClient.CountGroupsUsingPolicy(ctx, policies[1].ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage)
	require.NoError(t, groupClient.Delete(ctx, created.ID))
	usage, err = groupClient.CountGroupsUsingPolicy(ctx, policies[1].ID)
	require.NoError(t, err)
	require.Zero(t, usage)
}

func TestGroupStoragePolicyPoolLegacyFallback(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "group-legacy.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	policy, err := client.StoragePolicy.Create().
		SetName("legacy").
		SetType(types.PolicyTypeLocal).
		Save(ctx)
	require.NoError(t, err)

	// Simulate a database that has not been backfilled: only storage_policy_id is set.
	group, err := client.Group.Create().
		SetName("legacy group").
		SetMaxStorage(1 << 30).
		SetPermissions(&boolset.BooleanSet{}).
		SetSettings(&types.GroupSetting{}).
		SetStoragePolicyID(policy.ID).
		Save(ctx)
	require.NoError(t, err)

	policyClient := NewStoragePolicyClient(client, nil)
	available, err := policyClient.GetAvailableByGroup(ctx, group)
	require.NoError(t, err)
	require.Len(t, available, 1)
	require.Equal(t, policy.ID, available[0].ID)

	defaultPolicy, err := policyClient.GetDefaultByGroup(ctx, group)
	require.NoError(t, err)
	require.Equal(t, policy.ID, defaultPolicy.ID)

	ok, err := policyClient.IsPolicyAvailableToGroup(ctx, group, policy.ID)
	require.NoError(t, err)
	require.True(t, ok)
}
