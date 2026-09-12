package answeringstart

import (
	"context"
	"errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/answeringstart"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"time"
)

type PO struct {
	ID                   uint64 `gorm:"primaryKey;autoIncrement:false"`
	OrgID                int64
	UserID               uint64 `gorm:"column:started_by_user_id"`
	TesteeID             uint64
	RequestKey           string
	RequestHash          string
	QuestionnaireCode    string
	QuestionnaireVersion string
	ModelCode            string
	ModelVersion         string
	OriginType           string
	OriginID             string
	StartedAt            time.Time
	ConductingStoreID    *uint64
	OwnershipVersion     uint32
	ContractVersion      uint32
}

func (PO) TableName() string { return "answering_start" }

type Repository struct{ db *gorm.DB }

var _ port.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) reader(ctx context.Context) *gorm.DB {
	if tx, ok := dbctx.TxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}
func (r *Repository) FindRequest(ctx context.Context, org int64, user uint64, key string) (*domain.Record, error) {
	var po PO
	err := r.reader(ctx).Where("org_id=? AND started_by_user_id=? AND request_key=?", org, user, key).Take(&po).Error
	return restore(&po, err)
}
func (r *Repository) Find(ctx context.Context, id uint64) (*domain.Record, error) {
	var po PO
	return restore(&po, r.reader(ctx).Where("id=?", id).Take(&po).Error)
}
func (r *Repository) Insert(ctx context.Context, record *domain.Record) error {
	tx, err := dbctx.RequireTx(ctx)
	if err != nil {
		return err
	}
	i, c := record.Intent(), record.Context()
	po := PO{ID: c.ID(), OrgID: i.OrgID, UserID: i.UserID, TesteeID: i.TesteeID, RequestKey: i.RequestKey, RequestHash: record.Hash(), QuestionnaireCode: i.QuestionnaireCode, QuestionnaireVersion: i.QuestionnaireVersion, ModelCode: i.ModelCode, ModelVersion: i.ModelVersion, OriginType: string(i.Origin.Type), OriginID: i.Origin.ID, StartedAt: c.StartedAt(), ConductingStoreID: c.StoreID(), OwnershipVersion: c.OwnershipVersion(), ContractVersion: c.Version()}
	err = tx.WithContext(ctx).Create(&po).Error
	var duplicate *mysqldriver.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return port.ErrDuplicate
	}
	return err
}
func restore(po *PO, err error) (*domain.Record, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := sheet.RestoreStartContext(po.ID, po.StartedAt, po.ConductingStoreID, po.OwnershipVersion, po.ContractVersion)
	if err != nil {
		return nil, err
	}
	return domain.Restore(domain.Intent{OrgID: po.OrgID, UserID: po.UserID, TesteeID: po.TesteeID, RequestKey: po.RequestKey, QuestionnaireCode: po.QuestionnaireCode, QuestionnaireVersion: po.QuestionnaireVersion, ModelCode: po.ModelCode, ModelVersion: po.ModelVersion, Origin: sheet.OriginRef{Type: sheet.OriginType(po.OriginType), ID: po.OriginID}}, po.RequestHash, c)
}
