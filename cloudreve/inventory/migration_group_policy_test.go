package inventory

import (
	"context"
	"path/filepath"
	"testing"

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
