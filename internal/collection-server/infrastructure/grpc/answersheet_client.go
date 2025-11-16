package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	answersheetpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// AnswersheetClient 答卷客户端接口
type AnswersheetClient interface {
	// SaveAnswersheet 保存答卷
	SaveAnswersheet(ctx context.Context, req *answersheetpb.SaveAnswerSheetRequest) (*answersheetpb.SaveAnswerSheetResponse, error)
	// GetAnswersheet 获取答卷详情
	GetAnswersheet(ctx context.Context, id uint64) (*answersheetpb.GetAnswerSheetResponse, error)
	// ListAnswersheets 获取答卷列表
	ListAnswersheets(ctx context.Context, req *answersheetpb.ListAnswerSheetsRequest) (*answersheetpb.ListAnswerSheetsResponse, error)
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
	// Close 关闭连接
	Close() error
}

// answersheetClient 答卷客户端实现
type answersheetClient struct {
	conn   *grpc.ClientConn
	client answersheetpb.AnswerSheetServiceClient
	config *options.GRPCClientOptions
}

// NewAnswersheetClient 创建新的答卷客户端
func NewAnswersheetClient(config *options.GRPCClientOptions) (AnswersheetClient, error) {
	// 设置连接选项
	kacp := keepalive.ClientParameters{
		Time:                30 * time.Second, // 每30秒发送一次ping（从10秒增加到30秒）
		Timeout:             10 * time.Second, // ping超时时间（从3秒增加到10秒）
		PermitWithoutStream: false,            // 只在有活跃RPC时发送ping（从true改为false）
	}

	opts := []grpc.DialOption{
		grpc.WithTimeout(time.Duration(config.Timeout) * time.Second),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithUnaryInterceptor(middleware.UnaryClientLoggingInterceptor()),
		grpc.WithStreamInterceptor(middleware.StreamClientLoggingInterceptor()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(20*1024*1024), // 20MB
			grpc.MaxCallSendMsgSize(20*1024*1024), // 20MB
		),
	}

	// 根据配置决定是否使用TLS
	if config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 建立连接
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Timeout)*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, config.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to answersheet service: %w", err)
	}

	// 创建客户端
	client := answersheetpb.NewAnswerSheetServiceClient(conn)

	log.Infof("Connected to answersheet service at %s", config.Endpoint)

	return &answersheetClient{
		conn:   conn,
		client: client,
		config: config,
	}, nil
}

// SaveAnswersheet 保存答卷
func (c *answersheetClient) SaveAnswersheet(ctx context.Context, req *answersheetpb.SaveAnswerSheetRequest) (*answersheetpb.SaveAnswerSheetResponse, error) {
	// 添加超时控制
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.SaveAnswerSheet(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to save answersheet: %w", err)
	}

	return resp, nil
}

// GetAnswersheet 获取答卷详情
func (c *answersheetClient) GetAnswersheet(ctx context.Context, id uint64) (*answersheetpb.GetAnswerSheetResponse, error) {
	req := &answersheetpb.GetAnswerSheetRequest{
		Id: id,
	}

	resp, err := c.client.GetAnswerSheet(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get answersheet %d: %w", id, err)
	}

	return resp, nil
}

// ListAnswersheets 获取答卷列表
func (c *answersheetClient) ListAnswersheets(ctx context.Context, req *answersheetpb.ListAnswerSheetsRequest) (*answersheetpb.ListAnswerSheetsResponse, error) {
	resp, err := c.client.ListAnswerSheets(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list answersheets: %w", err)
	}

	return resp, nil
}

// HealthCheck 健康检查
func (c *answersheetClient) HealthCheck(ctx context.Context) error {
	// 尝试获取一个空的答卷列表来检查连接
	req := &answersheetpb.ListAnswerSheetsRequest{
		Page:     1,
		PageSize: 1,
	}

	_, err := c.client.ListAnswerSheets(ctx, req)
	if err != nil {
		return fmt.Errorf("answersheet client health check failed: %w", err)
	}

	return nil
}

// Close 关闭连接
func (c *answersheetClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
