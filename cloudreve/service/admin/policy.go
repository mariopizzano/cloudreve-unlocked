package admin

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/constants"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cluster/routes"
	"github.com/cloudreve/Cloudreve/v4/pkg/credmanager"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/cos"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/ks3"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/obs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/onedrive"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/oss"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/s3"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"

	"github.com/cloudreve/Cloudreve/v4/pkg/request"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

// PathTestService 本地路径测试服务
type PathTestService struct {
	Path string `json:"path" binding:"required"`
}

// SlaveTestService 从机测试服务
type SlaveTestService struct {
	Secret string `json:"secret" binding:"required"`
	Server string `json:"server" binding:"required"`
}

type (
	SlavePingParameterCtx struct{}
	// SlavePingService ping slave node
	SlavePingService struct {
		Callback string `json:"callback" binding:"required"`
	}
)

// AddPolicyService 存储策略添加服务
type AddPolicyService struct {
	//Policy model.Policy `json:"policy" binding:"required"`
}

// PolicyService 存储策略ID服务
type PolicyService struct {
	ID     uint   `uri:"id" json:"id" binding:"required"`
	Region string `json:"region"`
}

// Delete 删除存储策略
func (service *SingleStoragePolicyService) Delete(c *gin.Context) error {
	// 禁止删除默认策略
	if service.ID == 1 {
		return serializer.NewError(serializer.CodeDeleteDefaultPolicy, "", nil)
	}

	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	ctx := context.WithValue(c, inventory.LoadStoragePolicyGroup{}, true)
	ctx = context.WithValue(ctx, inventory.SkipStoragePolicyCache{}, true)
	policy, err := storagePolicyClient.GetPolicyByID(ctx, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodePolicyNotExist, "", err)
	}

	// If policy is used by groups, return error
	if len(policy.Edges.Groups) > 0 {
		return serializer.NewError(serializer.CodePolicyUsedByGroups, strconv.Itoa(len(policy.Edges.Groups)), nil)
	}

	// The policy can also be referenced only from a group's policy pool without being
	// its legacy default.
	poolUsage, err := dep.GroupClient().CountGroupsUsingPolicy(ctx, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to check group storage policy pool usage", err)
	}
	if poolUsage > 0 {
		return serializer.NewError(serializer.CodePolicyUsedByGroups, strconv.Itoa(poolUsage), nil)
	}

	used, err := dep.FileClient().IsStoragePolicyUsedByEntities(ctx, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to check if policy is used by entities", err)
	}

	if used {
		return serializer.NewError(serializer.CodePolicyUsedByFiles, "", nil)
	}

	// If policy is mounted on folders, return error
	used, err = dep.FileClient().IsStoragePolicyUsedByFolders(ctx, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to check if policy is mounted on folders", err)
	}

	if used {
		return serializer.NewError(serializer.CodePolicyUsedByFiles, "", nil)
	}

	// If this policy is a member of a load_balance policy, deleting it would leave the
	// virtual policy with a dangling member, so refuse the deletion.
	memberOf, err := findLoadBalancePolicyUsing(ctx, storagePolicyClient, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to check if policy is used by a load balance policy", err)
	}

	if memberOf != nil {
		return serializer.NewError(serializer.CodePolicyUsedByLoadBalance, memberOf.Name, nil)
	}

	err = storagePolicyClient.Delete(ctx, policy)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete policy", err)
	}

	_ = dep.KV().Delete(inventory.LoadBalanceCounterKeyPrefix, strconv.Itoa(service.ID))

	return nil
}

// findLoadBalancePolicyUsing returns the first load_balance policy that references the
// given concrete policy as a member, or nil if none does.
func findLoadBalancePolicyUsing(ctx context.Context, client inventory.StoragePolicyClient, policyID int) (*ent.StoragePolicy, error) {
	res, err := client.ListPolicyByType(ctx, types.PolicyTypeLoadBalance)
	if err != nil {
		return nil, err
	}

	for _, policy := range res {
		for _, member := range inventory.LoadBalanceMembers(policy) {
			if member.PolicyID == policyID {
				return policy, nil
			}
		}
	}

	return nil, nil
}

