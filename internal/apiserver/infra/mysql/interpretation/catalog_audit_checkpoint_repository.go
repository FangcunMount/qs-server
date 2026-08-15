package interpretation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	basemysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type catalogAuditCheckpointPO struct {
	CheckpointKey            string     `gorm:"column:checkpoint_key;primaryKey;size:64"`
	SchemaVersion            int        `gorm:"column:schema_version;not null"`
	Revision                 int64      `gorm:"column:revision;not null"`
	CycleID                  string     `gorm:"column:cycle_id;size:64;not null"`
	Phase                    string     `gorm:"column:phase;size:32;not null"`
	AfterAssessmentID        uint64     `gorm:"column:after_assessment_id;not null"`
	SourceUpperAssessmentID  uint64     `gorm:"column:source_upper_assessment_id;not null"`
	CatalogUpperAssessmentID uint64     `gorm:"column:catalog_upper_assessment_id;not null"`
	WorkingCountsJSON        string     `gorm:"column:working_counts_json;type:json;not null"`
	WorkingOrgCountsJSON     string     `gorm:"column:working_org_counts_json;type:json;not null"`
	LastCompletedJSON        *string    `gorm:"column:last_completed_json;type:json"`
	NextCycleAt              *time.Time `gorm:"column:next_cycle_at"`
	CreatedAt                time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt                time.Time  `gorm:"column:updated_at;not null"`
}

func (catalogAuditCheckpointPO) TableName() string { return "interpretation_catalog_audit_checkpoint" }

// CatalogAuditCheckpointRepository owns only the durable checkpoint. Catalog
// scanning and repair remain backed by Mongo report documents.
type CatalogAuditCheckpointRepository struct {
	db *gorm.DB
}

func NewCatalogAuditCheckpointRepository(db *gorm.DB) *CatalogAuditCheckpointRepository {
	return &CatalogAuditCheckpointRepository{db: db}
}

func (r *CatalogAuditCheckpointRepository) LoadAuditCheckpoint(ctx context.Context) (catalogreconcile.AuditCheckpoint, error) {
	if r == nil || r.db == nil {
		return catalogreconcile.AuditCheckpoint{}, fmt.Errorf("catalog audit checkpoint repository is not configured")
	}
	var po catalogAuditCheckpointPO
	err := r.db.WithContext(ctx).Where("checkpoint_key = ?", catalogreconcile.AuditCheckpointID).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return catalogreconcile.AuditCheckpoint{}, catalogreconcile.ErrAuditCheckpointMissing
	}
	if err != nil {
		return catalogreconcile.AuditCheckpoint{}, err
	}
	if po.SchemaVersion != catalogreconcile.AuditCheckpointSchema {
		return catalogreconcile.AuditCheckpoint{}, fmt.Errorf("unsupported catalog audit checkpoint schema version %d", po.SchemaVersion)
	}
	return catalogAuditCheckpointFromPO(po)
}

func (r *CatalogAuditCheckpointRepository) SaveAuditCheckpoint(ctx context.Context, expectedRevision int64, checkpoint catalogreconcile.AuditCheckpoint) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("catalog audit checkpoint repository is not configured")
	}
	if checkpoint.Revision != expectedRevision+1 {
		return fmt.Errorf("catalog audit checkpoint revision is invalid")
	}
	po, err := catalogAuditCheckpointToPO(checkpoint)
	if err != nil {
		return err
	}
	if expectedRevision == 0 {
		if err := r.db.WithContext(ctx).Create(&po).Error; err != nil {
			if basemysql.IsDuplicateError(err) {
				return catalogreconcile.ErrAuditCheckpointCAS
			}
			return err
		}
		return nil
	}
	result := r.db.WithContext(ctx).Model(&catalogAuditCheckpointPO{}).
		Where("checkpoint_key = ? AND revision = ?", catalogreconcile.AuditCheckpointID, expectedRevision).
		Updates(catalogAuditCheckpointUpdates(po))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return catalogreconcile.ErrAuditCheckpointCAS
	}
	return nil
}

