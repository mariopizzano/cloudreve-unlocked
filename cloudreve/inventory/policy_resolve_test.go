package inventory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/stretchr/testify/require"
)

func TestResolveUploadPolicyReturnsConcreteMember(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "resolve-policy.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	member, err := client.StoragePolicy.Create().
		SetName("member").
		SetType(types.PolicyTypeLocal).
		Save(ctx)
	require.NoError(t, err)

	lb, err := client.StoragePolicy.Create().
		SetName("lb").
		SetType(types.PolicyTypeLoadBalance).
		SetSettings(&types.PolicySetting{
			Members: []types.LoadBalanceMember{{PolicyID: member.ID, Weight: 1}},
		}).
		Save(ctx)
	require.NoError(t, err)

	policyClient := NewStoragePolicyClient(client, nil)
	resolved, err := policyClient.ResolveUploadPolicy(ctx, lb, true)
	require.NoError(t, err)
	require.Equal(t, member.ID, resolved.ID)
	require.NotEqual(t, types.PolicyTypeLoadBalance, resolved.Type)
}
