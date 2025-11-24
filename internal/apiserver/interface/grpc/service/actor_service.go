package service

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	testeeShared "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/shared"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/actor"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ActorService Actor 服务的 gRPC 实现
type ActorService struct {
	pb.UnimplementedActorServiceServer
	testeeService testeeShared.Service
}

// NewActorService 创建 Actor 服务
func NewActorService(testeeService testeeShared.Service) *ActorService {
	return &ActorService{
		testeeService: testeeService,
	}
}

// RegisterService 注册服务到 gRPC 服务器
func (s *ActorService) RegisterService(server *grpc.Server) {
	pb.RegisterActorServiceServer(server, s)
}

// CreateTestee 创建受试者
func (s *ActorService) CreateTestee(ctx context.Context, req *pb.CreateTesteeRequest) (*pb.TesteeResponse, error) {
	log.Infof("gRPC CreateTestee called: org_id=%d, name=%s, profile_id=%d",
		req.OrgId, req.Name, req.IamChildId)

	// 构建应用层 DTO
	// 注意：当前 ProfileID 对应 IAM.Child.ID（使用 IamChildId 字段）
	dto := testeeShared.CreateTesteeDTO{
		OrgID:      int64(req.OrgId),
		ProfileID:  toUint64Ptr(req.IamChildId),
		Name:       req.Name,
		Gender:     int8(req.Gender),
		Birthday:   toTimePtr(req.Birthday),
		Tags:       req.Tags,
		Source:     req.Source,
		IsKeyFocus: req.IsKeyFocus,
	}

	// 调用应用服务创建受试者
	result, err := s.testeeService.Create(ctx, dto)
	if err != nil {
		log.Errorf("Failed to create testee: %v", err)
		return nil, fmt.Errorf("failed to create testee: %w", err)
	}

	log.Infof("Testee created successfully: id=%d", result.ID)
	return toTesteeProtoResponse(result), nil
}

// GetTestee 获取受试者详情
func (s *ActorService) GetTestee(ctx context.Context, req *pb.GetTesteeRequest) (*pb.TesteeResponse, error) {
	log.Infof("gRPC GetTestee called: id=%d", req.Id)

	result, err := s.testeeService.GetByID(ctx, req.Id)
	if err != nil {
		log.Errorf("Failed to get testee: %v", err)
		return nil, fmt.Errorf("failed to get testee: %w", err)
	}

	return toTesteeProtoResponse(result), nil
}

// UpdateTestee 更新受试者信息
func (s *ActorService) UpdateTestee(ctx context.Context, req *pb.UpdateTesteeRequest) (*pb.TesteeResponse, error) {
	log.Infof("gRPC UpdateTestee called: id=%d", req.Id)

	// 构建应用层 DTO
	dto := testeeShared.UpdateTesteeDTO{
		Name:       &req.Name,
		Tags:       req.Tags,
		IsKeyFocus: &req.IsKeyFocus,
	}

	if req.Gender > 0 {
		gender := int8(req.Gender)
		dto.Gender = &gender
	}

	if req.Birthday != nil {
		birthday := req.Birthday.AsTime()
		dto.Birthday = &birthday
	}

	// 调用应用服务更新受试者
	result, err := s.testeeService.Update(ctx, req.Id, dto)
	if err != nil {
		log.Errorf("Failed to update testee: %v", err)
		return nil, fmt.Errorf("failed to update testee: %w", err)
	}

	log.Infof("Testee updated successfully: id=%d", result.ID)
	return toTesteeProtoResponse(result), nil
}

// TesteeExists 检查受试者是否存在
func (s *ActorService) TesteeExists(ctx context.Context, req *pb.TesteeExistsRequest) (*pb.TesteeExistsResponse, error) {
	log.Infof("gRPC TesteeExists called: org_id=%d, profile_id=%d", req.OrgId, req.IamChildId)

	// 注意：当前 ProfileID 对应 IAM.Child.ID（使用 IamChildId 字段）
	testee, err := s.testeeService.FindByProfileID(ctx, int64(req.OrgId), req.IamChildId)
	if err != nil {
		// 如果是未找到错误，返回不存在
		return &pb.TesteeExistsResponse{
			Exists:   false,
			TesteeId: 0,
		}, nil
	}

	return &pb.TesteeExistsResponse{
		Exists:   true,
		TesteeId: testee.ID,
	}, nil
}

// ListTesteesByOrg 根据机构查询受试者列表
func (s *ActorService) ListTesteesByOrg(ctx context.Context, req *pb.ListTesteesByOrgRequest) (*pb.TesteeListResponse, error) {
	log.Infof("gRPC ListTesteesByOrg called: org_id=%d, offset=%d, limit=%d",
		req.OrgId, req.Offset, req.Limit)

	results, err := s.testeeService.ListByOrg(ctx, int64(req.OrgId), int(req.Offset), int(req.Limit))
	if err != nil {
		log.Errorf("Failed to list testees: %v", err)
		return nil, fmt.Errorf("failed to list testees: %w", err)
	}

	// 获取总数
	total, err := s.testeeService.CountByOrg(ctx, int64(req.OrgId))
	if err != nil {
		log.Errorf("Failed to count testees: %v", err)
		return nil, fmt.Errorf("failed to count testees: %w", err)
	}

	// 转换为 proto 响应
	items := make([]*pb.TesteeResponse, 0, len(results))
	for _, result := range results {
		items = append(items, toTesteeProtoResponse(result))
	}

	return &pb.TesteeListResponse{
		Items: items,
		Total: total,
	}, nil
}

// toTesteeProtoResponse 将应用层结果转换为 proto 响应
func toTesteeProtoResponse(result *testeeShared.CompositeTesteeResult) *pb.TesteeResponse {
	// 注意：当前 ProfileID 对应 IAM.Child.ID（映射到 IamChildId 字段）
	resp := &pb.TesteeResponse{
		Id:         result.ID,
		OrgId:      uint64(result.OrgID),
		IamUserId:  0, // 已废弃
		IamChildId: toUint64FromUint64Ptr(result.ProfileID),
		Name:       result.Name,
		Gender:     int32(result.Gender),
		Birthday:   toTimestampPtr(result.Birthday),
		Tags:       result.Tags,
		Source:     result.Source,
		IsKeyFocus: result.IsKeyFocus,
	}

	if result.AssessmentStats != nil {
		resp.AssessmentStats = &pb.AssessmentStats{
			TotalCount:       int32(result.AssessmentStats.TotalAssessments),
			LastAssessmentAt: toTimestampPtr(result.AssessmentStats.LastAssessmentAt),
			LastRiskLevel:    result.AssessmentStats.LastRiskLevel,
		}
	}

	return resp
}

// toInt64Ptr 将 uint64 转换为 *int64
func toInt64Ptr(v uint64) *int64 {
	if v == 0 {
		return nil
	}
	i := int64(v)
	return &i
}

// toUint64Ptr 将 uint64 转换为 *uint64
func toUint64Ptr(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

// toUint64FromInt64Ptr 将 *int64 转换为 uint64
func toUint64FromInt64Ptr(v *int64) uint64 {
	if v == nil {
		return 0
	}
	return uint64(*v)
}

// toUint64FromUint64Ptr 将 *uint64 转换为 uint64
func toUint64FromUint64Ptr(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

// toTimePtr 将 *timestamppb.Timestamp 转换为 *time.Time
func toTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// toTimestampPtr 将 *time.Time 转换为 *timestamppb.Timestamp
func toTimestampPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
