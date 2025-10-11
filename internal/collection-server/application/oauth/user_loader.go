package oauth

import (
    "context"
    "fmt"
    "strconv"

    grpcclient "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/grpc"
    "github.com/fangcun-mount/qs-server/pkg/log"
)

// GRPCUserLoader 基于 gRPC 的用户加载器
type GRPCUserLoader struct {
    userServiceClient *grpcclient.UserServiceClient
}

// NewGRPCUserLoader 创建 gRPC 用户加载器
func NewGRPCUserLoader(userServiceClient *grpcclient.UserServiceClient) *GRPCUserLoader {
    return &GRPCUserLoader{
        userServiceClient: userServiceClient,
    }
}

// LoadOrCreateUser 加载或创建用户
func (l *GRPCUserLoader) LoadOrCreateUser(
    ctx context.Context,
    userInfo UserInfo,
    payload map[string]interface{},
) (string, bool, error) {
    // 获取 app_id
    appID, _ := payload["app_id"].(string)
    if appID == "" {
        return "", false, fmt.Errorf("app_id is required in payload")
    }

    // 调用 apiserver gRPC 服务，创建或更新微信账号
    wechatAccount, err := l.userServiceClient.CreateOrUpdateMiniProgramAccount(
        ctx,
        appID,
        userInfo.GetOpenID(),
        userInfo.GetUniqueID(),
        userInfo.GetNickname(),
        userInfo.GetAvatar(),
        "", // session_key 在这里不需要传递，因为已经在 queryUserInfo 阶段获取
    )
    if err != nil {
        log.Errorf("Failed to create or update miniprogram account: %v", err)
        return "", false, fmt.Errorf("failed to create or update account: %w", err)
    }

    userIDStr := strconv.FormatUint(wechatAccount.UserId, 10)
    log.Infof("User %s successfully loaded/created (is_new: %v)", userIDStr, wechatAccount.IsNewAccount)

    return userIDStr, wechatAccount.IsNewAccount, nil
}
