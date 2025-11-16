package user

import (
	"context"
	"fmt"
	"strconv"
	"time"

	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// TesteeRegistrar 受试者注册服务
type TesteeRegistrar struct {
	userServiceClient *grpcclient.UserServiceClient
}

// NewTesteeRegistrar 创建受试者注册服务
func NewTesteeRegistrar(userServiceClient *grpcclient.UserServiceClient) *TesteeRegistrar {
	return &TesteeRegistrar{
		userServiceClient: userServiceClient,
	}
}

// CreateTesteeRequest 创建受试者请求
type CreateTesteeRequest struct {
	Name     string `json:"name" binding:"required"`
	Sex      uint32 `json:"sex" binding:"required,oneof=0 1 2"` // 0-未知, 1-男, 2-女
	Birthday string `json:"birthday" binding:"required"`        // RFC3339 格式
}

// CreateTesteeResponse 创建受试者响应
type CreateTesteeResponse struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Sex      uint32 `json:"sex"`
	Birthday string `json:"birthday"`
	Age      int32  `json:"age"`
}

// CreateTestee 创建受试者
func (r *TesteeRegistrar) CreateTestee(ctx context.Context, userIDStr string, req *CreateTesteeRequest) (*CreateTesteeResponse, error) {
	log.Infof("Creating testee for user: %s", userIDStr)

	// 转换 userID 为 uint64
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	// 解析生日
	birthday, err := time.Parse(time.RFC3339, req.Birthday)
	if err != nil {
		return nil, fmt.Errorf("invalid birthday format: %w", err)
	}

	// 调用 apiserver gRPC 创建受试者
	testee, err := r.userServiceClient.CreateTestee(
		ctx,
		userID,
		req.Name,
		req.Sex,
		birthday.Unix(),
	)
	if err != nil {
		log.Errorf("Failed to create testee for user %s: %v", userIDStr, err)
		return nil, fmt.Errorf("failed to create testee: %w", err)
	}

	log.Infof("Successfully created testee for user %d", testee.UserId)

	return &CreateTesteeResponse{
		UserID:   strconv.FormatUint(testee.UserId, 10),
		Name:     testee.Name,
		Sex:      testee.Sex,
		Birthday: testee.Birthday.AsTime().Format(time.RFC3339),
		Age:      testee.Age,
	}, nil
}
