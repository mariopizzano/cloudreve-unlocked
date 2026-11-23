package explorer

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type (
	GetStoragePolicyParameterCtx struct{}
	GetStoragePolicyService      struct {
		Uri string `form:"uri" binding:"required"`
	}

	PatchStoragePolicyParameterCtx struct{}
	PatchStoragePolicyService      struct {
		Uri      string `json:"uri" binding:"required"`
		PolicyID string `json:"policy_id"`
	}

	StoragePolicyMountResponse struct {
		Mounted   *StoragePolicy  `json:"mounted,omitempty"`
		Effective *StoragePolicy  `json:"effective,omitempty"`
		Policies  []StoragePolicy `json:"policies"`
	}
)

// Get returns the storage policy mounted on a folder, the effective policy used for
// new uploads and the list of policies that can be mounted.
func (s *GetStoragePolicyService) Get(c *gin.Context) (*StoragePolicyMountResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(s.Uri)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	res, err := m.GetStoragePolicy(c, uri)
	if err != nil {
		return nil, err
	}

	policies, err := dep.StoragePolicyClient().ListPolicies(c, &inventory.ListPolicyParameters{
		PaginationArgs: &inventory.PaginationArgs{Page: 0, PageSize: 1000},
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list storage policies", err)
	}

	hasher := dep.HashIDEncoder()
	return &StoragePolicyMountResponse{
		Mounted:   BuildStoragePolicy(res.Mounted, hasher),
		Effective: BuildStoragePolicy(res.Effective, hasher),
		Policies: lo.Map(policies.Policies, func(policy *ent.StoragePolicy, index int) StoragePolicy {
			return *BuildStoragePolicy(policy, hasher)
		}),
	}, nil
}

// Patch mounts or unmounts a storage policy on a folder. An empty policy_id clears
// the mount, so that the folder inherits the policy from its closest parent.
func (s *PatchStoragePolicyService) Patch(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(s.Uri)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	var policyID int
	if s.PolicyID != "" {
		policyID, err = dep.HashIDEncoder().Decode(s.PolicyID, hashid.PolicyID)
		if err != nil {
			return serializer.NewError(serializer.CodeParamErr, "unknown storage policy id", err)
		}
	}

	return m.MountStoragePolicy(c, uri, policyID)
}
