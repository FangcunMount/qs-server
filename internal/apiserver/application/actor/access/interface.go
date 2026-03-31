package access

import "context"

// TesteeAccessScope 描述当前操作者在某个机构下的 testee 可见范围。
type TesteeAccessScope struct {
	IsAdmin     bool
	ClinicianID *uint64
}

// TesteeAccessService 统一解析后台操作者的 testee 可见范围。
type TesteeAccessService interface {
	ResolveAccessScope(ctx context.Context, orgID int64, operatorUserID int64) (*TesteeAccessScope, error)
	ValidateTesteeAccess(ctx context.Context, orgID int64, operatorUserID int64, testeeID uint64) error
	ListAccessibleTesteeIDs(ctx context.Context, orgID int64, operatorUserID int64) ([]uint64, error)
}
