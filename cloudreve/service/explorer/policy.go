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
	GetUserStoragePolicyParameterCtx struct{}
	GetUserStoragePolicyService      struct{}

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

	UserStoragePolicyResponse struct {
		Policies  []StoragePolicy `json:"policies"`
		Default   *StoragePolicy  `json:"default,omitempty"`
		Preferred *StoragePolicy  `json:"preferred,omitempty"`
		Effective *StoragePolicy  `json:"effective,omitempty"`
	}
)

// Get returns the storage policy pool of the current user's group together with the
// group default, the user's preference and the effective policy for new uploads.
func (s *GetUserStoragePolicyService) Get(c *gin.Context) (*UserStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()

	group, err := dep.GroupClient().GetByID(c, u.GroupUsers)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user group", err)
	}

	policyClient := dep.StoragePolicyClient()
	available, err := policyClient.GetAvailableByGroup(c, group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get available storage policies", err)
	}

	var (
		defaultPolicy   *ent.StoragePolicy
		preferredPolicy *ent.StoragePolicy
	)
	if group.StoragePolicyID > 0 {
		for _, policy := range available {
			if policy.ID == group.StoragePolicyID {
				defaultPolicy = policy
				break
			}
		}
	}
	if defaultPolicy == nil && len(available) > 0 {
		defaultPolicy = available[0]
	}

	if u.Settings != nil && u.Settings.PreferredPolicy > 0 {
		for _, policy := range available {
			if policy.ID == u.Settings.PreferredPolicy {
				preferredPolicy = policy
				break
			}
		}
	}

	effective := preferredPolicy
	if effective == nil {
		effective = defaultPolicy
	}
	if effective != nil {
		resolved, err := policyClient.ResolveUploadPolicy(c, effective, false)
		if err == nil {
			effective = resolved
		}
	}

	return &UserStoragePolicyResponse{
		Policies: lo.Map(available, func(policy *ent.StoragePolicy, _ int) StoragePolicy {
			return *BuildStoragePolicy(policy, hasher)
		}),
		Default:   BuildStoragePolicy(defaultPolicy, hasher),
		Preferred: BuildStoragePolicy(preferredPolicy, hasher),
		Effective: BuildStoragePolicy(effective, hasher),
	}, nil
}

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

	group, err := dep.GroupClient().GetByID(c, user.GroupUsers)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user group", err)
	}

	policies, err := dep.StoragePolicyClient().GetAvailableByGroup(c, group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get available storage policies", err)
	}

	policies = mergeMountPolicies(policies, res.Mounted)

	hasher := dep.HashIDEncoder()
	return &StoragePolicyMountResponse{
		Mounted:   BuildStoragePolicy(res.Mounted, hasher),
		Effective: BuildStoragePolicy(res.Effective, hasher),
		Policies: lo.Map(policies, func(policy *ent.StoragePolicy, index int) StoragePolicy {
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

	if policyID > 0 {
		group, err := dep.GroupClient().GetByID(c, user.GroupUsers)
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to get user group", err)
		}

		available, err := dep.StoragePolicyClient().IsPolicyAvailableToGroup(c, group, policyID)
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to validate storage policy", err)
		}
		if !available {
			return serializer.NewError(serializer.CodePolicyNotAllowed, "Storage policy is not available to your group", nil)
		}
	}

	return m.MountStoragePolicy(c, uri, policyID)
}

// mergeMountPolicies keeps a legacy mounted policy visible even when it is no longer
// part of the group's pool, so the user can inspect or unmount it.
func mergeMountPolicies(pool []*ent.StoragePolicy, mounted *ent.StoragePolicy) []*ent.StoragePolicy {
	if mounted == nil {
		return pool
	}
	for _, policy := range pool {
		if policy.ID == mounted.ID {
			return pool
		}
	}
	return append(pool, mounted)
}
