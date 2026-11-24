package explorer

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/stretchr/testify/require"
)

func TestMergeMountPolicies(t *testing.T) {
	pool := []*ent.StoragePolicy{{ID: 1}, {ID: 2}}

	res := mergeMountPolicies(pool, nil)
	require.Len(t, res, 2)

	res = mergeMountPolicies(pool, &ent.StoragePolicy{ID: 2})
	require.Len(t, res, 2)

	res = mergeMountPolicies(pool, &ent.StoragePolicy{ID: 3})
	require.Len(t, res, 3)
	require.Equal(t, 3, res[2].ID)
}