// validateLoadBalancePolicy validates and normalizes the member configuration of a
// load_balance policy. It is a no-op for other policy types.
func validateLoadBalancePolicy(ctx context.Context, client inventory.StoragePolicyClient, policy *ent.StoragePolicy) error {
	if policy.Type != types.PolicyTypeLoadBalance {
		return nil
	}

	if policy.Settings == nil {
		policy.Settings = &types.PolicySetting{}
	}

	ids := inventory.LoadBalanceMembers(policy)
	members := make(map[int]*ent.StoragePolicy, len(ids))
	for _, member := range ids {
		loaded, err := client.GetPolicyByID(ctx, member.PolicyID)
		if err != nil {
			return serializer.NewError(serializer.CodePolicyNotExist, fmt.Sprintf("Member policy %d does not exist", member.PolicyID), err)
		}
		members[member.PolicyID] = loaded
	}

	return validateLoadBalanceMembers(policy, members)
}

// validateLoadBalanceMembers validates an already loaded set of member policies and
// normalizes the policy settings in place.
func validateLoadBalanceMembers(policy *ent.StoragePolicy, members map[int]*ent.StoragePolicy) error {
	if policy.Settings == nil {
		policy.Settings = &types.PolicySetting{}
	}

	strategy := policy.Settings.LoadBalanceStrategy
	if strategy == "" {
		strategy = types.LoadBalanceStrategyRoundRobin
	}
	if strategy != types.LoadBalanceStrategyRoundRobin {
		return serializer.NewError(serializer.CodeParamErr, "Unsupported load balance strategy: "+strategy, nil)
	}
	policy.Settings.LoadBalanceStrategy = strategy

	normalized := inventory.LoadBalanceMembers(policy)
	if len(normalized) == 0 {
		return serializer.NewError(serializer.CodeParamErr, "A load balance policy requires at least one member policy", nil)
	}

	var (
		firstType      string
		firstRelay     bool
		firstChunkSize int64
	)
	for i, configured := range normalized {
		if configured.PolicyID == policy.ID {
			return serializer.NewError(serializer.CodeParamErr, "A load balance policy cannot reference itself", nil)
		}

		member, ok := members[configured.PolicyID]
		if !ok || member == nil {
			return serializer.NewError(serializer.CodePolicyNotExist, fmt.Sprintf("Member policy %d does not exist", configured.PolicyID), nil)
		}

		if member.Type == types.PolicyTypeLoadBalance {
			return serializer.NewError(serializer.CodeParamErr, "A load balance policy cannot include another load balance policy", nil)
		}

		// The client picks its uploader and chunking strategy from the policy returned
		// when listing a folder. Keep all members compatible with each other so that
		// whichever member is selected works with that choice.
		relay := member.Settings != nil && member.Settings.Relay
		chunkSize := int64(0)
		if member.Settings != nil {
			chunkSize = member.Settings.ChunkSize
		}
		if i == 0 {
			firstType, firstRelay, firstChunkSize = member.Type, relay, chunkSize
			continue
		}

		if member.Type != firstType {
			return serializer.NewError(serializer.CodeParamErr, "All member policies must use the same storage provider type", nil)
		}
		if relay != firstRelay {
			return serializer.NewError(serializer.CodeParamErr, "All member policies must use the same upload relay setting", nil)
		}
		if chunkSize != firstChunkSize {
			return serializer.NewError(serializer.CodeParamErr, "All member policies must use the same chunk size", nil)
		}
	}

	policy.Settings.Members = normalized
	return nil
}

