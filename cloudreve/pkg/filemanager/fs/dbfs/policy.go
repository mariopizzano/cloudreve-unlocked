package dbfs

import (
	"context"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

// GetStoragePolicy returns the storage policy mounted on the given folder (if any),
// together with the effective policy used for new uploads in that folder.
func (f *DBFS) GetStoragePolicy(ctx context.Context, uri *fs.URI) (*fs.StoragePolicyMount, error) {
	navigator, err := f.getNavigator(ctx, uri, NavigatorCapabilityInfo)
	if err != nil {
		return nil, err
	}

	target, err := f.getFileByPath(ctx, navigator, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to get target folder: %w", err)
	}

	if target.Type() != types.FileTypeFolder {
		return nil, fs.ErrNotSupportedAction.WithError(fmt.Errorf("storage policy can only be mounted on folders"))
	}

	res := &fs.StoragePolicyMount{}
	if target.Model.StoragePolicyFiles > 0 {
		mounted, err := f.storagePolicyClient.GetPolicyByID(ctx, target.Model.StoragePolicyFiles)
		if err != nil {
			f.l.Warning("Failed to get mounted storage policy %d for folder %q: %s", target.Model.StoragePolicyFiles, target.Model.Name, err)
		} else {
			res.Mounted = mounted
		}
	}

	res.Effective, err = f.getPreferredPolicy(ctx, target)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// MountStoragePolicy mounts the given storage policy on a folder, or clears the mount
// if policyID is zero. Existing files are not affected, only new uploads will use it.
func (f *DBFS) MountStoragePolicy(ctx context.Context, uri *fs.URI, policyID int) error {
	navigator, err := f.getNavigator(ctx, uri, NavigatorCapabilityModifyProps, NavigatorCapabilityLockFile)
	if err != nil {
		return err
	}

	target, err := f.getFileByPath(ctx, navigator, uri)
	if err != nil {
		return fmt.Errorf("failed to get target folder: %w", err)
	}

	if target.Type() != types.FileTypeFolder {
		return fs.ErrNotSupportedAction.WithError(fmt.Errorf("storage policy can only be mounted on folders"))
	}

	if target.OwnerID() != f.user.ID && !f.user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return fs.ErrOwnerOnly.WithError(fmt.Errorf("only file owner can mount storage policy"))
	}

	if policyID > 0 {
		if _, err := f.storagePolicyClient.GetPolicyByID(ctx, policyID); err != nil {
			return serializer.NewError(serializer.CodePolicyNotExist, "Storage policy not found", err)
		}
	}

	// Lock target
	lr := &LockByPath{target.Uri(true), target, target.Type(), ""}
	ls, err := f.acquireByPath(ctx, -1, f.user, true, fs.LockApp(fs.ApplicationUpdateMetadata), lr)
	defer func() { _ = f.Release(ctx, ls) }()
	if err != nil {
		return err
	}

	if err := f.fileClient.SetStoragePolicy(ctx, target.Model, policyID); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to update folder storage policy", err)
	}

	target.Model.StoragePolicyFiles = policyID
	return nil
}
