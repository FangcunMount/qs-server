package iam

import (
	"context"
	"fmt"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"strings"
)

type scopedRetirementGateway struct {
	gateway iambridge.OperatorScopeGateway
}

// NewScopedOperatorRetirementAuthzGateway never calls the legacy per-user replacement API.
func NewScopedOperatorRetirementAuthzGateway(gateway iambridge.OperatorAuthzGateway) iambridge.OperatorAuthzGateway {
	scoped, ok := gateway.(iambridge.OperatorScopeGateway)
	if !ok || gateway == nil || !gateway.IsEnabled() {
		return nil
	}
	return &scopedRetirementGateway{gateway: scoped}
}
func (g *scopedRetirementGateway) IsEnabled() bool { return g != nil && g.gateway != nil }
func (g *scopedRetirementGateway) facts(ctx context.Context, org, user int64) (iambridge.OperatorAssignmentFacts, error) {
	if !g.IsEnabled() || org <= 0 || user <= 0 {
		return iambridge.OperatorAssignmentFacts{}, fmt.Errorf("valid retirement company and user required")
	}
	facts, err := g.gateway.LoadOperatorAssignmentFacts(ctx, user)
	if err != nil {
		return facts, err
	}
	if facts.PolicyVersion <= 0 {
		return facts, errors.WithCode(code.ErrConflict, "invalid authorization version")
	}
	for _, fact := range facts.Assignments {
		if fact.ManagementProtection != "standard" {
			return facts, errors.WithCode(code.ErrConflict, "protected authorization requires review")
		}
		if fact.RoleName == "user" {
			continue
		}
		switch fact.RoleName {
		case "qs:assessment_operator", "qs:result_reviewer", "qs:content_manager", "qs:evaluation_plan_manager":
		default:
			return facts, errors.WithCode(code.ErrConflict, "unexpected authorization requires review")
		}
		if strings.HasPrefix(fact.RoleName, "qs:") && (fact.Scope == nil || fact.Scope.OrgID != org) {
			return facts, errors.WithCode(code.ErrConflict, "unconfigured or other-company authorization requires review")
		}
	}
	return facts, nil
}
func (g *scopedRetirementGateway) LoadOperatorRoleProjection(ctx context.Context, org, user int64) (iambridge.OperatorRoleProjection, error) {
	facts, err := g.facts(ctx, org, user)
	if err != nil {
		return iambridge.OperatorRoleProjection{}, err
	}
	return scopedOperatorProjection(org, facts)
}
func (g *scopedRetirementGateway) ReplaceManagedOperatorRoles(ctx context.Context, org, user int64, roles []string, by, reason string) (int64, error) {
	if len(roles) != 0 {
		return 0, errors.WithCode(code.ErrInvalidArgument, "retirement can only revoke roles")
	}
	facts, err := g.facts(ctx, org, user)
	if err != nil {
		return 0, err
	}
	return g.gateway.ReplaceOperatorScopedRoles(ctx, org, user, []iambridge.OperatorScopedRole{}, facts.PolicyVersion, by, reason)
}
