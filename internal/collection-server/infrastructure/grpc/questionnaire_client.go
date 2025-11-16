package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// QuestionnaireClient 问卷客户端接口
type QuestionnaireClient interface {
	// GetQuestionnaire 获取问卷详情
	GetQuestionnaire(ctx context.Context, code string) (*questionnaire.GetQuestionnaireResponse, error)
	// ListQuestionnaires 获取问卷列表
	ListQuestionnaires(ctx context.Context, req *questionnaire.ListQuestionnairesRequest) (*questionnaire.ListQuestionnairesResponse, error)
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
	// Close 关闭连接
	Close() error
}

// questionnaireClient 问卷客户端实现
type questionnaireClient struct {
	conn   *grpc.ClientConn
	client questionnaire.QuestionnaireServiceClient
	config *options.GRPCClientOptions
}

// NewQuestionnaireClient 创建新的问卷客户端
func NewQuestionnaireClient(config *options.GRPCClientOptions) (QuestionnaireClient, error) {
	// 设置连接选项
	kacp := keepalive.ClientParameters{
		Time:                30 * time.Second, // 每30秒发送一次ping
		Timeout:             10 * time.Second, // ping超时时间
		PermitWithoutStream: false,            // 只在有活跃RPC时发送ping
	}
	opts := []grpc.DialOption{
		grpc.WithTimeout(time.Duration(config.Timeout) * time.Second),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithUnaryInterceptor(middleware.UnaryClientLoggingInterceptor()),
		grpc.WithStreamInterceptor(middleware.StreamClientLoggingInterceptor()),
	}

	// 根据配置决定是否使用TLS
	if config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 建立连接
	conn, err := grpc.Dial(config.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to questionnaire service: %w", err)
	}

	// 创建客户端
	client := questionnaire.NewQuestionnaireServiceClient(conn)

	log.Infof("Connected to questionnaire service at %s", config.Endpoint)

	return &questionnaireClient{
		conn:   conn,
		client: client,
		config: config,
	}, nil
}

// GetQuestionnaire 获取问卷详情
func (c *questionnaireClient) GetQuestionnaire(ctx context.Context, code string) (*questionnaire.GetQuestionnaireResponse, error) {
	req := &questionnaire.GetQuestionnaireRequest{
		Code: code,
	}

	resp, err := c.client.GetQuestionnaire(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get questionnaire %s: %w", code, err)
	}

	return resp, nil
}

// ListQuestionnaires 获取问卷列表
func (c *questionnaireClient) ListQuestionnaires(ctx context.Context, req *questionnaire.ListQuestionnairesRequest) (*questionnaire.ListQuestionnairesResponse, error) {
	resp, err := c.client.ListQuestionnaires(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list questionnaires: %w", err)
	}

	return resp, nil
}

// HealthCheck 健康检查
func (c *questionnaireClient) HealthCheck(ctx context.Context) error {
	// 尝试获取一个空的问卷列表来检查连接
	req := &questionnaire.ListQuestionnairesRequest{
		Page:     1,
		PageSize: 1,
	}

	_, err := c.client.ListQuestionnaires(ctx, req)
	if err != nil {
		return fmt.Errorf("questionnaire client health check failed: %w", err)
	}

	return nil
}

// Close 关闭连接
func (c *questionnaireClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
