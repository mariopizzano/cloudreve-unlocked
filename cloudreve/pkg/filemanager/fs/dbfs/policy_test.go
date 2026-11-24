package dbfs

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/stretchr/testify/require"
)

func TestChoosePreferredPolicyPrecedence(t *testing.T) {
	mounted := &ent.StoragePolicy{ID: 1, Name: "mounted", Type: types.PolicyTypeLocal}
	preferred := &ent.StoragePolicy{ID: 2, Name: "preferred", Type: types.PolicyTypeLocal}
	groupDefault := &ent.StoragePolicy{ID: 3, Name: "group", Type: types.PolicyTypeLocal}
	systemDefault := &ent.StoragePolicy{ID: 4, Name: "system", Type: types.PolicyTypeLocal}

	require.Equal(t, mounted, choosePreferredPolicy(mounted, preferred, groupDefault, systemDefault))
	require.Equal(t, preferred, choosePreferredPolicy(nil, preferred, groupDefault, systemDefault))
	require.Equal(t, groupDefault, choosePreferredPolicy(nil, nil, groupDefault, systemDefault))
	require.Equal(t, systemDefault, choosePreferredPolicy(nil, nil, nil, systemDefault))
	require.Nil(t, choosePreferredPolicy(nil, nil, nil, nil))
}

// The preference may resolve to a virtual load_balance policy. It must be kept as
// configured here; concrete resolution only happens when PrepareUpload advances it.
func TestChoosePreferredPolicyKeepsLoadBalanceConfigured(t *testing.T) {
	loadBalance := &ent.StoragePolicy{ID: 9, Name: "lb", Type: types.PolicyTypeLoadBalance}
	groupDefault := &ent.StoragePolicy{ID: 3, Name: "group", Type: types.PolicyTypeLocal}

	require.Equal(t, loadBalance, choosePreferredPolicy(nil, loadBalance, groupDefault, nil))
}
