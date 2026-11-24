package inventory

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/stretchr/testify/assert"
)

func TestLoadBalanceMembers(t *testing.T) {
	asserts := assert.New(t)

	asserts.Nil(LoadBalanceMembers(nil))
	asserts.Nil(LoadBalanceMembers(&ent.StoragePolicy{}))

	policy := &ent.StoragePolicy{
		Settings: &types.PolicySetting{
			Members: []types.LoadBalanceMember{
				{PolicyID: 0, Weight: 2},
				{PolicyID: 5, Weight: 0},
				{PolicyID: 5, Weight: 9},
				{PolicyID: -1, Weight: 1},
				{PolicyID: 3, Weight: -4},
				{PolicyID: 7, Weight: types.LoadBalanceMaxWeight + 10},
			},
		},
	}
	members := LoadBalanceMembers(policy)
	asserts.Equal([]types.LoadBalanceMember{
		{PolicyID: 5, Weight: types.LoadBalanceDefaultWeight},
		{PolicyID: 3, Weight: types.LoadBalanceDefaultWeight},
		{PolicyID: 7, Weight: types.LoadBalanceMaxWeight},
	}, members)
}

func TestBuildLoadBalanceCycle(t *testing.T) {
	asserts := assert.New(t)

	asserts.Nil(BuildLoadBalanceCycle(nil))

	cycle := BuildLoadBalanceCycle([]types.LoadBalanceMember{
		{PolicyID: 1, Weight: 3},
		{PolicyID: 2, Weight: 1},
	})
	asserts.Equal([]int{1, 2, 1, 1}, cycle)

	cycle = BuildLoadBalanceCycle([]types.LoadBalanceMember{
		{PolicyID: 1, Weight: 2},
		{PolicyID: 2, Weight: 1},
	})
	asserts.Equal([]int{1, 2, 1}, cycle)

	cycle = BuildLoadBalanceCycle([]types.LoadBalanceMember{
		{PolicyID: 1, Weight: 1},
		{PolicyID: 2, Weight: 1},
	})
	asserts.Equal([]int{1, 2}, cycle)
}

func TestNextLoadBalanceIndexRoundRobin(t *testing.T) {
	asserts := assert.New(t)
	kv := cache.NewMemoStore("", nil)

	var indexes []int
	for i := 0; i < 7; i++ {
		index, err := nextLoadBalanceIndex(kv, 42, 3)
		asserts.NoError(err)
		indexes = append(indexes, index)
	}

	asserts.Equal([]int{0, 1, 2, 0, 1, 2, 0}, indexes)
}

func TestNextLoadBalanceIndexIndependentPolicies(t *testing.T) {
	asserts := assert.New(t)
	kv := cache.NewMemoStore("", nil)

	first, err := nextLoadBalanceIndex(kv, 1, 2)
	asserts.NoError(err)
	asserts.Zero(first)

	second, err := nextLoadBalanceIndex(kv, 2, 2)
	asserts.NoError(err)
	asserts.Zero(second)

	first, err = nextLoadBalanceIndex(kv, 1, 2)
	asserts.NoError(err)
	asserts.Equal(1, first)
}

func TestNextLoadBalanceIndexNoMembers(t *testing.T) {
	asserts := assert.New(t)
	kv := cache.NewMemoStore("", nil)

	_, err := nextLoadBalanceIndex(kv, 1, 0)
	asserts.Error(err)
}

func TestNextLoadBalanceIndexWithoutKV(t *testing.T) {
	asserts := assert.New(t)

	index, err := nextLoadBalanceIndex(nil, 1, 4)
	asserts.NoError(err)
	asserts.Zero(index)
}

// TestWeightedDistribution verifies that a 3:1 policy sends 75% of the selections to the
// first member and 25% to the second one over a full cycle and across cycles.
func TestWeightedDistribution(t *testing.T) {
	asserts := assert.New(t)
	kv := cache.NewMemoStore("", nil)

	cycle := BuildLoadBalanceCycle([]types.LoadBalanceMember{
		{PolicyID: 1, Weight: 3},
		{PolicyID: 2, Weight: 1},
	})
	asserts.Len(cycle, 4)

	counts := map[int]int{}
	for i := 0; i < 400; i++ {
		index, err := nextLoadBalanceIndex(kv, 9, len(cycle))
		asserts.NoError(err)
		counts[cycle[index]]++
	}

	asserts.Equal(300, counts[1])
	asserts.Equal(100, counts[2])
}
