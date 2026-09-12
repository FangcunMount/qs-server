// Package operatorprepare stages inactive, unprivileged Operators for reviewed IAM users.
package operatorprepare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	actorrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"gorm.io/gorm"
)

type SnapshotReader interface {
	LoadFresh(context.Context, string) (*authz.Snapshot, error)
}

// IdentityCheck must read IAM and reject a missing, mismatched or inactive user.
type IdentityCheck func(context.Context, int64) error

type Command struct {
	OrgID, UserID, ActorID  int64
	Name, RequestID, Reason string
}

type Report struct {
	State       string  `json:"state"`
	Command     Command `json:"command"`
	Fingerprint string  `json:"fingerprint"`
	OperatorID  uint64  `json:"operator_id,string,omitempty"`
	LastError   string  `json:"last_error,omitempty"`
}

type Tool struct {
	db       *gorm.DB
	loader   SnapshotReader
	identity IdentityCheck
	gateway  iambridge.OperatorAuthzGateway
	gate     *retirement.Repository
	service  app.OperatorLifecycleService
}

func New(db *gorm.DB, loader SnapshotReader, gateway iambridge.OperatorAuthzGateway, identity IdentityCheck) *Tool {
	gate := retirement.NewRepository(db)
	repo := actorrepo.NewOperatorRepository(db)
	validator := domain.NewValidator()
	uow := dbctx.NewUnitOfWork(db)
	runner := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return uow.WithinTransaction(ctx, fn) })
	service := app.NewLifecycleService(repo, domain.NewFactory(repo, validator), validator, domain.NewEditor(validator), domain.NewLifecycler(), runner, nil, nil, gateway, gate)
	return &Tool{db: db, loader: loader, gateway: gateway, identity: identity, gate: gate, service: service}
}

func (t *Tool) Preflight(ctx context.Context, cmd Command) (*Report, error) {
	if cmd.OrgID <= 0 || cmd.UserID <= 0 || cmd.ActorID <= 0 || cmd.UserID == cmd.ActorID || strings.TrimSpace(cmd.Name) != cmd.Name || cmd.Name == "" || cmd.RequestID == "" || len(cmd.RequestID) > 64 || strings.TrimSpace(cmd.RequestID) != cmd.RequestID || strings.TrimSpace(cmd.Reason) != cmd.Reason || cmd.Reason == "" || len([]rune(cmd.Reason)) > 500 {
		return nil, fmt.Errorf("exact company, target, actor, name, request and reason required")
	}
	if t.loader == nil || t.identity == nil || t.gateway == nil || !t.gateway.IsEnabled() {
		return nil, fmt.Errorf("authoritative IAM dependencies required")
	}
	snapshot, err := t.loader.LoadFresh(ctx, strconv.FormatInt(cmd.ActorID, 10))
	if err != nil {
		return nil, err
	}
	if snapshot == nil || !snapshot.IsQSAdmin() {
		return nil, fmt.Errorf("headquarters permission required")
	}
	var actor actorrepo.OperatorPO
	if err = t.db.WithContext(ctx).Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", cmd.OrgID, cmd.ActorID).Take(&actor).Error; err != nil {
		return nil, fmt.Errorf("active headquarters identity required")
	}
	var retiring int64
	if err = t.db.WithContext(ctx).Table("operator_retirement_tasks").Where("user_id IN ?", []int64{cmd.ActorID, cmd.UserID}).Count(&retiring).Error; err != nil {
		return nil, err
	}
	if retiring != 0 {
		return nil, fmt.Errorf("actor or target has a retirement task")
	}
	if err = t.identity(ctx, cmd.UserID); err != nil {
		return nil, err
	}
	projection, err := t.gateway.LoadOperatorRoleProjection(ctx, cmd.OrgID, cmd.UserID)
	if err != nil {
		return nil, err
	}
	if projection.ProtectedAccess || len(projection.DirectRoles) != 0 || len(projection.EffectiveRoles) != 0 {
		return nil, fmt.Errorf("target already has authorization; preparation requires no roles")
	}
	var existing []actorrepo.OperatorPO
	// Include soft-deleted and other-company rows: preparation cannot recover or move identities.
	if err = t.db.WithContext(ctx).Where("user_id=?", cmd.UserID).Find(&existing).Error; err != nil {
		return nil, err
	}
	report := &Report{State: "pending", Command: cmd}
	if len(existing) > 0 {
		if len(existing) != 1 {
			return nil, fmt.Errorf("multiple Operator memberships require review")
		}
		row := existing[0]
		if row.OrgID != cmd.OrgID || row.DeletedAt != nil || row.IsActive || row.Name != cmd.Name || int64(row.CreatedBy) != cmd.ActorID || len(row.Roles) != 0 || len(row.EffectiveRoles) != 0 {
			return nil, fmt.Errorf("existing Operator conflicts with inactive preparation")
		}
		// This observes current state, not ownership of an earlier request or historical completion.
		report.State = "existing_inactive"
		report.OperatorID = uint64(row.ID)
	}
	raw, err := json.Marshal(struct {
		Command    Command
		Actor      actorrepo.OperatorPO
		Existing   []actorrepo.OperatorPO
		Projection iambridge.OperatorRoleProjection
	}{cmd, actor, existing, projection})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	report.Fingerprint = hex.EncodeToString(sum[:])
	return report, nil
}

func (t *Tool) Apply(ctx context.Context, cmd Command, fingerprint string) (*Report, error) {
	if fingerprint == "" {
		return nil, fmt.Errorf("reviewed fingerprint required")
	}
	var report *Report
	err := t.gate.WithinMutation(ctx, cmd.UserID, func(locked context.Context) error {
		var err error
		report, err = t.Preflight(locked, cmd)
		if err != nil {
			return err
		}
		// Never call Register on an existing identity: it is also an update API.
		if report.State == "existing_inactive" {
			return nil
		}
		if report.Fingerprint != fingerprint {
			return fmt.Errorf("preparation facts changed; repeat preflight")
		}
		audited := context.WithValue(locked, middleware.UserClaimsContextKey{}, &middleware.UserClaims{UserID: strconv.FormatInt(cmd.ActorID, 10), OrgID: strconv.FormatInt(cmd.OrgID, 10)})
		result, err := t.service.Register(audited, app.RegisterOperatorDTO{OrgID: cmd.OrgID, UserID: cmd.UserID, Name: cmd.Name, IsActive: false})
		if err != nil {
			return err
		}
		report.OperatorID = result.ID
		report.State = "created_inactive"
		return nil
	})
	return report, err
}
