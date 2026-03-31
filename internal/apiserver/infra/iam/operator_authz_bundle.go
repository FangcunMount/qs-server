package iam

// OperatorAuthzBundle 注入 Operator 生命周期/权限服务：IAM Assignment + 授权快照加载器。
type OperatorAuthzBundle struct {
	Assignment *AuthzAssignmentClient
	Snapshot   *AuthzSnapshotLoader
}
