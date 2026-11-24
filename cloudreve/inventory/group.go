package inventory

import (
	"context"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/group"
	"github.com/cloudreve/Cloudreve/v4/ent/groupstoragepolicy"
	"github.com/cloudreve/Cloudreve/v4/ent/schema"
	"github.com/cloudreve/Cloudreve/v4/ent/storagepolicy"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
)

type (
	// Ctx keys for eager loading options.
	LoadGroupPolicy struct{}
)

const (
	AnonymousGroupID = 3
)

type (
	GroupClient interface {
		TxOperator
		// AnonymousGroup returns the anonymous group.
		AnonymousGroup(ctx context.Context) (*ent.Group, error)
		// ListAll returns all groups.
		ListAll(ctx context.Context) ([]*ent.Group, error)
		// GetByID returns the group by id.
		GetByID(ctx context.Context, id int) (*ent.Group, error)
		// ListGroups returns a list of groups with pagination.
		ListGroups(ctx context.Context, args *ListGroupParameters) (*ListGroupResult, error)
		// CountUsers returns the number of users in the group.
		CountUsers(ctx context.Context, id int) (int, error)
		// Upsert upserts a group and synchronizes its available storage policy pool.
		Upsert(ctx context.Context, group *ent.Group, policyPool []int) (*ent.Group, error)
		// StoragePolicyPool returns the storage policies explicitly assigned to the group.
		StoragePolicyPool(ctx context.Context, groupID int) ([]*ent.StoragePolicy, error)
		// CountGroupsUsingPolicy counts groups that have the policy in their pool.
		CountGroupsUsingPolicy(ctx context.Context, policyID int) (int, error)
		// Delete deletes a group.
		Delete(ctx context.Context, id int) error
	}
	ListGroupParameters struct {
		*PaginationArgs
	}
	ListGroupResult struct {
		*PaginationResults
		Groups []*ent.Group
	}
)

func NewGroupClient(client *ent.Client, dbType conf.DBType, cache cache.Driver) GroupClient {
	return &groupClient{client: client, maxSQlParam: sqlParamLimit(dbType), cache: cache}
}

type groupClient struct {
	client      *ent.Client
	cache       cache.Driver
	maxSQlParam int
}

func (c *groupClient) SetClient(newClient *ent.Client) TxOperator {
	return &groupClient{client: newClient, maxSQlParam: c.maxSQlParam, cache: c.cache}
}

func (c *groupClient) GetClient() *ent.Client {
	return c.client
}

func (c *groupClient) CountUsers(ctx context.Context, id int) (int, error) {
	return c.client.Group.Query().Where(group.ID(id)).QueryUsers().Count(ctx)
}

func (c *groupClient) AnonymousGroup(ctx context.Context) (*ent.Group, error) {
	return withGroupEagerLoading(ctx, c.client.Group.Query().Where(group.ID(AnonymousGroupID))).First(ctx)
}

func (c *groupClient) ListAll(ctx context.Context) ([]*ent.Group, error) {
	return withGroupEagerLoading(ctx, c.client.Group.Query()).All(ctx)
}

// NormalizeGroupPolicyPool deduplicates the requested policy IDs and makes the
// default policy part of the pool. When no explicit default is provided, the first
// policy of the pool becomes the default.
func NormalizeGroupPolicyPool(ids []int, defaultID int) ([]int, int) {
	normalized := make([]int, 0, len(ids)+1)
	seen := make(map[int]struct{}, len(ids)+1)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}

	if defaultID > 0 {
		if _, ok := seen[defaultID]; !ok {
			normalized = append(normalized, defaultID)
		}
	} else if len(normalized) > 0 {
		defaultID = normalized[0]
	}

	return normalized, defaultID
}

func (c *groupClient) Upsert(ctx context.Context, group *ent.Group, policyPool []int) (*ent.Group, error) {
	defaultID := 0
	if group.Edges.StoragePolicies != nil {
		defaultID = group.Edges.StoragePolicies.ID
	}

	normalized, defaultID := NormalizeGroupPolicyPool(policyPool, defaultID)
	if group.ID != AnonymousGroupID && len(normalized) == 0 {
		return nil, fmt.Errorf("group storage policy pool cannot be empty")
	}

	if len(normalized) > 0 {
		count, err := c.client.StoragePolicy.Query().
			Where(storagepolicy.IDIn(normalized...)).
			Count(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to validate group storage policies: %w", err)
		}
		if count != len(normalized) {
			return nil, fmt.Errorf("one or more storage policies do not exist")
		}
	}

	gc, tx, ctx, err := WithTx(ctx, c)
	if err != nil {
		return nil, err
	}

	res, err := gc.upsertGroup(ctx, group, normalized, defaultID)
	if err != nil {
		_ = Rollback(tx)
		return nil, err
	}

	if err := gc.syncPolicyPool(ctx, res.ID, normalized); err != nil {
		_ = Rollback(tx)
		return nil, err
	}

	if err := Commit(tx); err != nil {
		return nil, err
	}

	return res, nil
}

