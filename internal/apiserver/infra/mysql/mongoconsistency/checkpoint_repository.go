package mongoconsistency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	basemysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type checkpointPO struct {
	CheckpointKey     string     `gorm:"column:checkpoint_key;primaryKey;size:64"`
	SchemaVersion     int        `gorm:"column:schema_version;not null"`
	Revision          int64      `gorm:"column:revision;not null"`
	CycleID           string     `gorm:"column:cycle_id;size:64;not null"`
	Phase             string     `gorm:"column:phase;size:64;not null"`
	Cursor            uint64     `gorm:"column:cursor;not null"`
	CycleUpperBound   uint64     `gorm:"column:cycle_upper_bound;not null"`
	StatisticsJSON    string     `gorm:"column:statistics_json;type:json;not null"`
	LastCompletedJSON *string    `gorm:"column:last_completed_json;type:json"`
	NextCycleAt       *time.Time `gorm:"column:next_cycle_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`
}

func (checkpointPO) TableName() string { return "mongo_consistency_audit_checkpoint" }

type CheckpointRepository struct{ db *gorm.DB }

func NewCheckpointRepository(db *gorm.DB) *CheckpointRepository {
	return &CheckpointRepository{db: db}
}

func (r *CheckpointRepository) Load(ctx context.Context) (appaudit.Checkpoint, error) {
	if r == nil || r.db == nil {
		return appaudit.Checkpoint{}, fmt.Errorf("mongo consistency checkpoint repository is not configured")
	}
	var po checkpointPO
	err := r.db.WithContext(ctx).Where("checkpoint_key = ?", appaudit.CheckpointKey).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return appaudit.Checkpoint{}, appaudit.ErrCheckpointMissing
	}
	if err != nil {
		return appaudit.Checkpoint{}, err
	}
	if po.SchemaVersion != appaudit.CheckpointSchemaVersion {
		return appaudit.Checkpoint{}, fmt.Errorf("unsupported mongo consistency checkpoint schema %d", po.SchemaVersion)
	}
	return fromPO(po)
}

func (r *CheckpointRepository) Save(ctx context.Context, expectedRevision int64, checkpoint appaudit.Checkpoint) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("mongo consistency checkpoint repository is not configured")
	}
	if checkpoint.Revision != expectedRevision+1 {
		return fmt.Errorf("mongo consistency checkpoint revision is invalid")
	}
	po, err := toPO(checkpoint)
	if err != nil {
		return err
	}
	if expectedRevision == 0 {
		if err := r.db.WithContext(ctx).Create(&po).Error; err != nil {
			if basemysql.IsDuplicateError(err) {
				return appaudit.ErrCheckpointCAS
			}
			return err
		}
		return nil
	}
	result := r.db.WithContext(ctx).Model(&checkpointPO{}).
		Where("checkpoint_key = ? AND revision = ?", appaudit.CheckpointKey, expectedRevision).
		Updates(map[string]any{
			"schema_version": po.SchemaVersion, "revision": po.Revision, "cycle_id": po.CycleID,
			"phase": po.Phase, "cursor": po.Cursor, "cycle_upper_bound": po.CycleUpperBound,
			"statistics_json": po.StatisticsJSON, "last_completed_json": po.LastCompletedJSON,
			"next_cycle_at": po.NextCycleAt, "updated_at": po.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return appaudit.ErrCheckpointCAS
	}
	return nil
}

func toPO(checkpoint appaudit.Checkpoint) (checkpointPO, error) {
	stats, err := json.Marshal(checkpoint.Working)
	if err != nil {
		return checkpointPO{}, fmt.Errorf("marshal mongo consistency statistics: %w", err)
	}
	var completed *string
	if checkpoint.LastCompleted != nil {
		raw, marshalErr := json.Marshal(checkpoint.LastCompleted)
		if marshalErr != nil {
			return checkpointPO{}, fmt.Errorf("marshal mongo consistency completed cycle: %w", marshalErr)
		}
		value := string(raw)
		completed = &value
	}
	updatedAt := checkpoint.UpdatedAt.UTC()
	return checkpointPO{
		CheckpointKey: appaudit.CheckpointKey, SchemaVersion: checkpoint.SchemaVersion,
		Revision: checkpoint.Revision, CycleID: checkpoint.CycleID, Phase: string(checkpoint.Phase),
		Cursor: checkpoint.Cursor, CycleUpperBound: checkpoint.UpperBound, StatisticsJSON: string(stats),
		LastCompletedJSON: completed, NextCycleAt: optionalTime(checkpoint.NextCycleAt),
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}, nil
}

func fromPO(po checkpointPO) (appaudit.Checkpoint, error) {
	checkpoint := appaudit.Checkpoint{
		SchemaVersion: po.SchemaVersion, Revision: po.Revision, CycleID: po.CycleID,
		Phase: appaudit.Phase(po.Phase), Cursor: po.Cursor, UpperBound: po.CycleUpperBound,
		UpdatedAt: po.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(po.StatisticsJSON), &checkpoint.Working); err != nil {
		return appaudit.Checkpoint{}, fmt.Errorf("decode mongo consistency statistics: %w", err)
	}
	if checkpoint.Working.Findings == nil {
		checkpoint.Working.Findings = make(map[string]int64)
	}
	if checkpoint.Working.Samples == nil {
		checkpoint.Working.Samples = make(map[string][]string)
	}
	if po.LastCompletedJSON != nil {
		var completed appaudit.CompletedCycle
		if err := json.Unmarshal([]byte(*po.LastCompletedJSON), &completed); err != nil {
			return appaudit.Checkpoint{}, fmt.Errorf("decode mongo consistency completed cycle: %w", err)
		}
		checkpoint.LastCompleted = &completed
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
