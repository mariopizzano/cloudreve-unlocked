package inventory

import (
	"context"
	"encoding/gob"
	"fmt"
	"sort"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/groupstoragepolicy"
	"github.com/cloudreve/Cloudreve/v4/ent/storagepolicy"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
)

const (
	// StoragePolicyCacheKey is the cache key of storage policy.
	StoragePolicyCacheKey = "storage_policy_"
	// LoadBalanceCounterKeyPrefix is the KV key prefix of the persistent round-robin
	// counter of a load_balance policy.
	LoadBalanceCounterKeyPrefix = "load_balance_counter_"
)

// LoadBalanceCounterKey returns the KV key of the persistent round-robin counter for the
// given load_balance policy.
func LoadBalanceCounterKey(policyID int) string {
	return LoadBalanceCounterKeyPrefix + strconv.Itoa(policyID)
}

func init() {
	gob.Register(ent.StoragePolicy{})
	gob.Register([]ent.StoragePolicy{})
}

type (
	LoadStoragePolicyGroup struct{}
	SkipStoragePolicyCache struct{}

	StoragePolicyClient interface {
		TxOperator
		// GetByGroup returns the default storage policy of the group.
		GetByGroup(ctx context.Context, group *ent.Group) (*ent.StoragePolicy, error)
		// GetAvailableByGroup returns the storage policies assigned to the group pool.
		// When no pool is configured it falls back to the legacy storage_policy_id.
		GetAvailableByGroup(ctx context.Context, group *ent.Group) ([]*ent.StoragePolicy, error)
		// GetDefaultByGroup returns the default policy of the group, guaranteed to be
		// part of the available pool when a pool is configured.
		GetDefaultByGroup(ctx context.Context, group *ent.Group) (*ent.StoragePolicy, error)
		// IsPolicyAvailableToGroup reports whether the policy is selectable by the group.
		IsPolicyAvailableToGroup(ctx context.Context, group *ent.Group, policyID int) (bool, error)
		// GetSystemDefault returns the global fallback policy (policy 1 when available).
		GetSystemDefault(ctx context.Context) (*ent.StoragePolicy, error)
		// GetPolicyByID returns the storage policy by id.
		GetPolicyByID(ctx context.Context, id int) (*ent.StoragePolicy, error)
		// UpdateAccessKey updates the access key of the storage policy. It also clear related cache in KV.
		UpdateAccessKey(ctx context.Context, policy *ent.StoragePolicy, token string) error
		// ListPolicyByType returns the storage policies by type.
		ListPolicyByType(ctx context.Context, t types.PolicyType) ([]*ent.StoragePolicy, error)
		// ListPolicies returns the storage policies with pagination.
		ListPolicies(ctx context.Context, args *ListPolicyParameters) (*ListPolicyResult, error)
		// ResolveUploadPolicy returns a concrete storage policy to be used for a new
		// upload. If the given policy is not a load_balance policy it is returned as-is.
		// Otherwise a member policy is selected by the configured strategy. When advance
		// is true the persistent round-robin counter is advanced.
		ResolveUploadPolicy(ctx context.Context, policy *ent.StoragePolicy, advance bool) (*ent.StoragePolicy, error)
		// Upsert upserts the storage policy.
		Upsert(ctx context.Context, policy *ent.StoragePolicy) (*ent.StoragePolicy, error)
		// Delete deletes the storage policy.
		Delete(ctx context.Context, policy *ent.StoragePolicy) error
	}

	ListPolicyParameters struct {
		*PaginationArgs
		Type types.PolicyType
	}

	ListPolicyResult struct {
		*PaginationResults
		Policies []*ent.StoragePolicy
	}
)

// NewStoragePolicyClient returns a new StoragePolicyClient.
func NewStoragePolicyClient(client *ent.Client, cache cache.Driver) StoragePolicyClient {
	return &storagePolicyClient{client: client, cache: cache}
}

type storagePolicyClient struct {
	client *ent.Client
	cache  cache.Driver
}

func (c *storagePolicyClient) SetClient(newClient *ent.Client) TxOperator {
	return &storagePolicyClient{client: newClient, cache: c.cache}
}

func (c *storagePolicyClient) GetClient() *ent.Client {
	return c.client
}