func (c *groupClient) upsertGroup(ctx context.Context, group *ent.Group, normalized []int, defaultID int) (*ent.Group, error) {
	if group.ID == 0 {
		stm := c.client.Group.Create().
			SetName(group.Name).
			SetMaxStorage(group.MaxStorage).
			SetSpeedLimit(group.SpeedLimit).
			SetPermissions(group.Permissions).
			SetSettings(group.Settings)

		if defaultID > 0 {
			stm.SetStoragePolicyID(defaultID)
		}

		return stm.Save(ctx)
	}

	stm := c.client.Group.UpdateOne(group).
		SetName(group.Name).
		SetMaxStorage(group.MaxStorage).
		SetSpeedLimit(group.SpeedLimit).
		SetPermissions(group.Permissions).
		SetSettings(group.Settings).
		ClearStoragePolicies()

	if defaultID > 0 {
		stm.SetStoragePolicyID(defaultID)
	}

	return stm.Save(ctx)
}

// syncPolicyPool makes group_storage_policies match the normalized pool: rows that
// are no longer selected are removed and missing rows are inserted. It is idempotent.
func (c *groupClient) syncPolicyPool(ctx context.Context, groupID int, policyIDs []int) error {
	if groupID == AnonymousGroupID {
		policyIDs = nil
	}

	query := c.client.GroupStoragePolicy.Delete().Where(groupstoragepolicy.GroupID(groupID))
	if len(policyIDs) > 0 {
		query = query.Where(groupstoragepolicy.PolicyIDNotIn(policyIDs...))
	}
	// Join rows are replaced, not versioned: hard delete them so the unique
	// (group_id, policy_id) index never collides with a soft-deleted row.
	if _, err := query.Exec(schema.SkipSoftDelete(ctx)); err != nil {
		return fmt.Errorf("failed to clear group storage policy pool: %w", err)
	}

	if len(policyIDs) == 0 {
		return nil
	}

	existing, err := c.client.GroupStoragePolicy.Query().
		Where(groupstoragepolicy.GroupID(groupID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query group storage policy pool: %w", err)
	}

	present := make(map[int]struct{}, len(existing))
	for _, row := range existing {
		present[row.PolicyID] = struct{}{}
	}

	for _, id := range policyIDs {
		if _, ok := present[id]; ok {
			continue
		}
		if _, err := c.client.GroupStoragePolicy.Create().
			SetGroupID(groupID).
			SetPolicyID(id).
			Save(ctx); err != nil {
			return fmt.Errorf("failed to add storage policy to group pool: %w", err)
		}
	}

	return nil
}

func (c *groupClient) StoragePolicyPool(ctx context.Context, groupID int) ([]*ent.StoragePolicy, error) {
	return withStoragePolicyEagerLoading(ctx, c.client.StoragePolicy.Query().
		Where(storagepolicy.HasGroupPolicyPoolWith(groupstoragepolicy.GroupID(groupID))).
		Order(ent.Asc(storagepolicy.FieldID))).
		All(ctx)
}

func (c *groupClient) CountGroupsUsingPolicy(ctx context.Context, policyID int) (int, error) {
	return c.client.GroupStoragePolicy.Query().
		Where(groupstoragepolicy.PolicyID(policyID)).
		Count(ctx)
}

func (c *groupClient) Delete(ctx context.Context, id int) error {
	if _, err := c.client.GroupStoragePolicy.Delete().
		Where(groupstoragepolicy.GroupID(id)).
		Exec(schema.SkipSoftDelete(ctx)); err != nil {
		return fmt.Errorf("failed to delete group storage policy pool: %w", err)
	}

	if err := c.client.Group.DeleteOneID(id).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	return nil
}

func (c *groupClient) ListGroups(ctx context.Context, args *ListGroupParameters) (*ListGroupResult, error) {
	query := withGroupEagerLoading(ctx, c.client.Group.Query())
	pageSize := capPageSize(c.maxSQlParam, args.PageSize, 10)
	queryWithoutOrder := query.Clone()
	query.Order(getGroupOrderOption(args)...)

	// Count total items
	total, err := queryWithoutOrder.Clone().
		Count(ctx)
	if err != nil {
		return nil, err
	}

	groups, err := query.Clone().
		Limit(pageSize).
		Offset(args.Page * pageSize).
		All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListGroupResult{
		Groups: groups,
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   pageSize,
		},
	}, nil
}

func (c *groupClient) GetByID(ctx context.Context, id int) (*ent.Group, error) {
	return withGroupEagerLoading(ctx, c.client.Group.Query().Where(group.ID(id))).First(ctx)
}

func getGroupOrderOption(args *ListGroupParameters) []group.OrderOption {
	orderTerm := getOrderTerm(args.Order)
	switch args.OrderBy {
	case group.FieldName:
		return []group.OrderOption{group.ByName(orderTerm), group.ByID(orderTerm)}
	case group.FieldMaxStorage:
		return []group.OrderOption{group.ByMaxStorage(orderTerm), group.ByID(orderTerm)}
	default:
		return []group.OrderOption{group.ByID(orderTerm)}
	}
}

func withGroupEagerLoading(ctx context.Context, q *ent.GroupQuery) *ent.GroupQuery {
	if _, ok := ctx.Value(LoadGroupPolicy{}).(bool); ok {
		q.WithStoragePolicies(func(spq *ent.StoragePolicyQuery) {
			withStoragePolicyEagerLoading(ctx, spq)
		})
	}
	return q
}
