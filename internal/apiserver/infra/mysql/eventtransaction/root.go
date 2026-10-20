package eventtransaction

import (
	"bytes"
	"context"
	"database/sql"
	"sync/atomic"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

type originalRootKey struct{}
type originalRoot struct {
	primary   *sql.DB
	root      *sql.Tx
	execution transaction.Execution
	closed    atomic.Bool
}

func bindOriginal(ctx context.Context, primary *sql.DB, e transaction.Execution) (context.Context, func(), error) {
	if _, nested := ctx.Value(originalRootKey{}).(*originalRoot); nested {
		return nil, nil, transaction.ErrTransactionRequired
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return nil, nil, err
	}
	pool := tx.Statement.ConnPool
	if prepared, ok := pool.(*gorm.PreparedStmtTX); ok {
		pool = prepared.Tx
	}
	raw, ok := pool.(*sql.Tx)
	if !ok || raw == nil {
		return nil, nil, transaction.ErrTransactionRequired
	}
	view := tx.WithContext(ctx)
	view.Statement.ConnPool = raw
	parent, err := view.DB()
	if err != nil || parent != primary {
		return nil, nil, transaction.ErrBinding
	}
	r := &originalRoot{primary: primary, root: raw, execution: e}
	return context.WithValue(ctx, originalRootKey{}, r), func() { r.closed.Store(true) }, nil
}

// OriginalPlan grants only readonly access to the exact plan whose SDK guard
// is already bound in this active host root. It never authorizes a new execution.
func OriginalPlan(ctx context.Context, primary *sql.DB, proof *evidence.EventEvidenceV2) (transaction.FrozenPlan, error) {
	root, ok := ctx.Value(originalRootKey{}).(*originalRoot)
	if !ok || root == nil || root.closed.Load() {
		return transaction.FrozenPlan{}, transaction.ErrTransactionRequired
	}
	if root.primary != primary {
		return transaction.FrozenPlan{}, transaction.ErrBinding
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	pool := tx.Statement.ConnPool
	if prepared, ok := pool.(*gorm.PreparedStmtTX); ok {
		pool = prepared.Tx
	}
	if pool != root.root {
		return transaction.FrozenPlan{}, transaction.ErrBinding
	}
	if err := proof.Validate(); err != nil {
		return transaction.FrozenPlan{}, err
	}
	for _, b := range root.execution.Plan.Input().Branches {
		ref, err := root.execution.Plan.Reference(b.ID, root.execution.Fence, root.execution.WriterEpoch)
		if err != nil {
			return transaction.FrozenPlan{}, err
		}
		raw, err := transaction.EncodeReference(ref)
		if err == nil && bytes.Equal(raw, proof.TransactionReference) && standard.ReferenceFromMessage(b.Message) == proof.Reference {
			return root.execution.Plan, nil
		}
	}
	return transaction.FrozenPlan{}, transaction.ErrBinding
}
