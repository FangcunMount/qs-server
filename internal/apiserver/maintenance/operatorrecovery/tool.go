// Package operatorrecovery provides restricted, explicit recovery of previously retired identities.
// It never reassigns roles or edits original retirement manifests.
package operatorrecovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"gorm.io/gorm"
)

type SnapshotReader interface {
	LoadFresh(context.Context, string) (*authz.Snapshot, error)
}
type Report struct {
	State           string                 `json:"state"`
	Command         app.RecoveryCommand    `json:"command"`
	Fingerprint     string                 `json:"fingerprint,omitempty"`
	Recovery        *domain.Recovery       `json:"recovery,omitempty"`
	CurrentOperator map[string]interface{} `json:"current_operator,omitempty"`
	LastError       string                 `json:"last_error,omitempty"`
}
type Tool struct {
	db      *gorm.DB
	loader  SnapshotReader
	service *app.Service
	repo    *repository.Repository
}

func New(db *gorm.DB, loader SnapshotReader, gateway iambridge.OperatorAuthzGateway) *Tool {
	repo := repository.NewRepository(db)
	return &Tool{db: db, loader: loader, service: app.NewMaintenanceService(repo, gateway), repo: repo}
}
func (t *Tool) administrator(ctx context.Context, cmd app.RecoveryCommand) (context.Context, error) {
	if t.db == nil || t.loader == nil {
		return ctx, fmt.Errorf("recovery dependencies unavailable")
	}
	snapshot, err := t.loader.LoadFresh(ctx, strconv.FormatInt(cmd.ActorID, 10))
	if err != nil {
		return ctx, err
	}
	if snapshot == nil || !snapshot.IsQSAdmin() {
		return ctx, fmt.Errorf("headquarters permission required")
	}
	var count int64
	if err = t.db.WithContext(ctx).Table("operators").Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", cmd.OrgID, cmd.ActorID).Count(&count).Error; err != nil {
		return ctx, err
	}
	if count != 1 {
		return ctx, fmt.Errorf("active headquarters identity required")
	}
	if err = t.db.WithContext(ctx).Table("operator_retirement_tasks").Where("user_id=?", cmd.ActorID).Count(&count).Error; err != nil {
		return ctx, err
	}
	if count != 0 {
		return ctx, fmt.Errorf("headquarters identity is retiring")
	}
	return authz.WithSnapshot(ctx, snapshot), nil
}
func (t *Tool) current(ctx context.Context, cmd app.RecoveryCommand) (map[string]interface{}, error) {
	var current map[string]interface{}
	err := t.db.WithContext(ctx).Table("operators").Where("org_id=? AND id=?", cmd.OrgID, cmd.OperatorID).Take(&current).Error
	return current, err
}

// Status reports historical completion independently of current identity facts.
// Later changes do not grant permission to replay the recovery or rewrite its archive.
func (t *Tool) Status(ctx context.Context, cmd app.RecoveryCommand) (*Report, error) {
	ctx, err := t.administrator(ctx, cmd)
	if err != nil {
		return nil, err
	}
	receipt, err := t.repo.FindRecovery(ctx, cmd.RequestID)
	if err != nil {
		return nil, err
	}
	report := &Report{State: "pending", Command: cmd, Recovery: receipt}
	if receipt != nil {
		if receipt.OrgID != cmd.OrgID || receipt.OperatorID != cmd.OperatorID || receipt.ActorID != cmd.ActorID || receipt.ExpectedVersion != cmd.ExpectedVersion || receipt.PolicyVersion != cmd.ExpectedPolicyVersion || receipt.Reason != cmd.Reason {
			return report, fmt.Errorf("recovery request conflicts with historical receipt")
		}
		report.State = "historical_completed"
	}
	report.CurrentOperator, err = t.current(ctx, cmd)
	return report, err
}
func (t *Tool) Preflight(ctx context.Context, cmd app.RecoveryCommand) (*Report, error) {
	report, err := t.Status(ctx, cmd)
	if err != nil || report.State != "pending" {
		return report, err
	}
	ctx, err = t.administrator(ctx, cmd)
	if err != nil {
		return report, err
	}
	plan, err := t.service.PreviewIdentityRecovery(ctx, cmd)
	if err != nil {
		return report, err
	}
	// A plan must agree with the local retired state, not just the old exit task.
	var matches int64
	if err = t.db.WithContext(ctx).Table("operators").Where("id=? AND org_id=? AND user_id=? AND version=? AND is_active=FALSE AND deleted_at IS NOT NULL", plan.OperatorID, plan.OrgID, plan.UserID, plan.ExpectedVersion).Count(&matches).Error; err != nil {
		return report, err
	}
	if matches != 1 {
		return report, fmt.Errorf("retired identity changed; refresh preflight")
	}
	var exit map[string]interface{}
	if err = t.db.WithContext(ctx).Table("operator_retirement_tasks").Where("operator_id=? AND org_id=?", plan.OperatorID, plan.OrgID).Take(&exit).Error; err != nil {
		return report, err
	}
	// Ignore the preview creation time and maintenance acknowledgment in the deterministic input digest.
	input := cmd
	input.WritesStopped = false
	raw, err := json.Marshal(struct {
		Command        app.RecoveryCommand
		Operator, Exit map[string]interface{}
	}{input, report.CurrentOperator, exit})
	if err != nil {
		return report, err
	}
	sum := sha256.Sum256(raw)
	report.Fingerprint = hex.EncodeToString(sum[:])
	report.Recovery = plan
	return report, nil
}
func (t *Tool) Apply(ctx context.Context, cmd app.RecoveryCommand, fingerprint string) (*Report, error) {
	if !cmd.WritesStopped || fingerprint == "" {
		return nil, fmt.Errorf("paused writes and reviewed fingerprint required")
	}
	var result *Report
	err := t.repo.WithOperatorLock(ctx, cmd.OrgID, cmd.OperatorID, func(locked context.Context) error {
		report, err := t.Preflight(locked, cmd)
		result = report
		if err != nil {
			return err
		}
		if report.State == "historical_completed" {
			return nil
		}
		if report.Fingerprint != fingerprint {
			return fmt.Errorf("recovery facts changed; repeat preflight")
		}
		authorized, err := t.administrator(locked, cmd)
		if err != nil {
			return err
		}
		value, err := t.service.RecoverIdentity(authorized, cmd)
		if err != nil {
			return err
		}
		result.Recovery = value
		result.State = "historical_completed"
		result.CurrentOperator, err = t.current(locked, cmd)
		return err
	})
	return result, err
}
