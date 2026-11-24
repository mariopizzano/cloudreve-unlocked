package admin

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/stretchr/testify/assert"
)

func memberPolicy(id int, policyType string, relay bool, chunkSize int64) *ent.StoragePolicy {
	return &ent.StoragePolicy{
		ID:   id,
		Type: policyType,
		Settings: &types.PolicySetting{
			Relay:     relay,
			ChunkSize: chunkSize,
		},
	}
}

func loadBalancePolicy(id int, members ...types.LoadBalanceMember) *ent.StoragePolicy {
	return &ent.StoragePolicy{
		ID:   id,
		Type: types.PolicyTypeLoadBalance,
		Settings: &types.PolicySetting{
			Members: members,
		},
	}
}

func TestValidateLoadBalanceMembers(t *testing.T) {
	asserts := assert.New(t)

	members := map[int]*ent.StoragePolicy{
		1: memberPolicy(1, types.PolicyTypeS3, false, 5<<20),
		2: memberPolicy(2, types.PolicyTypeS3, false, 5<<20),
	}
	policy := loadBalancePolicy(100,
		types.LoadBalanceMember{PolicyID: 1, Weight: 3},
		types.LoadBalanceMember{PolicyID: 2, Weight: 1},
	)

	asserts.NoError(validateLoadBalanceMembers(policy, members))
	asserts.Equal(types.LoadBalanceStrategyRoundRobin, policy.Settings.LoadBalanceStrategy)
	asserts.Equal([]types.LoadBalanceMember{
		{PolicyID: 1, Weight: 3},
		{PolicyID: 2, Weight: 1},
	}, policy.Settings.Members)
}

func TestValidateLoadBalanceMembersNormalizesWeights(t *testing.T) {
	asserts := assert.New(t)

	members := map[int]*ent.StoragePolicy{
		1: memberPolicy(1, types.PolicyTypeS3, false, 0),
		2: memberPolicy(2, types.PolicyTypeS3, false, 0),
	}
	policy := loadBalancePolicy(100,
		types.LoadBalanceMember{PolicyID: 1, Weight: 0},
		types.LoadBalanceMember{PolicyID: 2, Weight: -5},
	)

	asserts.NoError(validateLoadBalanceMembers(policy, members))
	asserts.Equal([]types.LoadBalanceMember{
		{PolicyID: 1, Weight: types.LoadBalanceDefaultWeight},
		{PolicyID: 2, Weight: types.LoadBalanceDefaultWeight},
	}, policy.Settings.Members)
}

func TestValidateLoadBalanceMembersEmpty(t *testing.T) {
	asserts := assert.New(t)

	policy := loadBalancePolicy(100)
	asserts.Error(validateLoadBalanceMembers(policy, nil))
}

func TestValidateLoadBalanceMembersSelfReference(t *testing.T) {
	asserts := assert.New(t)

	policy := loadBalancePolicy(100, types.LoadBalanceMember{PolicyID: 100, Weight: 1})
	members := map[int]*ent.StoragePolicy{
		100: memberPolicy(100, types.PolicyTypeS3, false, 0),
	}
	asserts.Error(validateLoadBalanceMembers(policy, members))
}

func TestValidateLoadBalanceMembersNested(t *testing.T) {
	asserts := assert.New(t)

	policy := loadBalancePolicy(100, types.LoadBalanceMember{PolicyID: 1, Weight: 1})
	members := map[int]*ent.StoragePolicy{
		1: memberPolicy(1, types.PolicyTypeLoadBalance, false, 0),
	}
	asserts.Error(validateLoadBalanceMembers(policy, members))
}

func TestValidateLoadBalanceMembersMissing(t *testing.T) {
	asserts := assert.New(t)

	policy := loadBalancePolicy(100,
		types.LoadBalanceMember{PolicyID: 1, Weight: 1},
		types.LoadBalanceMember{PolicyID: 2, Weight: 1},
	)
	members := map[int]*ent.StoragePolicy{
		1: memberPolicy(1, types.PolicyTypeS3, false, 0),
	}
	asserts.Error(validateLoadBalanceMembers(policy, members))
}

func TestValidateLoadBalanceMembersIncompatible(t *testing.T) {
	asserts := assert.New(t)

	tests := []struct {
		name    string
		members map[int]*ent.StoragePolicy
	}{
		{
			name: "different provider type",
			members: map[int]*ent.StoragePolicy{
				1: memberPolicy(1, types.PolicyTypeS3, false, 0),
				2: memberPolicy(2, types.PolicyTypeLocal, false, 0),
			},
		},
		{
			name: "different relay setting",
			members: map[int]*ent.StoragePolicy{
				1: memberPolicy(1, types.PolicyTypeS3, false, 0),
				2: memberPolicy(2, types.PolicyTypeS3, true, 0),
			},
		},
		{
			name: "different chunk size",
			members: map[int]*ent.StoragePolicy{
				1: memberPolicy(1, types.PolicyTypeS3, false, 0),
				2: memberPolicy(2, types.PolicyTypeS3, false, 5<<20),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := loadBalancePolicy(100,
				types.LoadBalanceMember{PolicyID: 1, Weight: 1},
				types.LoadBalanceMember{PolicyID: 2, Weight: 1},
			)
			asserts.Error(validateLoadBalanceMembers(policy, test.members))
		})
	}
}

func TestValidateLoadBalanceMembersUnsupportedStrategy(t *testing.T) {
	asserts := assert.New(t)

	policy := loadBalancePolicy(100, types.LoadBalanceMember{PolicyID: 1, Weight: 1})
	policy.Settings.LoadBalanceStrategy = "random"
	members := map[int]*ent.StoragePolicy{
		1: memberPolicy(1, types.PolicyTypeS3, false, 0),
	}
	asserts.Error(validateLoadBalanceMembers(policy, members))
}
