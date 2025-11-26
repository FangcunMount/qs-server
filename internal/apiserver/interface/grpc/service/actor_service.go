package service

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/FangcunMount/component-base/pkg/log"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/actor"
)

// ActorService Actor gRPC 服务 - C 端接口
// 提供受试者相关的服务，主要面向 C 端用户（患者/家长）和外部系统（collection-server）
type ActorService struct {
	pb.UnimplementedActorServiceServer
	registrationService testeeApp.TesteeRegistrationService
	managementService   testeeApp.TesteeManagementService
	queryService        testeeApp.TesteeQueryService
}

// NewActorService 创建 Actor gRPC 服务
func NewActorService(
	registrationService testeeApp.TesteeRegistrationService,
	managementService testeeApp.TesteeManagementService,
	queryService testeeApp.TesteeQueryService,
) *ActorService {
	return &ActorService{
		registrationService: registrationService,
		managementService:   managementService,
		queryService:        queryService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *ActorService) RegisterService(server *grpc.Server) {
	pb.RegisterActorServiceServer(server, s)
}

// CreateTestee 创建受试者
// @Description C端用户注册或外部系统创建受试者
func (s *ActorService) CreateTestee(ctx context.Context, req *pb.CreateTesteeRequest) (*pb.TesteeResponse, error) {
	// 参数验证
	if req.OrgId == 0 {
		return nil, status.Error(codes.InvalidArgument, "机构ID不能为空")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "姓名不能为空")
	}

	// 构造 DTO
	var birthday *time.Time
	if req.Birthday != nil {
		t := req.Birthday.AsTime()
		birthday = &t
	}

	var profileID *uint64
	if req.IamChildId > 0 {
		profileID = &req.IamChildId
	}

	dto := testeeApp.RegisterTesteeDTO{
		OrgID:     int64(req.OrgId),
		ProfileID: profileID,
		Name:      req.Name,
		Gender:    int8(req.Gender),
		Birthday:  birthday,
		Source:    req.Source,
	}

	// 调用应用服务
	result, err := s.registrationService.Register(ctx, dto)
	if err != nil {
		log.Errorf("创建受试者失败: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 如果需要标记为重点关注
	if req.IsKeyFocus {
		if err := s.managementService.MarkAsKeyFocus(ctx, result.ID); err != nil {
			log.Warnf("标记重点关注失败: %v", err)
			// 不影响主流程，继续返回结果
		}
	}

	// 如果有标签需要添加
	for _, tag := range req.Tags {
		if tag != "" {
			if err := s.managementService.AddTag(ctx, result.ID, tag); err != nil {
				log.Warnf("添加标签失败: %v", err)
				// 不影响主流程
			}
		}
	}

	return s.toProtoTesteeResponse(result), nil
}

// GetTestee 获取受试者详情
// @Description 获取受试者的详细信息
func (s *ActorService) GetTestee(ctx context.Context, req *pb.GetTesteeRequest) (*pb.TesteeResponse, error) {
	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "受试者ID不能为空")
	}

	result, err := s.queryService.GetByID(ctx, req.Id)
	if err != nil {
		log.Errorf("获取受试者失败: %v", err)
		return nil, status.Error(codes.NotFound, "受试者不存在")
	}

	return s.toProtoTesteeResponse(result), nil
}

// UpdateTestee 更新受试者信息
// @Description 更新受试者的基本信息
func (s *ActorService) UpdateTestee(ctx context.Context, req *pb.UpdateTesteeRequest) (*pb.TesteeResponse, error) {
	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "受试者ID不能为空")
	}

	// 构造更新 DTO
	var birthday *time.Time
	if req.Birthday != nil {
		t := req.Birthday.AsTime()
		birthday = &t
	}

	dto := testeeApp.UpdateTesteeProfileDTO{
		TesteeID: req.Id,
		Name:     req.Name,
		Gender:   int8(req.Gender),
		Birthday: birthday,
	}

	// 调用应用服务
	if err := s.managementService.UpdateBasicInfo(ctx, dto); err != nil {
		log.Errorf("更新受试者失败: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 更新标签
	if len(req.Tags) > 0 {
		// 获取当前受试者信息
		current, err := s.queryService.GetByID(ctx, req.Id)
		if err != nil {
			log.Warnf("获取受试者当前信息失败: %v", err)
		} else {
			// 简单处理：先删除所有旧标签，再添加新标签
			for _, oldTag := range current.Tags {
				_ = s.managementService.RemoveTag(ctx, req.Id, oldTag)
			}
			for _, newTag := range req.Tags {
				if newTag != "" {
					_ = s.managementService.AddTag(ctx, req.Id, newTag)
				}
			}
		}
	}

	// 更新重点关注状态
	if req.IsKeyFocus {
		_ = s.managementService.MarkAsKeyFocus(ctx, req.Id)
	} else {
		_ = s.managementService.UnmarkKeyFocus(ctx, req.Id)
	}

	// 返回更新后的信息
	result, err := s.queryService.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "获取更新后的受试者信息失败")
	}

	return s.toProtoTesteeResponse(result), nil
}

