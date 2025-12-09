package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
)

// Service 受试者服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 转换 REST DTO 到 gRPC 请求
// 2. 调用 apiserver 的 Actor gRPC 服务
// 3. 转换 gRPC 响应到 REST DTO
type Service struct {
	actorClient         *grpcclient.ActorClient
	guardianshipService *iam.GuardianshipService
}

// NewService 创建受试者服务
func NewService(actorClient *grpcclient.ActorClient, guardianshipService *iam.GuardianshipService) *Service {
	return &Service{
		actorClient:         actorClient,
		guardianshipService: guardianshipService,
	}
}

// CreateTestee 创建受试者
func (s *Service) CreateTestee(ctx context.Context, req *CreateTesteeRequest) (*TesteeResponse, error) {
	log.Infof("Creating testee: name=%s, iamChildID=%d", req.Name, req.IAMChildID)

	// 从 IAM 获取默认机构ID（单租户场景）
	orgID := s.guardianshipService.GetDefaultOrgID()

	// 调用 gRPC 服务
	result, err := s.actorClient.CreateTestee(ctx, &grpcclient.CreateTesteeRequest{
		OrgID:      orgID,
		IAMUserID:  req.IAMUserID,
		IAMChildID: req.IAMChildID,
		Name:       req.Name,
		Gender:     req.Gender,
		Birthday:   req.Birthday,
		Tags:       req.Tags,
		Source:     req.Source,
		IsKeyFocus: req.IsKeyFocus,
	})
	if err != nil {
		log.Errorf("Failed to create testee via gRPC: %v", err)
		return nil, err
	}

	return convertToTesteeResponse(result), nil
}

// GetTestee 获取受试者详情
func (s *Service) GetTestee(ctx context.Context, testeeID uint64) (*TesteeResponse, error) {
	log.Infof("Getting testee: testeeID=%d", testeeID)

	result, err := s.actorClient.GetTestee(ctx, testeeID)
	if err != nil {
		log.Errorf("Failed to get testee via gRPC: %v", err)
		return nil, err
	}

	return convertToTesteeResponse(result), nil
}

// UpdateTestee 更新受试者信息
func (s *Service) UpdateTestee(ctx context.Context, testeeID uint64, req *UpdateTesteeRequest) (*TesteeResponse, error) {
	log.Infof("Updating testee: testeeID=%d", testeeID)

	result, err := s.actorClient.UpdateTestee(ctx, &grpcclient.UpdateTesteeRequest{
		ID:         testeeID,
		Name:       req.Name,
		Gender:     req.Gender,
		Birthday:   req.Birthday,
		Tags:       req.Tags,
		IsKeyFocus: req.IsKeyFocus,
	})
	if err != nil {
		log.Errorf("Failed to update testee via gRPC: %v", err)
		return nil, err
	}

	return convertToTesteeResponse(result), nil
}

// ListMyTestees 查询当前用户的受试者列表
// childIDs 是当前用户（监护人）在 IAM 系统中的所有孩子ID列表
func (s *Service) ListMyTestees(ctx context.Context, childIDs []uint64, req *ListTesteesRequest) (*ListTesteesResponse, error) {
	log.Infof("Listing my testees: childIDs=%v, offset=%d, limit=%d", childIDs, req.Offset, req.Limit)

	// 设置默认分页参数
	offset := req.Offset
	limit := req.Limit
	if limit == 0 {
		limit = 20 // 默认每页20条
	}

	testees, total, err := s.actorClient.ListTesteesByUser(ctx, childIDs, offset, limit)
	if err != nil {
		log.Errorf("Failed to list my testees via gRPC: %v", err)
		return nil, err
	}

	items := make([]*TesteeResponse, 0, len(testees))
	for _, t := range testees {
		items = append(items, convertToTesteeResponse(t))
	}

	return &ListTesteesResponse{
		Items: items,
		Total: total,
	}, nil
}

// TesteeExists 检查受试者是否存在
func (s *Service) TesteeExists(ctx context.Context, iamChildID uint64) (*TesteeExistsResponse, error) {
	// 从 IAM 获取默认机构ID（单租户场景）
	orgID := s.guardianshipService.GetDefaultOrgID()
	log.Infof("Checking testee existence: orgID=%d, iamChildID=%d", orgID, iamChildID)

	exists, testeeID, err := s.actorClient.TesteeExists(ctx, orgID, iamChildID)
	if err != nil {
		log.Errorf("Failed to check testee existence via gRPC: %v", err)
		return nil, err
	}

	return &TesteeExistsResponse{
		Exists:   exists,
		TesteeID: testeeID,
	}, nil
}

// convertToTesteeResponse 转换 gRPC 响应为应用层 DTO
func convertToTesteeResponse(from *grpcclient.TesteeResponse) *TesteeResponse {
	if from == nil {
		return nil
	}

	resp := &TesteeResponse{
		ID:         from.ID,
		OrgID:      from.OrgID,
		IAMUserID:  from.IAMUserID,
		IAMChildID: from.IAMChildID,
		Name:       from.Name,
		Gender:     from.Gender,
		Birthday:   from.Birthday,
		Tags:       from.Tags,
		Source:     from.Source,
		IsKeyFocus: from.IsKeyFocus,
		CreatedAt:  from.CreatedAt,
		UpdatedAt:  from.UpdatedAt,
	}

	// 转换测评统计信息
	if from.AssessmentStats != nil {
		resp.AssessmentStats = &AssessmentStatsDTO{
			TotalCount:       from.AssessmentStats.TotalCount,
			LastAssessmentAt: from.AssessmentStats.LastAssessmentAt,
			LastRiskLevel:    from.AssessmentStats.LastRiskLevel,
		}
	}

	return resp
}
