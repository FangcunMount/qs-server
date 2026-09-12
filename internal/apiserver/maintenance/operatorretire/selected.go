package operatorretire

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
)

// SelectedReport is separate from the historical doctor retirement manifest.
// Each reviewed exit uses the ordinary durable exit task and keeps its own before-state.
type SelectedReport struct {
	State           string                     `json:"state"`
	Command         app.Command                `json:"command"`
	Fingerprint     string                     `json:"fingerprint,omitempty"`
	CurrentOperator map[string]interface{}     `json:"current_operator,omitempty"`
	Roles           []authz.AssignmentRoleFact `json:"roles"`
	Task            *domain.Task               `json:"task,omitempty"`
	LastError       string                     `json:"last_error,omitempty"`
}

func (t *Tool) selectedActor(ctx context.Context, cmd app.Command) (context.Context, error) {
	if t.table != "operators" || t.loader == nil || t.gateway == nil {
		return ctx, fmt.Errorf("selected retirement requires final schema and IAM")
	}
	snapshot, err := t.loader.LoadFresh(ctx, strconv.FormatInt(cmd.ActorID, 10))
	if err != nil {
		return ctx, err
	}
	if snapshot == nil || !snapshot.IsQSAdmin() {
		return ctx, fmt.Errorf("headquarters permission required")
	}
	var count int64
	if err = t.db.WithContext(ctx).Table(t.table).Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", cmd.OrgID, cmd.ActorID).Count(&count).Error; err != nil {
		return ctx, err
	}
	if count != 1 {
		return ctx, fmt.Errorf("active headquarters identity required")
	}
	if err = t.db.WithContext(ctx).Table("operator_retirement_tasks").Where("user_id=?", cmd.ActorID).Count(&count).Error; err != nil {
		return ctx, err
	}
	if count != 0 {
		return ctx, fmt.Errorf("headquarters identity retiring")
	}
	return authz.WithSnapshot(ctx, snapshot), nil
}
func (t *Tool) SelectedPreflight(ctx context.Context, cmd app.Command) (*SelectedReport, error) {
	ctx, err := t.selectedActor(ctx, cmd)
	if err != nil {
		return nil, err
	}
	report := &SelectedReport{State: "pending", Command: cmd, Roles: []authz.AssignmentRoleFact{}}
	service := app.NewMaintenanceService(t.repository, t.gateway)
	task, err := service.Preview(ctx, cmd)
	if err != nil {
		return report, err
	}
	report.Task = task
	existing, err := t.repository.FindTask(ctx, cmd.OrgID, cmd.OperatorID)
	if err != nil {
		return report, err
	}
	if existing != nil {
		report.State = string(existing.Stage)
	}
	if err = t.db.WithContext(ctx).Table(t.table).Where("id=? AND org_id=?", cmd.OperatorID, cmd.OrgID).Take(&report.CurrentOperator).Error; err != nil {
		return report, err
	}
	facts, err := t.loader.LoadAssignmentFacts(ctx, strconv.FormatInt(task.UserID, 10))
	if err != nil {
		return report, err
	}
	if facts == nil || !facts.AssignmentFactsComplete {
		return report, fmt.Errorf("complete assignment facts required")
	}
	report.Roles = append(report.Roles, facts.AssignmentFacts...)
	sort.Slice(report.Roles, func(i, j int) bool { return report.Roles[i].RoleID < report.Roles[j].RoleID })
	// Global policy versions change as other explicitly selected users exit; this digest
	// covers this user's reviewed facts. The application revalidates IAM before revocation.
	report.Fingerprint = hash(struct {
		Command  app.Command
		Operator map[string]interface{}
		Roles    []authz.AssignmentRoleFact
		Existing *domain.Task
	}{cmd, report.CurrentOperator, report.Roles, existing})
	return report, nil
}
func (t *Tool) SelectedApply(ctx context.Context, cmd app.Command, fingerprint string, paused bool) (*SelectedReport, error) {
	if !paused || fingerprint == "" {
		return nil, fmt.Errorf("paused writes and reviewed fingerprint required")
	}
	var result *SelectedReport
	err := t.repository.WithOperatorLock(ctx, cmd.OrgID, cmd.OperatorID, func(locked context.Context) error {
		report, err := t.SelectedPreflight(locked, cmd)
		result = report
		if err != nil {
			return err
		}
		if report.State == string(domain.Completed) {
			return nil
		}
		if report.Fingerprint != fingerprint {
			return fmt.Errorf("selected operator facts changed; repeat preflight")
		}
		authorized, err := t.selectedActor(locked, cmd)
		if err != nil {
			return err
		}
		task, err := app.NewMaintenanceService(t.repository, t.gateway).Execute(authorized, cmd)
		result.Task = task
		if task != nil {
			result.State = string(task.Stage)
		}
		return err
	})
	return result, err
}

// SelectedVerify checks present admission and IAM facts, not just historical task completion.
func (t *Tool) SelectedVerify(ctx context.Context, cmd app.Command) (*SelectedReport, error) {
	report, err := t.SelectedPreflight(ctx, cmd)
	if err != nil {
		return report, err
	}
	if report.State != string(domain.Completed) {
		return report, fmt.Errorf("selected retirement is not complete")
	}
	for _, role := range report.Roles {
		if role.RoleName != "user" || role.ManagementProtection != "standard" {
			return report, fmt.Errorf("backend or unexpected authorization remains")
		}
	}
	var count int64
	if err = t.db.WithContext(ctx).Table(t.table).Where("org_id=? AND id=? AND is_active=FALSE AND deleted_at IS NOT NULL", cmd.OrgID, cmd.OperatorID).Count(&count).Error; err != nil {
		return report, err
	}
	if count != 1 {
		return report, fmt.Errorf("operator admission is not closed")
	}
	report.State = "verified"
	return report, nil
}
