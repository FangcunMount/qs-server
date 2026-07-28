package historicalseedstage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

type stagePO struct {
	ID           uint64    `gorm:"column:id;primaryKey"`
	OrgID        uint64    `gorm:"column:org_id"`
	BatchID      string    `gorm:"column:batch_id"`
	ScenarioID   string    `gorm:"column:scenario_id"`
	Stage        string    `gorm:"column:stage"`
	PayloadHash  string    `gorm:"column:payload_hash"`
	Status       string    `gorm:"column:status"`
	BusinessAt   time.Time `gorm:"column:business_at"`
	ResourceType string    `gorm:"column:resource_type"`
	ResourceID   string    `gorm:"column:resource_id"`
	PayloadJSON  []byte    `gorm:"column:payload_json"`
	ErrorText    *string   `gorm:"column:error_text"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (stagePO) TableName() string { return "seed_backfill_stage" }

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Complete(ctx context.Context, completion stageport.Completion) (*stageport.Record, error) {
	historical, ok := historicalseed.FromContext(ctx)
	if !ok {
		return nil, nil
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("historical seed stage repository is not configured")
	}
	if strings.TrimSpace(completion.Stage) == "" || completion.BusinessAt.IsZero() || strings.TrimSpace(completion.ResourceType) == "" || strings.TrimSpace(completion.ResourceID) == "" {
		return nil, fmt.Errorf("historical seed stage completion is incomplete")
	}
	payload, err := json.Marshal(completion.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal historical seed stage payload: %w", err)
	}
	fingerprint, err := json.Marshal(struct {
		BusinessAt   time.Time       `json:"business_at"`
		ResourceType string          `json:"resource_type"`
		ResourceID   string          `json:"resource_id"`
		Payload      json.RawMessage `json:"payload"`
	}{BusinessAt: completion.BusinessAt, ResourceType: completion.ResourceType, ResourceID: completion.ResourceID, Payload: payload})
	if err != nil {
		return nil, fmt.Errorf("marshal historical seed stage fingerprint: %w", err)
	}
	digest := sha256.Sum256(fingerprint)
	hash := hex.EncodeToString(digest[:])
	db := r.db.WithContext(ctx)
	if tx, ok := mysql.TxFromContext(ctx); ok {
		db = tx.WithContext(ctx)
	}
	var existing stagePO
	find := db.Where("org_id = ? AND batch_id = ? AND scenario_id = ? AND stage = ?", historical.OrgID, historical.BatchID, historical.ScenarioID, completion.Stage).First(&existing)
	if find.Error == nil {
		return compareExisting(existing, hash)
	}
	if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
		return nil, find.Error
	}
	row := stagePO{ID: meta.New().Uint64(), OrgID: historical.OrgID, BatchID: historical.BatchID, ScenarioID: historical.ScenarioID, Stage: completion.Stage, PayloadHash: hash, Status: "completed", BusinessAt: completion.BusinessAt, ResourceType: completion.ResourceType, ResourceID: completion.ResourceID, PayloadJSON: payload}
	if err := db.Create(&row).Error; err != nil {
		var winner stagePO
		if lookupErr := db.Where("org_id = ? AND batch_id = ? AND scenario_id = ? AND stage = ?", historical.OrgID, historical.BatchID, historical.ScenarioID, completion.Stage).First(&winner).Error; lookupErr == nil {
			return compareExisting(winner, hash)
		}
		return nil, err
	}
	return toRecord(row), nil
}

func (r *Repository) FindCurrent(ctx context.Context, stage string) (*stageport.Record, error) {
	historical, ok := historicalseed.FromContext(ctx)
	if !ok {
		return nil, nil
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("historical seed stage repository is not configured")
	}
	db := r.db.WithContext(ctx)
	if tx, ok := mysql.TxFromContext(ctx); ok {
		db = tx.WithContext(ctx)
	}
	var row stagePO
	err := db.Where("org_id = ? AND batch_id = ? AND scenario_id = ? AND stage = ?", historical.OrgID, historical.BatchID, historical.ScenarioID, strings.TrimSpace(stage)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRecord(row), nil
}

func compareExisting(existing stagePO, hash string) (*stageport.Record, error) {
	if existing.PayloadHash != hash {
		return nil, fmt.Errorf("%w: batch=%s scenario=%s stage=%s", stageport.ErrPayloadConflict, existing.BatchID, existing.ScenarioID, existing.Stage)
	}
	return toRecord(existing), nil
}

func (r *Repository) ListScenario(ctx context.Context, orgID uint64, batchID, scenarioID string) ([]stageport.Record, error) {
	var rows []stagePO
	if err := r.db.WithContext(ctx).Where("org_id = ? AND batch_id = ? AND scenario_id = ?", orgID, batchID, scenarioID).Order("business_at ASC, stage ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return toRecords(rows), nil
}

func (r *Repository) ListBatch(ctx context.Context, orgID uint64, batchID string, offset, limit int) ([]stageport.Record, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	var rows []stagePO
	if err := r.db.WithContext(ctx).Where("org_id = ? AND batch_id = ?", orgID, batchID).Order("scenario_id ASC, business_at ASC, stage ASC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return toRecords(rows), nil
}

func toRecords(rows []stagePO) []stageport.Record {
	result := make([]stageport.Record, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toRecord(row))
	}
	return result
}

func toRecord(row stagePO) *stageport.Record {
	errorText := ""
	if row.ErrorText != nil {
		errorText = *row.ErrorText
	}
	return &stageport.Record{ID: row.ID, OrgID: row.OrgID, BatchID: row.BatchID, ScenarioID: row.ScenarioID, Stage: row.Stage, PayloadHash: row.PayloadHash, Status: row.Status, BusinessAt: row.BusinessAt, ResourceType: row.ResourceType, ResourceID: row.ResourceID, PayloadJSON: append(json.RawMessage(nil), row.PayloadJSON...), ErrorText: errorText, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
