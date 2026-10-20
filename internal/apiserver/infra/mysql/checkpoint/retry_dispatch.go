package checkpoint

import (
	"context"
	"database/sql"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ port.AuthorityValidator = (*Repository)(nil)

// ValidateDispatchAuthority competes with the current Run in the original
// root. It does not authorize a business retry, create a Run or change attempt.
func (r *Repository) ValidateDispatchAuthority(ctx context.Context, f model.Frozen) error {
	if r == nil || r.db == nil || !f.Valid() || f.Authority().Owner != "qs.mysql" || f.Authority().Kind != model.Evaluation {
		return transaction.ErrBinding
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	pool := tx.Statement.ConnPool
	if prepared, ok := pool.(*gorm.PreparedStmtTX); ok {
		pool = prepared.Tx
	}
	raw, ok := pool.(*sql.Tx)
	if !ok || raw == nil {
		return transaction.ErrTransactionRequired
	}
	view := tx.WithContext(ctx)
	view.Statement.ConnPool = raw
	actual, err := view.DB()
	if err != nil {
		return err
	}
	primary, err := r.db.DB()
	if err != nil {
		return err
	}
	if actual != primary {
		return transaction.ErrBinding
	}
	var po RuntimeCheckpointPO
	err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("scope = ? AND assessment_id = ? AND deleted_at IS NULL", scopeEvaluationRun, f.Authority().AssessmentID).Order("attempt_no DESC, id DESC").First(&po).Error
	if err != nil {
		return err
	}
	current := runFromPO(po)
	if current.ID().String() != f.Authority().RunID || current.Attempt().Status != evalrun.StatusFailed {
		return transaction.ErrStaleExecution
	}
	// go-sql-driver rounds to microseconds; the original business column is
	// datetime(3). This is not the separate schedule index's upward rounding.
	return f.MatchDecision(current.RetryDecision(), f.DueAt().Round(time.Microsecond).Round(time.Millisecond))
}
