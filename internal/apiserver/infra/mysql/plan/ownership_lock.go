package plan

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm/clause"
)

// OwnershipLocker shares the Testee row lock used by Actor's store transfer.
// It has no standalone DB handle: an active caller transaction is mandatory.
type OwnershipLocker struct{}

func (OwnershipLocker) LockTesteeStore(ctx context.Context, orgID int64, id uint64) (*uint64, error) {
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if orgID <= 0 || id == 0 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "testee company required")
	}
	var row struct{ StoreID *uint64 }
	err = tx.WithContext(ctx).Table("testee").Select("store_id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, id).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return row.StoreID, nil
}