func (c *storagePolicyClient) Delete(ctx context.Context, policy *ent.StoragePolicy) error {
	if err := c.client.StoragePolicy.DeleteOne(policy).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete storage policy: %w", err)
	}

	// Clear cache
	if err := c.cache.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil {
		return fmt.Errorf("failed to clear storage policy cache: %w", err)
	}
	return nil
}

func (c *storagePolicyClient) Upsert(ctx context.Context, policy *ent.StoragePolicy) (*ent.StoragePolicy, error) {
	var nodeId *int
	if policy.NodeID != 0 {
		nodeId = &policy.NodeID
	}
	if policy.ID == 0 {
		p, err := c.client.StoragePolicy.
			Create().
			SetName(policy.Name).
			SetType(policy.Type).
			SetServer(policy.Server).
			SetBucketName(policy.BucketName).
			SetIsPrivate(policy.IsPrivate).
			SetAccessKey(policy.AccessKey).
			SetSecretKey(policy.SecretKey).
			SetMaxSize(policy.MaxSize).
			SetDirNameRule(policy.DirNameRule).
			SetFileNameRule(policy.FileNameRule).
			SetSettings(policy.Settings).
			SetNillableNodeID(nodeId).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage policy: %w", err)
		}
		return p, nil
	}

	updateQuery := c.client.StoragePolicy.UpdateOne(policy).
		SetName(policy.Name).
		SetType(policy.Type).
		SetServer(policy.Server).
		SetBucketName(policy.BucketName).
		SetIsPrivate(policy.IsPrivate).
		SetSecretKey(policy.SecretKey).
		SetMaxSize(policy.MaxSize).
		SetDirNameRule(policy.DirNameRule).
		SetFileNameRule(policy.FileNameRule).
		SetSettings(policy.Settings).
		SetNillableNodeID(nodeId)

	if policy.Type != types.PolicyTypeOd {
		updateQuery.SetAccessKey(policy.AccessKey)
	}

	p, err := updateQuery.Save(ctx)

	// Clear cache
	if err := c.cache.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil {
		return nil, fmt.Errorf("failed to clear storage policy cache: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update storage policy: %w", err)
	}
	return p, nil

}

func (c *storagePolicyClient) GetByGroup(ctx context.Context, group *ent.Group) (*ent.StoragePolicy, error) {
	val, skipCache := ctx.Value(SkipStoragePolicyCache{}).(bool)
	skipCache = skipCache && val

	res, err := withStoragePolicyEagerLoading(ctx, c.client.Group.QueryStoragePolicies(group)).WithNode().First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get storage policies: %w", err)
	}

	return res, nil
}

// GetAvailableByGroup returns the policies assigned to the group through the
// explicit group_storage_policies join. If the group has no rows in the join (for
// example on a database that has not been backfilled yet), the legacy
// storage_policy_id is used as a single-element pool.
func (c *storagePolicyClient) GetAvailableByGroup(ctx context.Context, group *ent.Group) ([]*ent.StoragePolicy, error) {
	if group == nil {
		return nil, nil
	}

	policies, err := withStoragePolicyEagerLoading(ctx, c.client.StoragePolicy.Query().
		Where(storagepolicy.HasGroupPolicyPoolWith(groupstoragepolicy.GroupID(group.ID))).
		Order(ent.Asc(storagepolicy.FieldID))).
		WithNode().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get group storage policies: %w", err)
	}

	if len(policies) > 0 {
		return policies, nil
	}

	if group.StoragePolicyID > 0 {
		legacy, err := c.GetPolicyByID(ctx, group.StoragePolicyID)
		if err != nil {
			return nil, err
		}
		return []*ent.StoragePolicy{legacy}, nil
	}

	return nil, nil
}

// GetDefaultByGroup returns the group's default policy. The default is always part
// of the available pool; when it is missing the first pool entry is used.
func (c *storagePolicyClient) GetDefaultByGroup(ctx context.Context, group *ent.Group) (*ent.StoragePolicy, error) {
	policies, err := c.GetAvailableByGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}

	if group.StoragePolicyID > 0 {
		for _, policy := range policies {
			if policy.ID == group.StoragePolicyID {
				return policy, nil
			}
		}
	}

	return policies[0], nil
}

// IsPolicyAvailableToGroup reports whether the given policy belongs to the group's
// pool. Groups without an explicit pool keep the legacy single-policy behaviour.
func (c *storagePolicyClient) IsPolicyAvailableToGroup(ctx context.Context, group *ent.Group, policyID int) (bool, error) {
	if group == nil || policyID <= 0 {
		return false, nil
	}

	policies, err := c.GetAvailableByGroup(ctx, group)
	if err != nil {
		return false, err
	}

	for _, policy := range policies {
		if policy.ID == policyID {
			return true, nil
		}
	}

	return false, nil
}