// TesteeExists 检查受试者是否存在
// @Description 检查指定机构下的用户档案是否已创建受试者
func (s *ActorService) TesteeExists(ctx context.Context, req *pb.TesteeExistsRequest) (*pb.TesteeExistsResponse, error) {
	if req.OrgId == 0 {
		return nil, status.Error(codes.InvalidArgument, "机构ID不能为空")
	}
	if req.IamChildId == 0 {
		return nil, status.Error(codes.InvalidArgument, "用户档案ID不能为空")
	}

	result, err := s.queryService.FindByProfile(ctx, int64(req.OrgId), req.IamChildId)
	if err != nil || result == nil {
		return &pb.TesteeExistsResponse{
			Exists:   false,
			TesteeId: 0,
		}, nil
	}

	return &pb.TesteeExistsResponse{
		Exists:   true,
		TesteeId: result.ID,
	}, nil
}

// ListTesteesByOrg 根据机构查询受试者列表
// @Description 查询指定机构下的受试者列表
func (s *ActorService) ListTesteesByOrg(ctx context.Context, req *pb.ListTesteesByOrgRequest) (*pb.TesteeListResponse, error) {
	if req.OrgId == 0 {
		return nil, status.Error(codes.InvalidArgument, "机构ID不能为空")
	}

	dto := testeeApp.ListTesteeDTO{
		OrgID:  int64(req.OrgId),
		Offset: int(req.Offset),
		Limit:  int(req.Limit),
	}

	// 设置默认值
	if dto.Limit <= 0 || dto.Limit > 100 {
		dto.Limit = 20
	}

	result, err := s.queryService.ListTestees(ctx, dto)
	if err != nil {
		log.Errorf("查询受试者列表失败: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return s.toProtoTesteeListResponse(result), nil
}

// toProtoTesteeResponse 转换为 proto TesteeResponse
func (s *ActorService) toProtoTesteeResponse(result *testeeApp.TesteeResult) *pb.TesteeResponse {
	if result == nil {
		return nil
	}

	resp := &pb.TesteeResponse{
		Id:         result.ID,
		OrgId:      uint64(result.OrgID),
		Name:       result.Name,
		Gender:     int32(result.Gender),
		Tags:       result.Tags,
		Source:     result.Source,
		IsKeyFocus: result.IsKeyFocus,
	}

	// 设置用户档案ID
	if result.ProfileID != nil {
		resp.IamChildId = *result.ProfileID
	}

	// 设置生日
	if result.Birthday != nil {
		resp.Birthday = timestamppb.New(*result.Birthday)
	}

	// 设置测评统计信息
	if result.TotalAssessments > 0 || result.LastAssessmentAt != nil {
		stats := &pb.AssessmentStats{
			TotalCount:    int32(result.TotalAssessments),
			LastRiskLevel: result.LastRiskLevel,
		}
		if result.LastAssessmentAt != nil {
			stats.LastAssessmentAt = timestamppb.New(*result.LastAssessmentAt)
		}
		resp.AssessmentStats = stats
	}

	return resp
}

// toProtoTesteeListResponse 转换为 proto TesteeListResponse
func (s *ActorService) toProtoTesteeListResponse(result *testeeApp.TesteeListResult) *pb.TesteeListResponse {
	if result == nil {
		return &pb.TesteeListResponse{
			Items: []*pb.TesteeResponse{},
			Total: 0,
		}
	}

	items := make([]*pb.TesteeResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, s.toProtoTesteeResponse(item))
	}

	return &pb.TesteeListResponse{
		Items: items,
		Total: result.TotalCount,
	}
}
