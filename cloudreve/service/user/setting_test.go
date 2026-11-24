package user

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/stretchr/testify/require"
)

func TestContainsPolicy(t *testing.T) {
	pool := []*ent.StoragePolicy{{ID: 1}, {ID: 2}}

	require.True(t, containsPolicy(pool, 1))
	require.True(t, containsPolicy(pool, 2))
	require.False(t, containsPolicy(pool, 3))
	require.False(t, containsPolicy(nil, 1))
}
