package testeestore

import (
	"context"
	stderrors "errors"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	storeinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/store"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/testeestore"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HistoryPO port.History

func (HistoryPO) TableName() string { return "testee_store_history" }

type Repository struct{ db *gorm.DB }

var _ port.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func missing(err error) error {
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrUserNotFound, "record not found in current company")
	}
	return err
}
func locked(ctx context.Context) (*gorm.DB, error) {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	db = db.WithContext(ctx)
	if db.Name() == "mysql" {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return db, nil
}
func (r *Repository) LockStore(ctx context.Context, org int64, id uint64) (*store.Store, error) {
	return storeinfra.NewRepository(r.db).LockStore(ctx, org, id)
}
func (r *Repository) LockTestee(ctx context.Context, org int64, id uint64) (*testee.Testee, error) {
	db, err := locked(ctx)
	if err != nil {
		return nil, err
	}
	var po actor.TesteePO
	if err = db.Where("org_id=? AND id=? AND deleted_at IS NULL", org, id).First(&po).Error; err != nil {
		return nil, missing(err)
	}
	return actor.NewTesteeMapper().ToDomain(&po), nil
}
func (r *Repository) FindChange(ctx context.Context, org int64, id uint64, request string) (*port.History, error) {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	var po HistoryPO
	err = db.WithContext(ctx).Where("org_id=? AND testee_id=? AND request_id=?", org, id, request).First(&po).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h := port.History(po)
	return &h, nil
}
func (r *Repository) SaveOwnership(ctx context.Context, h *port.History, expected uint32) error {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return err
	}
	q := db.WithContext(ctx).Model(&actor.TesteePO{}).Where("org_id=? AND id=? AND store_version=? AND deleted_at IS NULL", h.OrgID, h.TesteeID, expected)
	if h.FromStoreID == nil {
		q = q.Where("store_id IS NULL")
	} else {
		q = q.Where("store_id=?", *h.FromStoreID)
	}
	result := q.Updates(map[string]any{"store_id": h.ToStoreID, "store_version": h.Version, "updated_at": h.CreatedAt, "updated_by": h.ActorID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.WithCode(code.ErrConflict, "store ownership changed; refresh and retry")
	}
	return nil
}
func (r *Repository) AppendHistory(ctx context.Context, h *port.History) error {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return err
	}
	po := HistoryPO(*h)
	return db.WithContext(ctx).Create(&po).Error
}
func (r *Repository) History(ctx context.Context, org int64, id, before uint64, limit int) ([]port.History, error) {
	db := r.db.WithContext(ctx)
	if tx, ok := dbctx.TxFromContext(ctx); ok {
		db = tx.WithContext(ctx)
	}
	var subject actor.TesteePO
	if err := db.Select("id").Where("org_id=? AND id=? AND deleted_at IS NULL", org, id).First(&subject).Error; err != nil {
		return nil, missing(err)
	}
	q := db.Model(&HistoryPO{}).Where("org_id=? AND testee_id=?", org, id)
	if before > 0 {
		q = q.Where("id<?", before)
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows := []HistoryPO{}
	if err := q.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]port.History, len(rows))
	for i, row := range rows {
		result[i] = port.History(row)
	}
	return result, nil
}

func (r *Repository) ReadClinicianStore(ctx context.Context, org int64, id uint64) (*uint64, error) {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	var row struct{ StoreID *uint64 }
	err = db.WithContext(ctx).Table("clinician").Select("store_id").Where("org_id=? AND id=? AND deleted_at IS NULL", org, id).Take(&row).Error
	if err != nil {
		return nil, missing(err)
	}
	return row.StoreID, nil
}
