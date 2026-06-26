package iam

import iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"

// TestModuleOptions injects IAM dependencies for container/integration tests.
type TestModuleOptions struct {
	TokenVerifier       *iaminfra.TokenVerifier
	AuthzSnapshotLoader *iaminfra.AuthzSnapshotLoader
}

// NewTestModule builds an IAM module with injected dependencies.
func NewTestModule(opts TestModuleOptions) *Module {
	return &Module{
		tokenVerifier:       opts.TokenVerifier,
		authzSnapshotLoader: opts.AuthzSnapshotLoader,
	}
}
