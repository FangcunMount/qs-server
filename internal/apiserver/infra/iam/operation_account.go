package iam

import (
	"context"
	"errors"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
)

type RegisterOperationAccountInput struct {
	ExistingUserID string
	Name           string
	Phone          string
	Email          string
	ScopedOrgID    string
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

var ErrOperationAccountNotSupported = errors.New("iam operation account onboarding is not supported by IAM v2.0.6 SDK")

// OperationAccountService 封装 IAM 运营账号注册能力。
type OperationAccountService struct {
	enabled bool
}

func NewOperationAccountService(client *Client) (*OperationAccountService, error) {
	if client == nil || !client.enabled {
		return &OperationAccountService{enabled: false}, nil
	}

	sdkClient := client.SDK()
	if sdkClient == nil {
		return nil, fmt.Errorf("SDK client is nil")
	}

	logger.L(context.Background()).Infow("OperationAccountService disabled because IAM v2.0.6 no longer exposes AccountOnboardingService",
		"component", "iam.operation_account",
		"result", "unsupported",
	)
	return &OperationAccountService{
		enabled: false,
	}, nil
}

func (s *OperationAccountService) IsEnabled() bool {
	return s.enabled
}

func (s *OperationAccountService) RegisterOperationAccount(ctx context.Context, input RegisterOperationAccountInput) (*RegisterOperationAccountResult, error) {
	return nil, ErrOperationAccountNotSupported
}
