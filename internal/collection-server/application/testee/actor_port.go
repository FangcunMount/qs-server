package testee

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ActorPort 受试者 gRPC 端口（application-owned DTO）。
type ActorPort interface {
	GetTestee(ctx context.Context, testeeID uint64) (*TesteeResponse, error)
	TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (exists bool, testeeID uint64, err error)
	CreateTestee(ctx context.Context, input CreateTesteeInput) (*TesteeResponse, error)
	GetTesteeCareContext(ctx context.Context, testeeID uint64) (*TesteeCareContextResponse, error)
	UpdateTestee(ctx context.Context, testeeID uint64, req *UpdateTesteeRequest) (*TesteeResponse, error)
	ListTesteesByUser(ctx context.Context, profileIDs []uint64, offset, limit int32) ([]*TesteeResponse, int64, error)
}

// CreateTesteeInput 创建受试者 gRPC 入参。
type CreateTesteeInput struct {
	OrgID        uint64
	IAMUserID    string
	IAMProfileID string
	Name         string
	Gender       int32
	Birthday     *meta.Birthday
	Tags         []string
	Source       string
	IsKeyFocus   bool
}