// Test 从机响应ping
func (service *SlavePingService) Test(c *gin.Context) error {
	master, err := url.Parse(service.Callback)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Failed to parse callback url", err)
	}

	dep := dependency.FromContext(c)
	r := dep.RequestClient()
	res, err := r.Request(
		"GET",
		routes.MasterPingUrl(master).String(),
		nil,
		request.WithContext(c),
		request.WithLogger(logging.FromContext(c)),
		request.WithCorrelationID(),
		request.WithTimeout(time.Duration(10)*time.Second),
	).DecodeResponse()

	if err != nil {
		return serializer.NewError(serializer.CodeSlavePingMaster, err.Error(), nil)
	}

	version := constants.BackendVersion

	if strings.TrimSuffix(res.Data.(string), "-pro") != version {
		return serializer.NewError(serializer.CodeVersionMismatch, "Master: "+res.Data.(string)+", Slave: "+version, nil)
	}

	return nil
}

// Test 测试从机通信
func (service *SlaveTestService) Test() serializer.Response {
	//slave, err := url.Parse(service.Server)
	//if err != nil {
	//	return serializer.ParamErrDeprecated("Failed to parse slave node server URL: "+err.Error(), nil)
	//}
	//
	//controller, _ := url.Parse("/api/v3/slave/ping")
	//
	//// 请求正文
	//body := map[string]string{
	//	"callback": model.GetSiteURL().String(),
	//}
	//bodyByte, _ := json.Marshal(body)
	//
	//r := request.NewClientDeprecated()
	//res, err := r.Request(
	//	"POST",
	//	slave.ResolveReference(controller).String(),
	//	bytes.NewReader(bodyByte),
	//	request.WithTimeout(time.Duration(10)*time.Second),
	//	request.WithCredential(
	//		auth.HMACAuth{SecretKey: []byte(service.Secret)},
	//		int64(model.GetIntSetting("slave_api_timeout", 60)),
	//	),
	//).DecodeResponse()
	//if err != nil {
	//	return serializer.ParamErrDeprecated("Failed to connect to slave node: "+err.Error(), nil)
	//}
	//
	//if res.Code != 0 {
	//	return serializer.ParamErrDeprecated("Successfully connected to slave node, but slave returns: "+res.Msg, nil)
	//}

	return serializer.Response{}
}

// Test 测试本地路径
func (service *PathTestService) Test() serializer.Response {
	//policy := model.Policy{DirNameRule: service.Path}
	//path := policy.GeneratePath(1, "/My File")
	//path = filepath.Join(path, "test.txt")
	//file, err := util.CreatNestedFile(util.RelativePath(path))
	//if err != nil {
	//	return serializer.ParamErrDeprecated(fmt.Sprintf("Failed to create \"%s\": %s", path, err.Error()), nil)
	//}
	//
	//file.Close()
	//os.Remove(path)

	return serializer.Response{}
}

const (
	policyTypeCondition = "policy_type"
)

// Policies 列出存储策略
func (service *AdminListService) Policies(c *gin.Context) (*ListPolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	ctx := context.WithValue(c, inventory.LoadStoragePolicyGroup{}, true)
	res, err := storagePolicyClient.ListPolicies(ctx, &inventory.ListPolicyParameters{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     service.Page - 1,
			PageSize: service.PageSize,
			OrderBy:  service.OrderBy,
			Order:    inventory.OrderDirection(service.OrderDirection),
		},
		Type: types.PolicyType(service.Conditions[policyTypeCondition]),
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list policies", err)
	}

	return &ListPolicyResponse{
		Pagination: res.PaginationResults,
		Policies:   res.Policies,
	}, nil
}

type (
	SingleStoragePolicyService struct {
		ID int `uri:"id" json:"id" binding:"required"`
	}
	GetStoragePolicyParamCtx struct{}
)

const (
	countEntityQuery = "countEntity"
)

func (service *SingleStoragePolicyService) Get(c *gin.Context) (*GetStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	ctx := context.WithValue(c, inventory.LoadStoragePolicyGroup{}, true)
	ctx = context.WithValue(ctx, inventory.SkipStoragePolicyCache{}, true)
	policy, err := storagePolicyClient.GetPolicyByID(ctx, service.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get policy", err)
	}

	res := &GetStoragePolicyResponse{StoragePolicy: policy}
	if policy.Type == types.PolicyTypeLoadBalance {
		for _, configured := range inventory.LoadBalanceMembers(policy) {
			member, err := storagePolicyClient.GetPolicyByID(ctx, configured.PolicyID)
			if err != nil {
				continue
			}
			res.MemberPolicies = append(res.MemberPolicies, member)
		}
	}

	if c.Query(countEntityQuery) != "" {
		count, size, err := dep.FileClient().CountEntityByStoragePolicyID(ctx, service.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to count entities", err)
		}
		res.EntitiesCount = count
		res.EntitiesSize = size
	}

	return res, nil
}

