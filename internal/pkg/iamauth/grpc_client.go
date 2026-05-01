package iamauth

import sdk "github.com/FangcunMount/iam/v2/pkg/sdk"

// GRPCClient apiserver / collection-server 的 IAM Client 均只需暴露 SDK 与启用态。
type GRPCClient interface {
	SDK() *sdk.Client
	IsEnabled() bool
}
