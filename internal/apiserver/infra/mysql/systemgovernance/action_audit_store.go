package systemgovernance

import (
	"context"
	"encoding/json"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type actionRunPO struct {
	ID             uint64     `gorm:"column:id;primaryKey"`
	RequestID      string     `gorm:"column:request_id"`
	ActionID       string     `gorm:"column:action_id"`
	OrgID          int64      `gorm:"column:org_id"`
	ActorUserID    uint64     `gorm:"column:actor_user_id"`
	Component      string     `gorm:"column:component"`
	TargetInstance string     `gorm:"column:target_instance"`
	InputJSON      string     `gorm:"column:input_json"`
	Status         string     `gorm:"column:status"`
	ResultJSON     string     `gorm:"column:result_json"`
	StartedAt      time.Time  `gorm:"column:started_at"`
	FinishedAt     *time.Time `gorm:"column:finished_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (actionRunPO) TableName() string { return "system_governance_action_runs" }

type ActionAuditStore struct{ db *gorm.DB }

type actionAuditEnvelope struct {
	SchemaVersion int                   `json:"schema_version"`
	Result        *app.ActionRunResult  `json:"result,omitempty"`
	Error         *app.ActionAuditError `json:"error,omitempty"`
}

func NewActionAuditStore(db *gorm.DB) *ActionAuditStore { return &ActionAuditStore{db: db} }

func (s *ActionAuditStore) Claim(ctx context.Context, record app.ActionAuditRecord) (*app.ActionAuditReplay, bool, error) {
	input, err := json.Marshal(record.Input)
	if err != nil {
		return nil, false, err
	}
	row := actionRunPO{
		RequestID: record.RequestID, ActionID: record.ActionID, OrgID: record.OrgID,
		ActorUserID: record.ActorUserID, Component: record.Component,
		TargetInstance: record.TargetInstance, InputJSON: string(input),
		Status: "running", StartedAt: record.StartedAt,
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return nil, true, nil
	}
	var existing actionRunPO
	if err := s.db.WithContext(ctx).Where("org_id = ? AND request_id = ?", record.OrgID, record.RequestID).Take(&existing).Error; err != nil {
		return nil, false, err
	}
	if existing.Status == "running" || existing.ResultJSON == "" {
		return nil, false, nil
	}
	prior, err := decodeActionAuditReplay(existing.ResultJSON)
	if prior != nil {
		prior.ActionID = existing.ActionID
	}
	return prior, false, err
}

func (s *ActionAuditStore) Complete(ctx context.Context, record app.ActionAuditRecord) error {
	resultJSON := ""
	if record.Result != nil || record.Error != nil {
		encoded, err := json.Marshal(actionAuditEnvelope{SchemaVersion: 2, Result: record.Result, Error: record.Error})
		if err != nil {
			return err
		}
		resultJSON = string(encoded)
	}
	updates := map[string]interface{}{
		"status": record.Status, "result_json": resultJSON,
		"finished_at": record.FinishedAt, "updated_at": time.Now(),
	}
	result := s.db.WithContext(ctx).Model(&actionRunPO{}).
		Where("org_id = ? AND request_id = ? AND status = ?", record.OrgID, record.RequestID, "running").
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func decodeActionAuditReplay(raw string) (*app.ActionAuditReplay, error) {
	var envelope actionAuditEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, err
	}
	if envelope.SchemaVersion >= 2 {
		return &app.ActionAuditReplay{Result: envelope.Result, Error: envelope.Error}, nil
	}
	var legacy app.ActionRunResult
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return nil, err
	}
	return &app.ActionAuditReplay{ActionID: legacy.ActionID, Result: &legacy}, nil
}

var _ app.ActionAuditStore = (*ActionAuditStore)(nil)
