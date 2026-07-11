package evaluation

import (
	"context"
	"fmt"
	"time"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

type EvaluationOutcomePO struct {
	ID               uint64    `gorm:"column:id;primaryKey"`
	AssessmentID     uint64    `gorm:"column:assessment_id;not null;uniqueIndex:uk_evaluation_outcome_assessment_id"`
	EvaluationRunID  string    `gorm:"column:evaluation_run_id;size:128;not null;uniqueIndex:uk_evaluation_outcome_run_id"`
	ModelKind        string    `gorm:"column:model_kind;size:50;not null"`
	ModelSubKind     *string   `gorm:"column:model_sub_kind;size:50"`
	ModelAlgorithm   *string   `gorm:"column:model_algorithm;size:50"`
	ModelCode        string    `gorm:"column:model_code;size:100;not null"`
	ModelVersion     string    `gorm:"column:model_version;size:50;not null"`
	ModelTitle       *string   `gorm:"column:model_title;size:255"`
	AlgorithmFamily  *string   `gorm:"column:algorithm_family;size:50"`
	DecisionKind     *string   `gorm:"column:decision_kind;size:50"`
	PayloadFormat    *string   `gorm:"column:payload_format;size:100"`
	InputSnapshotRef *string   `gorm:"column:input_snapshot_ref;size:200"`
	PayloadJSON      string    `gorm:"column:payload_json;type:longtext;not null"`
	SchemaVersion    uint      `gorm:"column:schema_version;not null"`
	EvaluatedAt      time.Time `gorm:"column:evaluated_at;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null"`
}

func (EvaluationOutcomePO) TableName() string { return "evaluation_outcome" }

type outcomeRepository struct {
	db *gorm.DB
}

func NewOutcomeRepository(db *gorm.DB) domainoutcome.Repository {
	return &outcomeRepository{db: db}
}

func (r *outcomeRepository) Save(ctx context.Context, record *domainoutcome.Record) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("evaluation outcome repository is not configured")
	}
	if record == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	po := outcomeToPO(record)
	return dbWithTransactionContext(ctx, r.db).Create(po).Error
}

func (r *outcomeRepository) FindByID(ctx context.Context, id domainoutcome.ID) (*domainoutcome.Record, error) {
	var po EvaluationOutcomePO
	if err := dbWithTransactionContext(ctx, r.db).First(&po, id.Uint64()).Error; err != nil {
		return nil, err
	}
	return outcomeFromPO(&po)
}

func (r *outcomeRepository) FindByAssessmentID(ctx context.Context, assessmentID meta.ID) (*domainoutcome.Record, error) {
	var po EvaluationOutcomePO
	if err := dbWithTransactionContext(ctx, r.db).Where("assessment_id = ?", assessmentID.Uint64()).First(&po).Error; err != nil {
		return nil, err
	}
	return outcomeFromPO(&po)
}

func outcomeToPO(record *domainoutcome.Record) *EvaluationOutcomePO {
	model := record.Model()
	runtime := record.Runtime()
	return &EvaluationOutcomePO{
		ID:               record.ID().Uint64(),
		AssessmentID:     record.AssessmentID().Uint64(),
		EvaluationRunID:  record.RunID(),
		ModelKind:        model.Kind.String(),
		ModelSubKind:     optionalString(string(model.SubKind)),
		ModelAlgorithm:   optionalString(string(model.Algorithm)),
		ModelCode:        model.Code,
		ModelVersion:     model.Version,
		ModelTitle:       optionalString(model.Title),
		AlgorithmFamily:  optionalString(runtime.AlgorithmFamily.String()),
		DecisionKind:     optionalString(string(runtime.DecisionKind)),
		PayloadFormat:    optionalString(runtime.PayloadFormat),
		InputSnapshotRef: optionalString(record.InputSnapshotRef()),
		PayloadJSON:      string(record.Payload()),
		SchemaVersion:    record.SchemaVersion(),
		EvaluatedAt:      record.EvaluatedAt(),
		CreatedAt:        record.EvaluatedAt(),
	}
}

func outcomeFromPO(po *EvaluationOutcomePO) (*domainoutcome.Record, error) {
	if po == nil {
		return nil, nil
	}
	return domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           meta.FromUint64(po.ID),
		AssessmentID: meta.FromUint64(po.AssessmentID),
		RunID:        po.EvaluationRunID,
		Model: domainoutcome.ModelIdentity{
			Kind:      modelcatalog.Kind(po.ModelKind),
			SubKind:   modelcatalog.SubKind(valueOrEmpty(po.ModelSubKind)),
			Algorithm: modelcatalog.Algorithm(valueOrEmpty(po.ModelAlgorithm)),
			Code:      po.ModelCode,
			Version:   po.ModelVersion,
			Title:     valueOrEmpty(po.ModelTitle),
		},
		Runtime: domainoutcome.RuntimeIdentity{
			AlgorithmFamily: modelcatalog.AlgorithmFamily(valueOrEmpty(po.AlgorithmFamily)),
			DecisionKind:    modelcatalog.DecisionKind(valueOrEmpty(po.DecisionKind)),
			PayloadFormat:   valueOrEmpty(po.PayloadFormat),
		},
		InputSnapshotRef: valueOrEmpty(po.InputSnapshotRef),
		Payload:          []byte(po.PayloadJSON),
		SchemaVersion:    po.SchemaVersion,
		EvaluatedAt:      po.EvaluatedAt,
	})
}

func dbWithTransactionContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := mysql.TxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
