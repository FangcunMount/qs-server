package authzmatrix

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	SubjectSourceProductionStaff = "production_staff"
	SubjectSourceSyntheticIAM    = "synthetic_iam_user"

	SyntheticEvaluatorNickname   = "__qs_authz_matrix_evaluator_v2__"
	SyntheticPlanManagerNickname = "__qs_authz_matrix_plan_manager_v2__"
)

var ErrSubjectNotFound = errors.New("authz matrix subject not found")

type SQLSubjectSource struct {
	db *sql.DB
}

func NewSQLSubjectSource(db *sql.DB) *SQLSubjectSource {
	return &SQLSubjectSource{db: db}
}

type subjectQuery struct {
	kind     string
	role     string
	excluded []string
}

func (s *SQLSubjectSource) Load(ctx context.Context) ([]Subject, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("qs MySQL connection is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only subject transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	queries := []subjectQuery{
		{kind: "admin", role: RoleAdmin},
		{kind: "evaluator", role: RoleEvaluator, excluded: []string{RoleAdmin, RolePlanManager}},
		{kind: "plan_manager", role: RolePlanManager, excluded: []string{RoleAdmin, RoleEvaluator}},
		{kind: "other", role: RoleStaff, excluded: []string{RoleAdmin, RoleEvaluator, RolePlanManager}},
	}
	result := make([]Subject, 0, len(queries))
	for _, query := range queries {
		userID, err := selectSubject(ctx, tx, query)
		if err != nil {
			return nil, err
		}
		result = append(result, Subject{
			Kind: query.kind, ExpectedRole: query.role, UserID: userID,
			Source: SubjectSourceProductionStaff,
		})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("complete read-only subject transaction: %w", err)
	}
	return result, nil
}

func selectSubject(ctx context.Context, tx *sql.Tx, query subjectQuery) (string, error) {
	clauses := []string{
		"is_active = 1",
		"deleted_at IS NULL",
		"JSON_VALID(roles)",
		"JSON_CONTAINS(roles, ?)",
	}
	args := []any{jsonString(query.role)}
	for _, excluded := range query.excluded {
		clauses = append(clauses, "NOT JSON_CONTAINS(roles, ?)")
		args = append(args, jsonString(excluded))
	}
	if query.kind == "other" {
		clauses = append(clauses, "JSON_LENGTH(roles) = 1")
	}
	statement := "SELECT CAST(user_id AS CHAR) FROM staff WHERE " + strings.Join(clauses, " AND ") + " ORDER BY user_id ASC LIMIT 1"
	var userID string
	if err := tx.QueryRowContext(ctx, statement, args...).Scan(&userID); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("%w: no active production subject found for %s role %s", ErrSubjectNotFound, query.kind, query.role)
		}
		return "", fmt.Errorf("select production subject for %s: %w", query.kind, err)
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(userID), 10, 64)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("invalid production user ID selected for %s", query.kind)
	}
	return strconv.FormatUint(parsed, 10), nil
}

// SyntheticSubjectDirectory resolves deliberately isolated IAM identities. It
// must be read-only while the matrix is running.
type SyntheticSubjectDirectory interface {
	FindActiveIsolatedUsers(context.Context, string) ([]string, error)
}

type StableSubjectSource struct {
	db        *sql.DB
	synthetic SyntheticSubjectDirectory
	snapshots SnapshotReader
}

func NewStableSubjectSource(db *sql.DB, synthetic SyntheticSubjectDirectory, snapshots SnapshotReader) *StableSubjectSource {
	return &StableSubjectSource{db: db, synthetic: synthetic, snapshots: snapshots}
}

func (s *StableSubjectSource) Load(ctx context.Context) ([]Subject, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("qs MySQL connection is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only subject transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	queries := []subjectQuery{
		{kind: "admin", role: RoleAdmin},
		{kind: "other", role: RoleStaff, excluded: []string{RoleAdmin, RoleEvaluator, RolePlanManager}},
	}
	production := make(map[string]Subject, len(queries))
	for _, query := range queries {
		userID, selectErr := selectSubject(ctx, tx, query)
		if selectErr != nil {
			return nil, selectErr
		}
		production[query.kind] = Subject{
			Kind: query.kind, ExpectedRole: query.role, UserID: userID,
			Source: SubjectSourceProductionStaff,
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("complete read-only subject transaction: %w", err)
	}
	if s.synthetic == nil || s.snapshots == nil {
		return nil, fmt.Errorf("synthetic IAM subject directory and snapshot reader are required")
	}
	result := []Subject{production["admin"]}
	for _, subject := range []struct {
		kind, role, nickname string
	}{
		{kind: "evaluator", role: RoleEvaluator, nickname: SyntheticEvaluatorNickname},
		{kind: "plan_manager", role: RolePlanManager, nickname: SyntheticPlanManagerNickname},
	} {
		candidates, syntheticErr := s.synthetic.FindActiveIsolatedUsers(ctx, subject.nickname)
		if syntheticErr != nil {
			return nil, fmt.Errorf("resolve isolated %s subject: %w", subject.kind, syntheticErr)
		}
		userID, syntheticErr := s.selectCandidateByDirectRole(ctx, candidates, subject.role)
		if syntheticErr != nil {
			return nil, fmt.Errorf("resolve isolated %s subject: %w", subject.kind, syntheticErr)
		}
		result = append(result, Subject{
			Kind: subject.kind, ExpectedRole: subject.role, UserID: userID, Source: SubjectSourceSyntheticIAM,
		})
	}
	return append(result, production["other"]), nil
}

func (s *StableSubjectSource) selectCandidateByDirectRole(ctx context.Context, candidates []string, role string) (string, error) {
	matches := make([]string, 0, 1)
	for _, userID := range candidates {
		snapshot, err := s.snapshots.Load(ctx, Domain, userID)
		if err != nil {
			return "", fmt.Errorf("load candidate IAM snapshot: %w", err)
		}
		if equalRoles(snapshot.DirectRoleNames(), []string{role}) {
			matches = append(matches, userID)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected one active isolated subject with direct role %s, found %d among %d nickname matches", role, len(matches), len(candidates))
	}
	return matches[0], nil
}

func jsonString(value string) string {
	return strconv.Quote(value)
}
