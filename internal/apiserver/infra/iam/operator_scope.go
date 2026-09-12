package iam

import (
	"context"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	sdkauthz "github.com/FangcunMount/iam/v5/pkg/sdk/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"sort"
	"strconv"
	"strings"
)

var _ iambridge.OperatorScopeGateway = (*operatorAuthzGateway)(nil)

func (g *operatorAuthzGateway) LoadOperatorAssignmentFacts(ctx context.Context, userID int64) (iambridge.OperatorAssignmentFacts, error) {
	if !g.IsEnabled() || userID <= 0 {
		return iambridge.OperatorAssignmentFacts{}, fmt.Errorf("valid IAM user and gateway required")
	}
	resp, err := g.snapshot.LoadScopedAssignmentFacts(ctx, strconv.FormatInt(userID, 10))
	if err != nil {
		return iambridge.OperatorAssignmentFacts{}, err
	}
	return operatorAssignmentFacts(resp)
}
func operatorAssignmentFacts(resp *pb.GetAuthorizationSnapshotResponse) (iambridge.OperatorAssignmentFacts, error) {
	if err := sdkauthz.ValidateAssignmentScopes(resp); err != nil {
		return iambridge.OperatorAssignmentFacts{}, err
	}
	result := iambridge.OperatorAssignmentFacts{PolicyVersion: resp.PolicyVersion, Assignments: make([]iambridge.OperatorAssignmentFact, 0, len(resp.AssignmentScopes))}
	for _, fact := range resp.AssignmentScopes {
		item := iambridge.OperatorAssignmentFact{AssignmentID: fact.AssignmentId, RoleID: fact.Role.RoleId, RoleName: fact.Role.RoleName, ManagementProtection: fact.Role.ManagementProtection}
		if fact.Scope != nil {
			org, _ := strconv.ParseInt(fact.Scope.OrgId, 10, 64)
			kind := "stores"
			if fact.Scope.Kind == pb.DataScopeKind_ALL_STORES {
				kind = "all_stores"
			}
			value := &iambridge.OperatorAssignmentScope{OrgID: org, Kind: kind, StoreIDs: make([]uint64, 0, len(fact.Scope.StoreIds))}
			for _, raw := range fact.Scope.StoreIds {
				id, _ := strconv.ParseUint(raw, 10, 64)
				value.StoreIDs = append(value.StoreIDs, id)
			}
			item.Scope = value
		}
		result.Assignments = append(result.Assignments, item)
	}
	return result, nil
}

func (g *operatorAuthzGateway) ReplaceOperatorScopedRoles(ctx context.Context, orgID, userID int64, roles []iambridge.OperatorScopedRole, expectedVersion int64, changedBy, reason string) (int64, error) {
	if !g.IsEnabled() || userID <= 0 {
		return 0, fmt.Errorf("valid IAM user and gateway required")
	}
	entries, err := operatorScopedRoleRequests(orgID, roles)
	if err != nil {
		return 0, err
	}
	result, err := g.assignment.ReplaceScoped(ctx, strconv.FormatInt(userID, 10), strconv.FormatInt(orgID, 10), entries, expectedVersion, changedBy, reason)
	if err != nil {
		if status.Code(err) == codes.Aborted {
			return 0, cberrors.WithCode(code.ErrConflict, "authorization changed; refresh before editing")
		}
		return 0, err
	}
	g.snapshot.ObserveAuthzVersion(result.PolicyVersion)
	return result.PolicyVersion, nil
}
func operatorScopedRoleRequests(orgID int64, roles []iambridge.OperatorScopedRole) ([]*pb.ScopedRoleAssignment, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("valid company required")
	}
	result := make([]*pb.ScopedRoleAssignment, 0, len(roles))
	seen := map[string]bool{}
	for _, role := range roles {
		if role.RoleName == "" || role.RoleName != strings.TrimSpace(role.RoleName) || seen[role.RoleName] || role.Scope.OrgID != orgID {
			return nil, fmt.Errorf("invalid role or mismatched assignment company")
		}
		seen[role.RoleName] = true
		value := &pb.DataScope{OrgId: strconv.FormatInt(orgID, 10)}
		switch role.Scope.Kind {
		case "all_stores":
			if len(role.Scope.StoreIDs) != 0 {
				return nil, fmt.Errorf("all stores cannot contain selected stores")
			}
			value.Kind = pb.DataScopeKind_ALL_STORES
		case "stores":
			if len(role.Scope.StoreIDs) == 0 {
				return nil, fmt.Errorf("selected stores required")
			}
			value.Kind = pb.DataScopeKind_STORES
			for _, id := range role.Scope.StoreIDs {
				if id == 0 || id > math.MaxInt64 {
					return nil, fmt.Errorf("invalid store ID")
				}
				value.StoreIds = append(value.StoreIds, strconv.FormatUint(id, 10))
			}
		default:
			return nil, fmt.Errorf("explicit scope kind required")
		}
		result = append(result, &pb.ScopedRoleAssignment{RoleName: role.RoleName, Scope: value})
	}
	return result, nil
}

// The normal projection is company-specific. Retirement's full facts path is
// separate and must continue to inspect all applications/companies.
func scopedOperatorProjection(orgID int64, facts iambridge.OperatorAssignmentFacts) (iambridge.OperatorRoleProjection, error) {
	if orgID <= 0 || facts.PolicyVersion <= 0 {
		return iambridge.OperatorRoleProjection{}, fmt.Errorf("valid company and policy version required")
	}
	result := iambridge.OperatorRoleProjection{PolicyVersion: facts.PolicyVersion, DirectRoles: []string{}, EffectiveRoles: []string{}}
	seen := map[string]bool{}
	for _, fact := range facts.Assignments {
		if fact.ManagementProtection != "standard" {
			result.ProtectedAccess = true
		}
		if fact.Scope == nil || fact.Scope.OrgID != orgID || !strings.HasPrefix(fact.RoleName, "qs:") || seen[fact.RoleName] {
			continue
		}
		seen[fact.RoleName] = true
		result.DirectRoles = append(result.DirectRoles, fact.RoleName)
	}
	sort.Strings(result.DirectRoles)
	result.EffectiveRoles = append(result.EffectiveRoles, result.DirectRoles...)
	return result, nil
}