// GetSystemDefault returns the default storage policy used when a group has no
// usable policy: policy ID 1 when it exists, otherwise the oldest policy.
func (c *storagePolicyClient) GetSystemDefault(ctx context.Context) (*ent.StoragePolicy, error) {
	policy, err := c.GetPolicyByID(ctx, 1)
	if err == nil {
		return policy, nil
	}

	policies, listErr := c.client.StoragePolicy.Query().
		Order(ent.Asc(storagepolicy.FieldID)).
		First(ctx)
	if listErr != nil {
		return nil, fmt.Errorf("failed to get system default storage policy: %w", listErr)
	}

	return policies, nil
}

// GetPolicyByID returns the storage policy by id.
func (c *storagePolicyClient) GetPolicyByID(ctx context.Context, id int) (*ent.StoragePolicy, error) {
	val, skipCache := ctx.Value(SkipStoragePolicyCache{}).(bool)
	skipCache = skipCache && val

	// Try to read from cache
	if c.cache != nil && !skipCache {
		if res, ok := c.cache.Get(StoragePolicyCacheKey + strconv.Itoa(id)); ok {
			cached := res.(ent.StoragePolicy)
			return &cached, nil
		}
	}

	res, err := withStoragePolicyEagerLoading(ctx, c.client.StoragePolicy.Query().Where(storagepolicy.ID(id))).WithNode().First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get storage policy: %w", err)
	}

	// Write to cache
	if c.cache != nil && !skipCache {
		_ = c.cache.Set(StoragePolicyCacheKey+strconv.Itoa(id), *res, -1)
	}

	return res, nil
}

// LoadBalanceMembers returns the de-duplicated, normalized member list configured on a
// load_balance policy, preserving the configured order. Invalid policy IDs are dropped
// and non-positive weights are normalized to the default weight.
func LoadBalanceMembers(policy *ent.StoragePolicy) []types.LoadBalanceMember {
	if policy == nil || policy.Settings == nil {
		return nil
	}

	seen := make(map[int]struct{}, len(policy.Settings.Members))
	members := make([]types.LoadBalanceMember, 0, len(policy.Settings.Members))
	for _, member := range policy.Settings.Members {
		if member.PolicyID <= 0 {
			continue
		}
		if _, ok := seen[member.PolicyID]; ok {
			continue
		}
		seen[member.PolicyID] = struct{}{}

		weight := member.Weight
		if weight <= 0 {
			weight = types.LoadBalanceDefaultWeight
		}
		if weight > types.LoadBalanceMaxWeight {
			weight = types.LoadBalanceMaxWeight
		}

		members = append(members, types.LoadBalanceMember{
			PolicyID: member.PolicyID,
			Weight:   weight,
		})
	}

	return members
}

// BuildLoadBalanceCycle returns a deterministic, weighted cyclic schedule of member
// policy IDs. Each member appears Weight times over a cycle of sum(weights) entries.
// Occurrences are evenly interleaved using fractional slot positions, so 3:1 becomes
// A, B, A, A instead of a bursty A, A, A, B. Ties prefer the member with the smaller
// weight and then the configured order, keeping the result stable.
func BuildLoadBalanceCycle(members []types.LoadBalanceMember) []int {
	type slot struct {
		position float64
		weight   int
		order    int
		policyID int
	}

	total := 0
	for _, member := range members {
		total += member.Weight
	}
	if total <= 0 {
		return nil
	}

	slots := make([]slot, 0, total)
	order := 0
	for _, member := range members {
		for k := 0; k < member.Weight; k++ {
			slots = append(slots, slot{
				position: (float64(k) + 0.5) / float64(member.Weight),
				weight:   member.Weight,
				order:    order,
				policyID: member.PolicyID,
			})
			order++
		}
	}

	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].position == slots[j].position {
			if slots[i].weight == slots[j].weight {
				return slots[i].order < slots[j].order
			}
			return slots[i].weight < slots[j].weight
		}
		return slots[i].position < slots[j].position
	})

	cycle := make([]int, len(slots))
	for i, s := range slots {
		cycle[i] = s.policyID
	}

	return cycle
}

