package service

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	userApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user"
	roleApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user/role"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	roleDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/role"
	pb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/user"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// UserService 用户模块 gRPC 服务 - 统一的用户体系服务
// 负责对外提供用户、受试者、填写人等所有用户相关功能
// 注意：Auditor（审核员）是内部员工角色，不对 collection-server 开放
type UserService struct {
	pb.UnimplementedUserServiceServer

	// 基础用户服务
	userCreator *userApp.UserCreator
	userEditor  *userApp.UserEditor

	// 角色服务
	testeeCreator *roleApp.TesteeCreator
	writerCreator *roleApp.WriterCreator
}

// NewUserService 创建用户模块 gRPC 服务
func NewUserService(
	userCreator *userApp.UserCreator,
	userEditor *userApp.UserEditor,
	testeeCreator *roleApp.TesteeCreator,
	writerCreator *roleApp.WriterCreator,
) *UserService {
	return &UserService{
		userCreator:   userCreator,
		userEditor:    userEditor,
		testeeCreator: testeeCreator,
		writerCreator: writerCreator,
	}
}

// RegisterService 注册 gRPC 服务
func (s *UserService) RegisterService(server *grpc.Server) {
	pb.RegisterUserServiceServer(server, s)
}

// ========== 基础用户服务 ==========

// CreateUser 创建用户
func (s *UserService) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	log.Infof("CreateUser called: username=%s", req.Username)

	userObj, err := s.userCreator.CreateUser(
		ctx,
		req.Username,
		req.Password,
		req.Nickname,
		req.Email,
		req.Phone,
		req.Introduction,
	)
	if err != nil {
		log.Errorf("Failed to create user: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.CreateUserResponse{
		UserId:   userObj.ID().Value(),
		Username: userObj.Username(),
		Nickname: userObj.Nickname(),
		Email:    userObj.Email(),
		Phone:    userObj.Phone(),
	}, nil
}

// UpdateUserBasicInfo 更新用户基本信息
func (s *UserService) UpdateUserBasicInfo(ctx context.Context, req *pb.UpdateUserBasicInfoRequest) (*pb.UpdateUserBasicInfoResponse, error) {
	log.Infof("UpdateUserBasicInfo called: user_id=%d", req.UserId)

	userObj, err := s.userEditor.UpdateBasicInfo(
		ctx,
		req.UserId,
		req.Username,
		req.Nickname,
		req.Email,
		req.Phone,
		req.Avatar,
		req.Introduction,
	)
	if err != nil {
		log.Errorf("Failed to update user: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.UpdateUserBasicInfoResponse{
		UserId:   userObj.ID().Value(),
		Username: userObj.Username(),
		Nickname: userObj.Nickname(),
		Avatar:   userObj.Avatar(),
	}, nil
}

// GetUser 获取用户信息
func (s *UserService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	log.Infof("GetUser called: user_id=%d", req.UserId)

	// 注意：这里需要通过 repository 直接查询，或者添加 UserQueryer
	// 暂时返回错误，需要实现查询服务
	return nil, status.Error(codes.Unimplemented, "GetUser not implemented yet")
}

// ========== 受试者服务 ==========

// CreateTestee 创建受试者
func (s *UserService) CreateTestee(ctx context.Context, req *pb.CreateTesteeRequest) (*pb.TesteeResponse, error) {
	log.Infof("CreateTestee called: user_id=%d, name=%s", req.UserId, req.Name)

	// 转换 birthday from timestamppb to time.Time
	var birthday *time.Time
	if req.Birthday != nil {
		t := req.Birthday.AsTime()
		birthday = &t
	}

	testee, err := s.testeeCreator.CreateTestee(
		ctx,
		user.NewUserID(req.UserId),
		req.Name,
		uint8(req.Sex),
		birthday,
	)
	if err != nil {
		log.Errorf("Failed to create testee: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return s.toTesteeResponse(testee), nil
}

// UpdateTestee 更新受试者
func (s *UserService) UpdateTestee(ctx context.Context, req *pb.UpdateTesteeRequest) (*pb.TesteeResponse, error) {
	log.Infof("UpdateTestee called: user_id=%d", req.UserId)

	var name *string
	var sex *uint8
	var birthday *time.Time

	if req.Name != "" {
		name = &req.Name
	}
	if req.Sex != 0 {
		s := uint8(req.Sex)
		sex = &s
	}
	if req.Birthday != nil {
		t := req.Birthday.AsTime()
		birthday = &t
	}

	testee, err := s.testeeCreator.UpdateTestee(ctx, user.NewUserID(req.UserId), name, sex, birthday)
	if err != nil {
		log.Errorf("Failed to update testee: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return s.toTesteeResponse(testee), nil
}

// GetTestee 获取受试者
func (s *UserService) GetTestee(ctx context.Context, req *pb.GetTesteeRequest) (*pb.TesteeResponse, error) {
	log.Infof("GetTestee called: user_id=%d", req.UserId)

	testee, err := s.testeeCreator.GetTesteeByUserID(ctx, user.NewUserID(req.UserId))
	if err != nil {
		log.Errorf("Failed to get testee: %v", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return s.toTesteeResponse(testee), nil
}

// TesteeExists 检查受试者是否存在
func (s *UserService) TesteeExists(ctx context.Context, req *pb.TesteeExistsRequest) (*pb.TesteeExistsResponse, error) {
	exists := s.testeeCreator.TesteeExists(ctx, user.NewUserID(req.UserId))
	return &pb.TesteeExistsResponse{Exists: exists}, nil
}

// ========== 填写人服务 ==========

// CreateWriter 创建填写人
func (s *UserService) CreateWriter(ctx context.Context, req *pb.CreateWriterRequest) (*pb.WriterResponse, error) {
	log.Infof("CreateWriter called: user_id=%d, name=%s", req.UserId, req.Name)

	writer, err := s.writerCreator.CreateWriter(ctx, user.NewUserID(req.UserId), req.Name)
	if err != nil {
		log.Errorf("Failed to create writer: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return s.toWriterResponse(writer), nil
}

// UpdateWriter 更新填写人
func (s *UserService) UpdateWriter(ctx context.Context, req *pb.UpdateWriterRequest) (*pb.WriterResponse, error) {
	log.Infof("UpdateWriter called: user_id=%d", req.UserId)

	writer, err := s.writerCreator.UpdateWriter(ctx, user.NewUserID(req.UserId), req.Name)
	if err != nil {
		log.Errorf("Failed to update writer: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return s.toWriterResponse(writer), nil
}

// GetWriter 获取填写人
func (s *UserService) GetWriter(ctx context.Context, req *pb.GetWriterRequest) (*pb.WriterResponse, error) {
	log.Infof("GetWriter called: user_id=%d", req.UserId)

	writer, err := s.writerCreator.GetWriterByUserID(ctx, user.NewUserID(req.UserId))
	if err != nil {
		log.Errorf("Failed to get writer: %v", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return s.toWriterResponse(writer), nil
}

// ========== 辅助转换函数 ==========

func (s *UserService) toTesteeResponse(testee *roleDomain.Testee) *pb.TesteeResponse {
	return &pb.TesteeResponse{
		UserId:   testee.GetUserID().Value(),
		Name:     testee.GetName(),
		Sex:      uint32(testee.GetSex()),
		Birthday: timestamppb.New(testee.GetBirthday()),
		Age:      int32(testee.GetAge()),
	}
}

func (s *UserService) toWriterResponse(writer *roleDomain.Writer) *pb.WriterResponse {
	return &pb.WriterResponse{
		UserId: writer.GetUserID().Value(),
		Name:   writer.GetName(),
	}
}
