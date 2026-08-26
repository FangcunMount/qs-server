package authzmatrix

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

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
		result = append(result, Subject{Kind: query.kind, ExpectedRole: query.role, UserID: userID})
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
			return "", fmt.Errorf("no active production subject found for %s role %s", query.kind, query.role)
		}
		return "", fmt.Errorf("select production subject for %s: %w", query.kind, err)
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(userID), 10, 64)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("invalid production user ID selected for %s", query.kind)
	}
	return strconv.FormatUint(parsed, 10), nil
}

func jsonString(value string) string {
	return strconv.Quote(value)
}