// nextLoadBalanceIndex advances the persistent round-robin counter of a load_balance
// policy and returns the index of the member to use next. It is exported through
// ResolveUploadPolicy but kept separate so the selection logic can be unit tested.
func nextLoadBalanceIndex(kv cache.Driver, policyID int, memberCount int) (int, error) {
	if memberCount <= 0 {
		return 0, fmt.Errorf("load balance policy has no member policies")
	}
	if kv == nil {
		return 0, nil
	}

	counter, err := kv.Incr(LoadBalanceCounterKey(policyID), 0)
	if err != nil {
		return 0, fmt.Errorf("failed to advance load balance counter: %w", err)
	}
	if counter <= 0 {
		return 0, nil
	}

	return int((counter - 1) % int64(memberCount)), nil
}

// ResolveUploadPolicy implements StoragePolicyClient.
func (c *storagePolicyClient) ResolveUploadPolicy(ctx context.Context, policy *ent.StoragePolicy, advance bool) (*ent.StoragePolicy, error) {
	if policy == nil || policy.Type != types.PolicyTypeLoadBalance {
		return policy, nil
	}

	ids := LoadBalanceMembers(policy)
	cycle := BuildLoadBalanceCycle(ids)
	if len(cycle) == 0 {
		return nil, fmt.Errorf("load balance policy %d has no member policies", policy.ID)
	}

	start := 0
	if advance {
		var err error
		start, err = nextLoadBalanceIndex(c.cache, policy.ID, len(cycle))
		if err != nil {
			return nil, err
		}
	}

	// Try each member starting from the selected index so that a missing or deleted
	// member does not break the whole policy.
	for offset := 0; offset < len(cycle); offset++ {
		id := cycle[(start+offset)%len(cycle)]
		member, err := c.GetPolicyByID(ctx, id)
		if err != nil || member == nil || member.Type == types.PolicyTypeLoadBalance {
			continue
		}
		return member, nil
	}

	return nil, fmt.Errorf("load balance policy %d has no valid member policies", policy.ID)
}

func (c *storagePolicyClient) ListPolicyByType(ctx context.Context, t types.PolicyType) ([]*ent.StoragePolicy, error) {
	policies, err := c.client.StoragePolicy.Query().Where(storagepolicy.TypeEQ(string(t))).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage policies: %w", err)
	}

	return policies, nil
}

func (c *storagePolicyClient) UpdateAccessKey(ctx context.Context, policy *ent.StoragePolicy, token string) error {
	_, err := c.client.StoragePolicy.UpdateOne(policy).SetAccessKey(token).Save(ctx)
	if err != nil {
		return fmt.Errorf("faield to update access key in DB: %w", err)
	}

	// Clear cache
	if err := c.cache.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil {
		return fmt.Errorf("failed to clear storage policy cache: %w", err)
	}

	return nil
}

func (c *storagePolicyClient) ListPolicies(ctx context.Context, args *ListPolicyParameters) (*ListPolicyResult, error) {
	query := c.client.StoragePolicy.Query().WithNode()
	if args.Type != "" {
		query = query.Where(storagepolicy.TypeEQ(string(args.Type)))
	}
	query.Order(getStoragePolicyOrderOption(args)...)

	// Count total items
	total, err := query.Clone().
		Count(ctx)
	if err != nil {
		return nil, err
	}

	policies, err := withStoragePolicyEagerLoading(ctx, query).Limit(args.PageSize).Offset(args.Page * args.PageSize).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListPolicyResult{
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   args.PageSize,
		},
		Policies: policies,
	}, nil
}

func getStoragePolicyOrderOption(args *ListPolicyParameters) []storagepolicy.OrderOption {
	orderTerm := getOrderTerm(args.Order)
	switch args.OrderBy {
	case storagepolicy.FieldUpdatedAt:
		return []storagepolicy.OrderOption{storagepolicy.ByUpdatedAt(orderTerm), storagepolicy.ByID(orderTerm)}
	default:
		return []storagepolicy.OrderOption{storagepolicy.ByID(orderTerm)}
	}
}

func withStoragePolicyEagerLoading(ctx context.Context, query *ent.StoragePolicyQuery) *ent.StoragePolicyQuery {
	if _, ok := ctx.Value(LoadStoragePolicyGroup{}).(bool); ok {
		query = query.WithGroups(func(gq *ent.GroupQuery) {
			withGroupEagerLoading(ctx, gq)
		})
	}
	return query
}
