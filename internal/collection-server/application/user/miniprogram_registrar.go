package user

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/auth"
	grpcclient "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/grpc"
	"github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/wechat"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// MiniProgramRegistrar 小程序注册服务
type MiniProgramRegistrar struct {
	userServiceClient *grpcclient.UserServiceClient
	miniProgramClient *wechat.MiniProgramClient
	jwtManager        *auth.JWTManager
	appID             string
}

// NewMiniProgramRegistrar 创建小程序注册服务
func NewMiniProgramRegistrar(
	userServiceClient *grpcclient.UserServiceClient,
	miniProgramClient *wechat.MiniProgramClient,
	jwtManager *auth.JWTManager,
	appID string,
) *MiniProgramRegistrar {
	return &MiniProgramRegistrar{
		userServiceClient: userServiceClient,
		miniProgramClient: miniProgramClient,
		jwtManager:        jwtManager,
		appID:             appID,
	}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Code     string `json:"code" binding:"required"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	OpenID    string `json:"open_id"`
	IsNewUser bool   `json:"is_new_user"`
}

// Register 小程序注册/登录
func (r *MiniProgramRegistrar) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	// 1. 调用微信 API，通过 code 换取 openid 和 session_key
	log.Debugf("Calling wechat code2session with code: %s", req.Code)
	sessionResp, err := r.miniProgramClient.Code2Session(ctx, req.Code)
	if err != nil {
		log.Errorf("Failed to call code2session: %v", err)
		return nil, fmt.Errorf("failed to get wechat session: %w", err)
	}

	log.Infof("Successfully got wechat session for openid: %s", sessionResp.OpenID)

	// 2. 调用 apiserver gRPC，创建或更新微信账号
	wechatAccount, err := r.userServiceClient.CreateOrUpdateMiniProgramAccount(
		ctx,
		r.appID,
		sessionResp.OpenID,
		sessionResp.UnionID,
		req.Nickname,
		req.Avatar,
		sessionResp.SessionKey,
	)
	if err != nil {
		log.Errorf("Failed to create or update miniprogram account: %v", err)
		return nil, fmt.Errorf("failed to create or update account: %w", err)
	}

	// 3. 生成 JWT Token
	userIDStr := strconv.FormatUint(wechatAccount.UserId, 10)
	token, err := r.jwtManager.GenerateToken(userIDStr, r.appID, sessionResp.OpenID)
	if err != nil {
		log.Errorf("Failed to generate JWT token: %v", err)
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	log.Infof("User %d successfully registered/logged in", wechatAccount.UserId)

	return &RegisterResponse{
		Token:     token,
		UserID:    userIDStr,
		OpenID:    sessionResp.OpenID,
		IsNewUser: wechatAccount.IsNewAccount,
	}, nil
}
