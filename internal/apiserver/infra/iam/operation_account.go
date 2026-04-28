package iam

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	authnv1 "github.com/FangcunMount/iam/api/grpc/iam/authn/v1"
	auth "github.com/FangcunMount/iam/pkg/sdk/auth/client"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
)

type RegisterOperationAccountInput struct {
	ExistingUserID string
	Name           string
	Phone          string
	Email          string
	ScopedTenantID string
	OperaLoginID   string
	Password       string
}

type RegisterOperationAccountResult struct {
	UserID       int64
	AccountID    string
	CredentialID string
	ExternalID   string
	IsNewUser    bool
	IsNewAccount bool
}

// OperationAccountService 封装 IAM 运营账号注册能力。
type OperationAccountService struct {
	client  *auth.Client
	enabled bool
	limiter backpressure.Acquirer
}

func NewOperationAccountService(client *Client) (*OperationAccountService, error) {
	if client == nil || !client.enabled {
		return &OperationAccountService{enabled: false}, nil
	}

	sdkClient := client.SDK()
	if sdkClient == nil {
		return nil, fmt.Errorf("SDK client is nil")
	}

	authClient := sdkClient.Auth()
	if authClient == nil {
		return nil, fmt.Errorf("auth client is nil")
	}

	logger.L(context.Background()).Infow("OperationAccountService initialized",
		"component", "iam.operation_account",
		"result", "success",
	)
	return &OperationAccountService{
		client:  authClient,
		enabled: true,
		limiter: client.Limiter(),
	}, nil
}

func (s *OperationAccountService) IsEnabled() bool {
	return s.enabled
}

func (s *OperationAccountService) RegisterOperationAccount(ctx context.Context, input RegisterOperationAccountInput) (*RegisterOperationAccountResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("operation account service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	resp, err := s.client.RegisterOperationAccount(ctx, &authnv1.RegisterOperationAccountRequest{
		ExistingUserId: input.ExistingUserID,
		Name:           input.Name,
		Phone:          input.Phone,
		Email:          input.Email,
		ScopedTenantId: input.ScopedTenantID,
		OperaLoginId:   input.OperaLoginID,
		Password:       input.Password,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("empty response from IAM RegisterOperationAccount")
	}

	userID, err := strconv.ParseInt(resp.GetUserId(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user id from IAM: %w", err)
	}

	return &RegisterOperationAccountResult{
		UserID:       userID,
		AccountID:    resp.GetAccountId(),
		CredentialID: resp.GetCredentialId(),
		ExternalID:   resp.GetExternalId(),
		IsNewUser:    resp.GetIsNewUser(),
		IsNewAccount: resp.GetIsNewAccount(),
	}, nil
}

func (s *OperationAccountService) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}
