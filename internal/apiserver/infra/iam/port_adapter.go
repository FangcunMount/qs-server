package iam

import (
	"context"
	"fmt"
	"strconv"

	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func (s *WeChatAppService) ResolveWeChatAppConfig(ctx context.Context, appID string) (*iambridge.WeChatAppConfig, error) {
	resp, err := s.GetWechatApp(ctx, appID)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.App == nil {
		return nil, fmt.Errorf("IAM returned empty wechat app")
	}
	return &iambridge.WeChatAppConfig{
		AppID:     resp.App.GetAppId(),
		AppSecret: resp.App.GetAppSecret(),
	}, nil
}

func (s *IdentityService) ResolveUserNames(ctx context.Context, ids []meta.ID) map[string]string {
	return ResolveUserNames(ctx, s, ids)
}

type authzSnapshotReader struct {
	loader *AuthzSnapshotLoader
}

func NewAuthzSnapshotReader(loader *AuthzSnapshotLoader) iambridge.AuthzSnapshotReader {
	if loader == nil {
		return nil
	}
	return &authzSnapshotReader{loader: loader}
}

func (r *authzSnapshotReader) LoadAuthzSnapshot(ctx context.Context, orgID, userID int64) (iambridge.AuthzSnapshot, error) {
	if r == nil || r.loader == nil {
		return nil, fmt.Errorf("iam authorization snapshot loader is not available")
	}
	return r.loader.Load(ctx, strconv.FormatInt(orgID, 10), strconv.FormatInt(userID, 10))
}
