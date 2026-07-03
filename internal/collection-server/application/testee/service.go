package testee

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/iamport"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Service 受试者服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 转换 REST DTO 到 gRPC 请求
// 2. 调用 apiserver 的 Actor gRPC 服务
// 3. 转换 gRPC 响应到 REST DTO
type Service struct {
	actorClient        ActorPort
	profileLinkService iamport.OrgDefaults
	profileService     iamport.ProfileCreator
}

// NewService 创建受试者服务
func NewService(actorClient ActorPort, profileLinkService iamport.OrgDefaults, profileService iamport.ProfileCreator) *Service {
	return &Service{
		actorClient:        actorClient,
		profileLinkService: profileLinkService,
		profileService:     profileService,
	}
}

// CreateTestee 创建受试者
func (s *Service) CreateTestee(ctx context.Context, userID uint64, req *CreateTesteeRequest) (*TesteeResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Creating testee: name=%s, userID=%d", req.Name, userID)

	l.Infow("开始创建受试者",
		"action", "create_testee",
		"name", req.Name,
		"iam_user_id", userID,
	)
	if userID == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	if s.profileService == nil || !s.profileService.IsEnabled() {
		return nil, fmt.Errorf("iam profile service not enabled")
	}

	// 从 IAM 获取默认机构ID（单租户场景）
	orgID := s.defaultOrgID()
	iamUserID := strconv.FormatUint(userID, 10)
	profile, err := s.profileService.CreateProfile(ctx, iamport.CreateProfileInput{
		UserID:       iamUserID,
		LegalName:    req.Name,
		Gender:       req.Gender,
		DOB:          birthdayString(req.Birthday),
		IDCardNumber: req.IDCardNumber,
		Relation:     req.Relation,
	})
	if err != nil {
		l.Errorw("创建 IAM Profile 失败",
			"action", "create_testee",
			"name", req.Name,
			"iam_user_id", iamUserID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	// 调用 gRPC 服务
	l.Debugw("调用 gRPC 服务创建受试者",
		"org_id", orgID,
		"iam_user_id", iamUserID,
		"iam_profile_id", profile.ProfileID,
	)

	result, err := s.actorClient.CreateTestee(ctx, CreateTesteeInput{
		OrgID:        orgID,
		IAMUserID:    iamUserID,
		IAMProfileID: profile.ProfileID,
		Name:         req.Name,
		Gender:       req.Gender,
		Birthday:     req.Birthday,
		Tags:         req.Tags,
		Source:       req.Source,
		IsKeyFocus:   req.IsKeyFocus,
	})
	if err != nil {
		log.Errorf("Failed to create testee via gRPC: %v", err)
		l.Errorw("创建受试者失败",
			"action", "create_testee",
			"name", req.Name,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Infow("创建受试者成功",
		"action", "create_testee",
		"result", "success",
		"testee_id", result.ID,
		"iam_profile_id", profile.ProfileID,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

// GetTestee 获取受试者详情
func (s *Service) GetTestee(ctx context.Context, testeeID uint64) (*TesteeResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting testee: testeeID=%d", testeeID)

	l.Debugw("获取受试者详情",
		"action", "get_testee",
		"testee_id", testeeID,
	)

	result, err := s.actorClient.GetTestee(ctx, testeeID)
	if err != nil {
		log.Errorf("Failed to get testee via gRPC: %v", err)
		l.Errorw("获取受试者失败",
			"action", "get_testee",
			"testee_id", testeeID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Debugw("获取受试者成功",
		"action", "get_testee",
		"testee_id", testeeID,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

// GetTesteeCareContext 获取受试者照护上下文
func (s *Service) GetTesteeCareContext(ctx context.Context, testeeID uint64) (*TesteeCareContextResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取受试者照护上下文",
		"action", "get_testee_care_context",
		"testee_id", testeeID,
	)

	result, err := s.actorClient.GetTesteeCareContext(ctx, testeeID)
	if err != nil {
		l.Errorw("获取受试者照护上下文失败",
			"action", "get_testee_care_context",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return nil, err
	}

	l.Debugw("获取受试者照护上下文成功",
		"action", "get_testee_care_context",
		"testee_id", testeeID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	if result == nil {
		return &TesteeCareContextResponse{}, nil
	}
	return result, nil
}

// UpdateTestee 更新受试者信息
func (s *Service) UpdateTestee(ctx context.Context, testeeID uint64, req *UpdateTesteeRequest) (*TesteeResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Updating testee: testeeID=%d", testeeID)

	l.Infow("开始更新受试者",
		"action", "update_testee",
		"testee_id", testeeID,
		"name", req.Name,
	)

	result, err := s.actorClient.UpdateTestee(ctx, testeeID, req)
	if err != nil {
		log.Errorf("Failed to update testee via gRPC: %v", err)
		l.Errorw("更新受试者失败",
			"action", "update_testee",
			"testee_id", testeeID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Infow("更新受试者成功",
		"action", "update_testee",
		"result", "success",
		"testee_id", testeeID,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

// ListMyTestees 查询当前用户的受试者列表
// profileIDs 是当前用户在 IAM 系统中拥有 active ProfileLink 的 ProfileID 列表
func (s *Service) ListMyTestees(ctx context.Context, profileIDs []uint64, req *ListTesteesRequest) (*ListTesteesResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Listing my testees: profileIDs=%v, offset=%d, limit=%d", profileIDs, req.Offset, req.Limit)

	l.Debugw("查询受试者列表",
		"action", "list_my_testees",
		"profile_ids_count", len(profileIDs),
		"offset", req.Offset,
		"limit", req.Limit,
	)

	// 设置默认分页参数
	offset := req.Offset
	limit := req.Limit
	if limit == 0 {
		limit = 20 // 默认每页20条
	}

	l.Debugw("开始从 gRPC 服务查询受试者列表",
		"offset", offset,
		"limit", limit,
	)

	testees, total, err := s.actorClient.ListTesteesByUser(ctx, profileIDs, offset, limit)
	if err != nil {
		log.Errorf("Failed to list my testees via gRPC: %v", err)
		l.Errorw("查询受试者列表失败",
			"action", "list_my_testees",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Debugw("查询受试者列表成功",
		"action", "list_my_testees",
		"result", "success",
		"total_count", total,
		"page_count", len(testees),
		"duration_ms", duration.Milliseconds(),
	)

	return &ListTesteesResponse{
		Items: testees,
		Total: total,
	}, nil
}

// TesteeExists 检查受试者是否存在
func (s *Service) TesteeExists(ctx context.Context, iamProfileID string) (*TesteeExistsResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	// 转换 string ID 为 uint64
	profileIDUint, err := strconv.ParseUint(iamProfileID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid iam_profile_id format: %w", err)
	}

	// 从 IAM 获取默认机构ID（单租户场景）
	orgID := s.defaultOrgID()
	log.Infof("Checking testee existence: orgID=%d, iamProfileID=%s", orgID, iamProfileID)

	l.Debugw("检查受试者存在性",
		"action", "testee_exists",
		"org_id", orgID,
		"iam_profile_id", iamProfileID,
	)

	exists, testeeID, err := s.actorClient.TesteeExists(ctx, orgID, profileIDUint)
	if err != nil {
		log.Errorf("Failed to check testee existence via gRPC: %v", err)
		l.Errorw("检查受试者存在性失败",
			"action", "testee_exists",
			"iam_profile_id", iamProfileID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Debugw("检查受试者存在性完成",
		"action", "testee_exists",
		"iam_profile_id", iamProfileID,
		"exists", exists,
		"testee_id", testeeID,
		"duration_ms", duration.Milliseconds(),
	)

	// 转换 TesteeID，如果为 0 则转换为空字符串
	testeeIDStr := ""
	if testeeID != 0 {
		testeeIDStr = strconv.FormatUint(testeeID, 10)
	}

	return &TesteeExistsResponse{
		Exists:   exists,
		TesteeID: testeeIDStr,
	}, nil
}

func birthdayString(birthday *meta.Birthday) string {
	if birthday == nil {
		return ""
	}
	return birthday.String()
}

func (s *Service) defaultOrgID() uint64 {
	if s.profileLinkService == nil {
		return 1
	}
	return s.profileLinkService.GetDefaultOrgID()
}
