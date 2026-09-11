package store

import (
	"context"
	stderrors "errors"
	"github.com/FangcunMount/component-base/pkg/errors"
	entryDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/actorstore"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// StorePO owns SQL mapping; domain aggregates never carry persistence tags.
type StorePO domain.State

func (StorePO) TableName() string { return "actor_stores" }

type HistoryPO port.History

func (HistoryPO) TableName() string { return "clinician_store_history" }

type Repository struct{ db *gorm.DB }

var _ port.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) dbFor(ctx context.Context) *gorm.DB {
	if tx, ok := dbctx.TxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}
func locked(ctx context.Context) (*gorm.DB, error) {
	tx, err := dbctx.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.Name() == "mysql" {
		tx = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx.WithContext(ctx), nil
}
func missing(err error) error {
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrUserNotFound, "record not found in current company")
	}
	return err
}
func conflict() error {
	return errors.WithCode(code.ErrConflict, "configuration changed; refresh and retry")
}
func (r *Repository) List(ctx context.Context, org int64, f port.Filter) (port.Page, error) {
	p := port.Page{Items: []port.Item{}}
	q := r.dbFor(ctx).Model(&StorePO{}).Where("org_id=?", org)
	if f.Search != "" {
		q = q.Where("(name LIKE ? OR code LIKE ?)", "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.Active != nil {
		q = q.Where("is_active=?", *f.Active)
	}
	if err := q.Count(&p.Total).Error; err != nil {
		return p, err
	}
	var rows []struct {
		StorePO
		ClinicianCount int64
	}
	err := q.Select("actor_stores.*, (SELECT COUNT(*) FROM clinician c WHERE c.store_id=actor_stores.id AND c.org_id=actor_stores.org_id AND c.deleted_at IS NULL) AS clinician_count").Order("id").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&rows).Error
	for _, row := range rows {
		p.Items = append(p.Items, port.Item{Store: domain.Restore(domain.State(row.StorePO)), ClinicianCount: row.ClinicianCount})
	}
	return p, err
}
func read(db *gorm.DB, org int64, id uint64) (*domain.Store, error) {
	var po StorePO
	if err := db.Where("org_id=? AND id=?", org, id).First(&po).Error; err != nil {
		return nil, missing(err)
	}
	return domain.Restore(domain.State(po)), nil
}
func (r *Repository) Get(ctx context.Context, org int64, id uint64) (*domain.Store, error) {
	return read(r.dbFor(ctx), org, id)
}
func (r *Repository) LockStore(ctx context.Context, org int64, id uint64) (*domain.Store, error) {
	db, err := locked(ctx)
	if err != nil {
		return nil, err
	}
	return read(db, org, id)
}
func (r *Repository) Create(ctx context.Context, s *domain.Store) error {
	po := StorePO(s.State())
	err := r.dbFor(ctx).Create(&po).Error
	var me *driver.MySQLError
	if stderrors.As(err, &me) && me.Number == 1062 {
		return errors.WithCode(code.ErrConflict, "store code already exists")
	}
	return err
}
func (r *Repository) Save(ctx context.Context, s *domain.Store, expected uint32) error {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return err
	}
	res := db.Model(&StorePO{}).Where("org_id=? AND id=? AND version=?", s.OrgID(), s.ID(), expected).Updates(map[string]any{"name": s.Name(), "address": s.Address(), "is_active": s.IsActive(), "version": s.Version(), "updated_at": s.UpdatedAt(), "updated_by": s.UpdatedBy()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return conflict()
	}
	return nil
}
func (r *Repository) CountClinicians(ctx context.Context, org int64, id uint64) (int64, error) {
	var n int64
	err := r.dbFor(ctx).Table("clinician").Where("org_id=? AND store_id=? AND deleted_at IS NULL", org, id).Count(&n).Error
	return n, err
}
func (r *Repository) LockClinician(ctx context.Context, org int64, id uint64) (*clinicianDomain.Clinician, error) {
	db, err := locked(ctx)
	if err != nil {
		return nil, err
	}
	var c actorInfra.ClinicianPO
	err = db.Table("clinician").Where("org_id=? AND id=? AND deleted_at IS NULL", org, id).Take(&c).Error
	if err != nil {
		return nil, missing(err)
	}
	return actorInfra.NewClinicianMapper().ToDomain(&c), nil
}
func (r *Repository) FindChange(ctx context.Context, org int64, id uint64, request string) (*port.History, error) {
	db, err := locked(ctx)
	if err != nil {
		return nil, err
	}
	var h HistoryPO
	err = db.Where("org_id=? AND clinician_id=? AND request_id=?", org, id, request).First(&h).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := port.History(h)
	return &v, nil
}
func (r *Repository) InvalidateEntries(ctx context.Context, org int64, id uint64, actor int64, now time.Time) (int64, error) {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return 0, err
	}
	res := db.Table("assessment_entry").Where("org_id=? AND clinician_id=? AND deleted_at IS NULL AND invalidated_at IS NULL", org, id).Updates(map[string]any{"invalidated_at": now, "invalidation_reason": entryDomain.InvalidationReasonStoreTransfer, "is_active": false, "updated_at": now, "updated_by": actor, "version": gorm.Expr("version + 1")})
	return res.RowsAffected, res.Error
}
func (r *Repository) SaveAssignment(ctx context.Context, h *port.History) error {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return err
	}
	res := db.Table("clinician").Where("id=? AND org_id=? AND version=? AND deleted_at IS NULL", h.ClinicianID, h.OrgID, h.Version-1).Updates(map[string]any{"store_id": h.ToStoreID, "version": h.Version, "updated_at": h.CreatedAt, "updated_by": h.ActorID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return conflict()
	}
	return nil
}
func (r *Repository) AppendHistory(ctx context.Context, h *port.History) error {
	db, err := dbctx.RequireTx(ctx)
	if err != nil {
		return err
	}
	po := HistoryPO(*h)
	return db.Create(&po).Error
}
func (r *Repository) History(ctx context.Context, org int64, id uint64) ([]port.History, error) {
	var c actorInfra.ClinicianPO
	if err := r.dbFor(ctx).Table("clinician").Where("org_id=? AND id=? AND deleted_at IS NULL", org, id).Take(&c).Error; err != nil {
		return nil, missing(err)
	}
	var rows []HistoryPO
	err := r.dbFor(ctx).Where("org_id=? AND clinician_id=?", org, id).Order("created_at DESC,id DESC").Find(&rows).Error
	items := make([]port.History, 0, len(rows))
	for _, h := range rows {
		items = append(items, port.History(h))
	}
	return items, err
}

func (r *Repository) Progress(ctx context.Context, org int64) (port.Progress, error) {
	var p port.Progress
	err := r.dbFor(ctx).Table("clinician").Where("org_id=? AND deleted_at IS NULL", org).Select("COUNT(*) AS total, COALESCE(SUM(store_id IS NOT NULL),0) AS configured, COALESCE(SUM(store_id IS NULL),0) AS unconfigured, COALESCE(SUM(is_active),0) AS active_total, COALESCE(SUM(is_active AND store_id IS NOT NULL),0) AS active_configured, COALESCE(SUM(is_active AND store_id IS NULL),0) AS active_unconfigured").Scan(&p).Error
	return p, err
}
