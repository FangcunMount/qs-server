package grpc

import (
	"context"
	"fmt"
	"time"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/user"
	"github.com/FangcunMount/qs-server/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserServiceClient 封装了 gRPC UserService 客户端
type UserServiceClient struct {
	conn   *grpc.ClientConn
	client pb.UserServiceClient
}

// NewUserServiceClient 创建 UserService 客户端
func NewUserServiceClient(endpoint string, timeout int) (*UserServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// 创建 gRPC 连接
	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to apiserver: %w", err)
	}

	log.Infof("Successfully connected to apiserver at %s", endpoint)

	return &UserServiceClient{
		conn:   conn,
		client: pb.NewUserServiceClient(conn),
	}, nil
}

// Close 关闭 gRPC 连接
func (c *UserServiceClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// CreateOrUpdateMiniProgramAccount 创建或更新小程序账号
func (c *UserServiceClient) CreateOrUpdateMiniProgramAccount(
	ctx context.Context,
	appID string,
	openID string,
	unionID string,
	nickname string,
	avatar string,
	sessionKey string,
) (*pb.WechatAccountResponse, error) {
	req := &pb.CreateOrUpdateMiniProgramAccountRequest{
		AppId:      appID,
		OpenId:     openID,
		UnionId:    unionID,
		Nickname:   nickname,
		Avatar:     avatar,
		SessionKey: sessionKey,
	}

	resp, err := c.client.CreateOrUpdateMiniProgramAccount(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create or update miniprogram account: %w", err)
	}

	return resp, nil
}

// GetWechatAccountByOpenID 根据 OpenID 获取微信账号
func (c *UserServiceClient) GetWechatAccountByOpenID(
	ctx context.Context,
	appID string,
	platform string,
	openID string,
) (*pb.WechatAccountResponse, error) {
	req := &pb.GetWechatAccountByOpenIDRequest{
		AppId:    appID,
		Platform: platform,
		OpenId:   openID,
	}

	resp, err := c.client.GetWechatAccountByOpenID(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get wechat account: %w", err)
	}

	return resp, nil
}

// GetUser 获取用户信息
func (c *UserServiceClient) GetUser(ctx context.Context, userID uint64) (*pb.GetUserResponse, error) {
	req := &pb.GetUserRequest{
		UserId: userID,
	}

	resp, err := c.client.GetUser(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return resp, nil
}

// CreateTestee 创建受试者
func (c *UserServiceClient) CreateTestee(
	ctx context.Context,
	userID uint64,
	name string,
	sex uint32,
	birthday int64, // Unix timestamp
) (*pb.TesteeResponse, error) {
	req := &pb.CreateTesteeRequest{
		UserId:   userID,
		Name:     name,
		Sex:      sex,
		Birthday: timestamppb.New(time.Unix(birthday, 0)),
	}

	resp, err := c.client.CreateTestee(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create testee: %w", err)
	}

	return resp, nil
}

// GetTestee 获取受试者信息
func (c *UserServiceClient) GetTestee(ctx context.Context, userID uint64) (*pb.TesteeResponse, error) {
	req := &pb.GetTesteeRequest{
		UserId: userID,
	}

	resp, err := c.client.GetTestee(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get testee: %w", err)
	}

	return resp, nil
}

// TesteeExists 检查受试者是否存在
func (c *UserServiceClient) TesteeExists(ctx context.Context, userID uint64) (bool, error) {
	req := &pb.TesteeExistsRequest{
		UserId: userID,
	}

	resp, err := c.client.TesteeExists(ctx, req)
	if err != nil {
		return false, fmt.Errorf("failed to check testee existence: %w", err)
	}

	return resp.Exists, nil
}