type (
	CreateStoragePolicyService struct {
		Policy *ent.StoragePolicy `json:"policy" binding:"required"`
	}
	CreateStoragePolicyParamCtx struct{}
)

func (service *CreateStoragePolicyService) Create(c *gin.Context) (*GetStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	if service.Policy.Type == types.PolicyTypeLocal {
		service.Policy.DirNameRule = util.DataPath("uploads/{uid}/{path}")
	}

	if err := validateLoadBalancePolicy(c, storagePolicyClient, service.Policy); err != nil {
		return nil, err
	}

	service.Policy.ID = 0
	policy, err := storagePolicyClient.Upsert(c, service.Policy)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create policy", err)
	}

	s := SingleStoragePolicyService{ID: policy.ID}
	return s.Get(c)
}

type (
	UpdateStoragePolicyService struct {
		Policy *ent.StoragePolicy `json:"policy" binding:"required"`
	}
	UpdateStoragePolicyParamCtx struct{}
)

func (service *UpdateStoragePolicyService) Update(c *gin.Context) (*GetStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	id := c.Param("id")
	if id == "" {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID is required", nil)
	}
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid ID", err)
	}

	service.Policy.ID = idInt

	if service.Policy.Type == types.PolicyTypeLoadBalance {
		// A concrete policy cannot become a load_balance policy while it is still a
		// member of another load_balance policy, otherwise nesting would be introduced.
		memberOf, err := findLoadBalancePolicyUsing(c, storagePolicyClient, idInt)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to check load balance membership", err)
		}
		if memberOf != nil {
			return nil, serializer.NewError(serializer.CodeParamErr,
				fmt.Sprintf("Policy is a member of load balance policy %q and cannot become a load balance policy", memberOf.Name), nil)
		}
	}

	if err := validateLoadBalancePolicy(c, storagePolicyClient, service.Policy); err != nil {
		return nil, err
	}

	sc, tx, ctx, err := inventory.WithTx(c, storagePolicyClient)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create transaction", err)
	}

	_, err = sc.Upsert(ctx, service.Policy)
	if err != nil {
		_ = inventory.Rollback(tx)
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update policy", err)
	}

	if err := inventory.Commit(tx); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to commit transaction", err)
	}

	_ = dep.KV().Delete(manager.EntityUrlCacheKeyPrefix)

	// Member order or selection may have changed, so restart the round-robin sequence.
	_ = dep.KV().Delete(inventory.LoadBalanceCounterKeyPrefix, strconv.Itoa(idInt))

	s := SingleStoragePolicyService{ID: idInt}
	return s.Get(c)
}

type (
	CreateStoragePolicyCorsService struct {
		Policy *ent.StoragePolicy `json:"policy" binding:"required"`
	}
	CreateStoragePolicyCorsParamCtx struct{}
)

func (service *CreateStoragePolicyCorsService) Create(c *gin.Context) error {
	dep := dependency.FromContext(c)

	switch service.Policy.Type {
	case types.PolicyTypeOss:
		handler, err := oss.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create oss driver", err)
		}
		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil

	case types.PolicyTypeCos:
		handler, err := cos.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create cos driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil

	case types.PolicyTypeS3:
		handler, err := s3.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create s3 driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil

	case types.PolicyTypeKs3:
		handler, err := ks3.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create ks3 driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil
	case types.PolicyTypeObs:
		handler, err := obs.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create obs driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil
	default:
		return serializer.NewError(serializer.CodeParamErr, "Unsupported policy type", nil)
	}
}

