package iam

import iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"

// AuthzSnapshotLoaderOptions 配置 IAM 授权快照加载（见 pkg/iamauth）。
type AuthzSnapshotLoaderOptions = iamauth.SnapshotLoaderOptions

// AuthzSnapshotLoader 实现 CurrentAuthzSnapshot（见 pkg/iamauth）。
type AuthzSnapshotLoader = iamauth.SnapshotLoader

// NewAuthzSnapshotLoader 创建加载器。
func NewAuthzSnapshotLoader(client *Client, opts AuthzSnapshotLoaderOptions) *AuthzSnapshotLoader {
	return iamauth.NewSnapshotLoader(client, opts)
}
