package operator

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	storeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
)

// ScopeService manages backend assignment ranges; medical identities are not
// involved. Identity/company come from authenticated Operator context.
type ScopeStoreReader interface {
	Get(context.Context, int64, uint64) (*storeDomain.Store, error)
}
type ScopeWriteDependencies struct {
	Stores ScopeStoreReader
	Gate   retirement.MutationGate
}

type ScopeService struct {
	writes  ScopeWriteDependencies
	repo    domain.Repository
	gateway iambridge.OperatorScopeGateway
}

func NewScopeService(repo domain.Repository, gateway iambridge.OperatorScopeGateway, writes ...ScopeWriteDependencies) *ScopeService {
	service := &ScopeService{repo: repo, gateway: gateway}
	if len(writes) > 0 {
		service.writes = writes[0]
	}
	return service
}

type ScopeConfiguration struct {
	OperatorID      uint64
	PolicyVersion   int64
	Assignments     []iambridge.OperatorAssignmentFact
	Unconfigured    []iambridge.OperatorAssignmentFact
	ProtectedAccess bool
}

func (s *ScopeService) target(ctx context.Context, id uint64) (*domain.Operator, error) {
	org, user := actorctx.OperatorOrgID(ctx), actorctx.GrantingUserID(ctx)
	snap, ok := authz.FromContext(ctx)
	if s == nil || s.repo == nil || s.gateway == nil || org <= 0 || user == 0 || user > math.MaxInt64 || !ok || snap == nil || !snap.IsQSAdmin() {
		return nil, errors.WithCode(code.ErrPermissionDenied, "company headquarters administrator required")
	}
	rangeForCompany, scopeErr := snap.ResolveStoreRange(org, "qs:actor:collection:operators", "read")
	if scopeErr != nil || !rangeForCompany.AllStores {
		return nil, errors.WithCode(code.ErrPermissionDenied, "headquarters company scope required")
	}
	actor, err := s.repo.FindByUser(ctx, org, int64(user))
	if err != nil {
		return nil, err
	}
	if actor == nil || !actor.IsActive() || actor.OrgID() != org || actor.UserID() != int64(user) {
		return nil, errors.WithCode(code.ErrPermissionDenied, "active company operator required")
	}
	resolved, err := operatorIDFromUint64("operator_id", id)
	if err != nil {
		return nil, err
	}
	target, err := s.repo.FindByID(ctx, resolved)
	if err != nil {
		return nil, err
	}
	if target == nil || target.OrgID() != org {
		return nil, errors.WithCode(code.ErrUserNotFound, "operator not found in current company")
	}
	return target, nil
}
func (s *ScopeService) Get(ctx context.Context, id uint64) (ScopeConfiguration, error) {
	target, err := s.target(ctx, id)
	if err != nil {
		return ScopeConfiguration{}, err
	}
	facts, err := s.gateway.LoadOperatorAssignmentFacts(ctx, target.UserID())
	if err != nil {
		return ScopeConfiguration{}, err
	}
	result := ScopeConfiguration{OperatorID: id, PolicyVersion: facts.PolicyVersion, Assignments: []iambridge.OperatorAssignmentFact{}, Unconfigured: []iambridge.OperatorAssignmentFact{}}
	for _, fact := range facts.Assignments {
		// Protection checks retain all-company facts even though the editable list
		// contains only the resolved company. Legacy facts have no inferred company.
		if fact.ManagementProtection != "standard" {
			result.ProtectedAccess = true
		}
		if fact.Scope == nil {
			result.Unconfigured = append(result.Unconfigured, fact)
			continue
		}
		if fact.Scope.OrgID == target.OrgID() {
			result.Assignments = append(result.Assignments, fact)
		}
	}
	return result, nil
}