type (
	GetOauthRedirectService struct {
		ID     int    `json:"id" binding:"required"`
		Secret string `json:"secret" binding:"required"`
		AppID  string `json:"app_id" binding:"required"`
	}
	GetOauthRedirectParamCtx struct{}
)

// GetOAuth 获取 OneDrive OAuth 地址
func (service *GetOauthRedirectService) GetOAuth(c *gin.Context) (string, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policy, err := storagePolicyClient.GetPolicyByID(c, service.ID)
	if err != nil || policy.Type != types.PolicyTypeOd {
		return "", serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	// Update to latest redirect url
	policy.Settings.OauthRedirect = routes.MasterPolicyOAuthCallback(dep.SettingProvider().SiteURL(c)).String()
	policy.SecretKey = service.Secret
	policy.BucketName = service.AppID
	policy, err = storagePolicyClient.Upsert(c, policy)
	if err != nil {
		return "", serializer.NewError(serializer.CodeDBError, "Failed to update policy", err)
	}

	client := onedrive.NewClient(policy, dep.RequestClient(), dep.CredManager(), dep.Logger(), dep.SettingProvider(), 0)
	redirect := client.OAuthURL(context.Background(), []string{
		"offline_access",
		"files.readwrite.all",
	})

	return redirect, nil
}

func GetPolicyOAuthURL(c *gin.Context) string {
	dep := dependency.FromContext(c)
	return routes.MasterPolicyOAuthCallback(dep.SettingProvider().SiteURL(c)).String()
}

// GetOauthCredentialStatus returns last refresh time of oauth credential
func (service *SingleStoragePolicyService) GetOauthCredentialStatus(c *gin.Context) (*OauthCredentialStatus, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policy, err := storagePolicyClient.GetPolicyByID(c, service.ID)
	if err != nil || policy.Type != types.PolicyTypeOd {
		return nil, serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	if policy.AccessKey == "" {
		return &OauthCredentialStatus{Valid: false}, nil
	}

	token, err := dep.CredManager().Obtain(c, onedrive.CredentialKey(policy.ID))
	if err != nil {
		if errors.Is(err, credmanager.ErrNotFound) {
			return &OauthCredentialStatus{Valid: false}, nil
		}

		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get credential", err)
	}

	return &OauthCredentialStatus{Valid: true, LastRefreshTime: token.RefreshedAt()}, nil
}

type (
	FinishOauthCallbackService struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
	}
	FinishOauthCallbackParamCtx struct{}
)

func (service *FinishOauthCallbackService) Finish(c *gin.Context) error {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policyId, err := strconv.Atoi(service.State)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid state", err)
	}

	policy, err := storagePolicyClient.GetPolicyByID(c, policyId)
	if err != nil {
		return serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	if policy.Type != types.PolicyTypeOd {
		return serializer.NewError(serializer.CodeParamErr, "Invalid policy type", nil)
	}

	client := onedrive.NewClient(policy, dep.RequestClient(), dep.CredManager(), dep.Logger(), dep.SettingProvider(), 0)
	credential, err := client.ObtainToken(c, onedrive.WithCode(service.Code))
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Failed to obtain token: "+err.Error(), err)
	}

	credManager := dep.CredManager()
	err = credManager.Upsert(c, credential)
	if err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to upsert credential", err)
	}

	_, err = credManager.Obtain(c, onedrive.CredentialKey(policy.ID))
	if err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to obtain credential", err)
	}

	return nil
}

func (service *SingleStoragePolicyService) GetSharePointDriverRoot(c *gin.Context) (string, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policy, err := storagePolicyClient.GetPolicyByID(c, service.ID)
	if err != nil {
		return "", serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	if policy.Type != types.PolicyTypeOd {
		return "", serializer.NewError(serializer.CodeParamErr, "Invalid policy type", nil)
	}

	client := onedrive.NewClient(policy, dep.RequestClient(), dep.CredManager(), dep.Logger(), dep.SettingProvider(), 0)
	root, err := client.GetSiteIDByURL(c, c.Query("url"))
	if err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to get site id", err)
	}

	return fmt.Sprintf("sites/%s/drive", root), nil
}
