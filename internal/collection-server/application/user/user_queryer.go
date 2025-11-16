package user

import (
	"context"
	"fmt"
	"strconv"
	"time"

	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// UserQueryer 用户查询服务
type UserQueryer struct {
	userServiceClient *grpcclient.UserServiceClient
}

// NewUserQueryer 创建用户查询服务
func NewUserQueryer(userServiceClient *grpcclient.UserServiceClient) *UserQueryer {
	return &UserQueryer{
		userServiceClient: userServiceClient,
	}
}

// UserInfo 用户信息
type UserInfo struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Nickname     string `json:"nickname"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Avatar       string `json:"avatar"`
	Introduction string `json:"introduction"`
	Status       uint32 `json:"status"`
}

// TesteeInfo 受试者信息
type TesteeInfo struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Sex      uint32 `json:"sex"`
	Birthday string `json:"birthday"`
	Age      int32  `json:"age"`
}

// GetUser 获取用户信息
func (q *UserQueryer) GetUser(ctx context.Context, userIDStr string) (*UserInfo, error) {
	log.Debugf("Getting user info for: %s", userIDStr)

	// 转换 userID
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	user, err := q.userServiceClient.GetUser(ctx, userID)
	if err != nil {
		log.Errorf("Failed to get user %s: %v", userIDStr, err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &UserInfo{
		UserID:       strconv.FormatUint(user.UserId, 10),
		Username:     user.Username,
		Nickname:     user.Nickname,
		Email:        user.Email,
		Phone:        user.Phone,
		Avatar:       user.Avatar,
		Introduction: user.Introduction,
		Status:       user.Status,
	}, nil
}

// GetTestee 获取受试者信息
func (q *UserQueryer) GetTestee(ctx context.Context, userIDStr string) (*TesteeInfo, error) {
	log.Debugf("Getting testee info for user: %s", userIDStr)

	// 转换 userID
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	testee, err := q.userServiceClient.GetTestee(ctx, userID)
	if err != nil {
		log.Errorf("Failed to get testee for user %s: %v", userIDStr, err)
		return nil, fmt.Errorf("failed to get testee: %w", err)
	}

	return &TesteeInfo{
		UserID:   strconv.FormatUint(testee.UserId, 10),
		Name:     testee.Name,
		Sex:      testee.Sex,
		Birthday: testee.Birthday.AsTime().Format(time.RFC3339),
		Age:      testee.Age,
	}, nil
}
