package operatorretire

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type OrphanCommand struct {
	OrgID, ActorID, UserID int64
	RequestID, Reason      string
}

type OrphanReport struct {
	State                  string                     `json:"state"`
	Command                OrphanCommand              `json:"command"`
	Fingerprint            string                     `json:"fingerprint"`
	Roles                  []authz.AssignmentRoleFact `json:"roles"`
	PolicyVersion          int64                      `json:"policy_version"`
	SubmittedPolicyVersion int64                      `json:"submitted_policy_version,omitempty"`
	LastError              string                     `json:"last_error,omitempty"`
}

// OrphanPreflight is deliberately stricter than normal retirement: no Operator
// row may exist in any company, including soft-deleted history.
func (t *Tool) OrphanPreflight(ctx context.Context, cmd OrphanCommand) (*OrphanReport, error) {
	if cmd.OrgID <= 0 || cmd.ActorID <= 0 || cmd.UserID <= 0 || cmd.ActorID == cmd.UserID || cmd.RequestID == "" || len(cmd.RequestID) > 64 || strings.TrimSpace(cmd.RequestID) != cmd.RequestID || cmd.Reason == "" || strings.TrimSpace(cmd.Reason) != cmd.Reason || len([]rune(cmd.Reason)) > 400 {
		return nil, fmt.Errorf("exact orphan identity, administrator, request and reason required")
	}
	ctx, err := t.selectedActor(ctx, app.Command{OrgID: cmd.OrgID, ActorID: cmd.ActorID})
	if err != nil {
		return nil, err
	}
	var count int64
	if err = t.db.WithContext(ctx).Table("operators").Where("user_id=?", cmd.UserID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, fmt.Errorf("operator membership or history exists; use audited Operator exit")
	}
	if err = t.db.WithContext(ctx).Table("operator_retirement_tasks").Where("user_id=?", cmd.UserID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, fmt.Errorf("existing retirement task requires review")
	}
	snapshot, err := t.loader.LoadAssignmentFacts(ctx, strconv.FormatInt(cmd.UserID, 10))
	if err != nil {
		return nil, err
	}
	if snapshot == nil || !snapshot.AssignmentFactsComplete || snapshot.AuthzVersion <= 0 {
		return nil, fmt.Errorf("complete current assignment facts required")
	}
	projection, err := t.gateway.LoadOperatorRoleProjection(ctx, cmd.OrgID, cmd.UserID)
	if err != nil {
		return nil, err
	}
	if projection.ProtectedAccess || projection.PolicyVersion != snapshot.AuthzVersion {
		return nil, fmt.Errorf("protected access or concurrent authorization change")
	}
	r := &OrphanReport{State: "pending", Command: cmd, Roles: append([]authz.AssignmentRoleFact{}, snapshot.AssignmentFacts...), PolicyVersion: snapshot.AuthzVersion}
	sort.Slice(r.Roles, func(i, j int) bool { return r.Roles[i].RoleID < r.Roles[j].RoleID })
	backend := 0
	for _, role := range r.Roles {
		if role.ManagementProtection != "standard" {
			return r, fmt.Errorf("protected or unknown role class")
		}
		switch role.RoleName {
		case "user":
		case "qs:assessment_operator", "qs:result_reviewer", "qs:evaluation_plan_manager":
			backend++
		default:
			return r, fmt.Errorf("unreviewed backend role requires explicit review")
		}
	}
	if backend == 0 {
		r.State = "already_unprivileged"
	}
	// Include global policy version: a later regrant cannot reuse an old plan even
	// when the role names match. Any concurrent authorization write requires preview.
	r.Fingerprint = hash(struct {
		Command OrphanCommand
		Roles   []authz.AssignmentRoleFact
		Version int64
	}{cmd, r.Roles, r.PolicyVersion})
	return r, nil
}

func (t *Tool) OrphanApply(ctx context.Context, cmd OrphanCommand, fingerprint string, paused bool) (*OrphanReport, error) {
	if !paused || fingerprint == "" {
		return nil, fmt.Errorf("actual paused writes and reviewed fingerprint required")
	}
	var result *OrphanReport
	err := t.repository.WithinMutation(ctx, cmd.UserID, func(locked context.Context) error {
		var err error
		result, err = t.OrphanPreflight(locked, cmd)
		if err != nil {
			return err
		}
		if result.State == "already_unprivileged" {
			return nil
		}
		if result.Fingerprint != fingerprint {
			return fmt.Errorf("orphan facts changed; repeat preflight")
		}
		version, err := t.gateway.ReplaceManagedOperatorRoles(locked, cmd.OrgID, cmd.UserID, []string{}, authz.SubjectKey(strconv.FormatInt(cmd.ActorID, 10)), cmd.RequestID+": "+cmd.Reason)
		if err != nil {
			return err
		}
		result.SubmittedPolicyVersion = version
		result.State = "revocation_submitted"
		verified, err := t.OrphanPreflight(locked, cmd)
		if err != nil {
			return err
		}
		if verified.State != "already_unprivileged" || verified.PolicyVersion < version {
			return fmt.Errorf("revocation not yet confirmed; inspect current facts")
		}
		result.State = "verified"
		return nil
	})
	return result, err
}