func catalogAuditCheckpointUpdates(po catalogAuditCheckpointPO) map[string]any {
	return map[string]any{
		"schema_version":              po.SchemaVersion,
		"revision":                    po.Revision,
		"cycle_id":                    po.CycleID,
		"phase":                       po.Phase,
		"after_assessment_id":         po.AfterAssessmentID,
		"source_upper_assessment_id":  po.SourceUpperAssessmentID,
		"catalog_upper_assessment_id": po.CatalogUpperAssessmentID,
		"working_counts_json":         po.WorkingCountsJSON,
		"working_org_counts_json":     po.WorkingOrgCountsJSON,
		"last_completed_json":         po.LastCompletedJSON,
		"next_cycle_at":               po.NextCycleAt,
		"updated_at":                  po.UpdatedAt,
	}
}

func catalogAuditCheckpointToPO(checkpoint catalogreconcile.AuditCheckpoint) (catalogAuditCheckpointPO, error) {
	workingCounts, err := json.Marshal(checkpoint.WorkingCounts)
	if err != nil {
		return catalogAuditCheckpointPO{}, fmt.Errorf("marshal catalog audit working counts: %w", err)
	}
	workingOrgCounts := checkpoint.WorkingOrgCounts
	if workingOrgCounts == nil {
		workingOrgCounts = make(map[int64]catalogreconcile.DriftCounts)
	}
	workingOrgJSON, err := json.Marshal(workingOrgCounts)
	if err != nil {
		return catalogAuditCheckpointPO{}, fmt.Errorf("marshal catalog audit organization counts: %w", err)
	}
	var lastCompletedJSON *string
	if checkpoint.LastCompleted != nil {
		raw, marshalErr := json.Marshal(checkpoint.LastCompleted)
		if marshalErr != nil {
			return catalogAuditCheckpointPO{}, fmt.Errorf("marshal catalog audit completed snapshot: %w", marshalErr)
		}
		value := string(raw)
		lastCompletedJSON = &value
	}
	updatedAt := checkpoint.UpdatedAt.UTC()
	createdAt := updatedAt
	return catalogAuditCheckpointPO{
		CheckpointKey: catalogreconcile.AuditCheckpointID, SchemaVersion: checkpoint.SchemaVersion,
		Revision: checkpoint.Revision, CycleID: checkpoint.CycleID, Phase: checkpoint.Phase,
		AfterAssessmentID: checkpoint.AfterAssessmentID, SourceUpperAssessmentID: checkpoint.SourceUpperAssessmentID,
		CatalogUpperAssessmentID: checkpoint.CatalogUpperAssessmentID, WorkingCountsJSON: string(workingCounts),
		WorkingOrgCountsJSON: string(workingOrgJSON), LastCompletedJSON: lastCompletedJSON,
		NextCycleAt: optionalTime(checkpoint.NextCycleAt), CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}

func catalogAuditCheckpointFromPO(po catalogAuditCheckpointPO) (catalogreconcile.AuditCheckpoint, error) {
	checkpoint := catalogreconcile.AuditCheckpoint{
		SchemaVersion: po.SchemaVersion, Revision: po.Revision, CycleID: po.CycleID, Phase: po.Phase,
		AfterAssessmentID: po.AfterAssessmentID, SourceUpperAssessmentID: po.SourceUpperAssessmentID,
		CatalogUpperAssessmentID: po.CatalogUpperAssessmentID, UpdatedAt: po.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(po.WorkingCountsJSON), &checkpoint.WorkingCounts); err != nil {
		return catalogreconcile.AuditCheckpoint{}, fmt.Errorf("decode catalog audit working counts: %w", err)
	}
	if err := json.Unmarshal([]byte(po.WorkingOrgCountsJSON), &checkpoint.WorkingOrgCounts); err != nil {
		return catalogreconcile.AuditCheckpoint{}, fmt.Errorf("decode catalog audit organization counts: %w", err)
	}
	if checkpoint.WorkingOrgCounts == nil {
		checkpoint.WorkingOrgCounts = make(map[int64]catalogreconcile.DriftCounts)
	}
	if po.LastCompletedJSON != nil {
		var snapshot catalogreconcile.CompletedAuditSnapshot
		if err := json.Unmarshal([]byte(*po.LastCompletedJSON), &snapshot); err != nil {
			return catalogreconcile.AuditCheckpoint{}, fmt.Errorf("decode catalog audit completed snapshot: %w", err)
		}
		checkpoint.LastCompleted = &snapshot
	}
	if po.NextCycleAt != nil {
		checkpoint.NextCycleAt = *po.NextCycleAt
	}
	return checkpoint, nil
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
