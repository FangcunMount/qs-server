// Package operatorretire coordinates reviewed Operator exits through the Actor service.
package operatorretire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	repo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"gorm.io/gorm"
)

const operatorTable = "operators"

type SnapshotReader interface {
	LoadFresh(context.Context, string) (*authz.Snapshot, error)
	LoadAssignmentFacts(context.Context, string) (*authz.Snapshot, error)
}

type Tool struct {
	db         *gorm.DB
	loader     SnapshotReader
	gateway    iambridge.OperatorAuthzGateway
	repository *repo.Repository
}

func New(db *gorm.DB, loader SnapshotReader, gateway iambridge.OperatorAuthzGateway) *Tool {
	return &Tool{db: db, loader: loader, gateway: gateway, repository: repo.NewRepository(db)}
}

func hash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
