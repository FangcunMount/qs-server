package authzmatrix

import (
	"context"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSQLSubjectSourceUsesReadOnlyDeterministicQueries(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	expectSubjectQuery(mock, "admin", []string{RoleAdmin}, "101")
	expectSubjectQuery(mock, "evaluator", []string{RoleEvaluator, RoleAdmin, RolePlanManager}, "102")
	expectSubjectQuery(mock, "plan_manager", []string{RolePlanManager, RoleAdmin, RoleEvaluator}, "103")
	expectSubjectQuery(mock, "other", []string{RoleStaff, RoleAdmin, RoleEvaluator, RolePlanManager}, "104")
	mock.ExpectCommit()

	got, err := NewSQLSubjectSource(db).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 4 || got[0].UserID != "101" || got[3].UserID != "104" {
		t.Fatalf("Load() = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStableSubjectSourceUsesIsolatedIAMForEvaluatorAndPlanManager(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	expectSubjectQuery(mock, "admin", []string{RoleAdmin}, "101")
	expectSubjectQuery(mock, "other", []string{RoleStaff, RoleAdmin, RoleEvaluator, RolePlanManager}, "104")
	mock.ExpectCommit()

	directory := &syntheticDirectoryStub{userIDs: map[string]string{
		SyntheticEvaluatorNickname: "102", SyntheticPlanManagerNickname: "103",
	}}
	got, err := NewStableSubjectSource(db, directory).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 4 || got[1].UserID != "102" || got[1].Source != SubjectSourceSyntheticIAM {
		t.Fatalf("Load() = %+v", got)
	}
	if strings.Join(directory.nicknames, ",") != SyntheticEvaluatorNickname+","+SyntheticPlanManagerNickname {
		t.Fatalf("synthetic nicknames = %v", directory.nicknames)
	}
	for i, subject := range got {
		if (i == 1 || i == 2) != (subject.Source == SubjectSourceSyntheticIAM) {
			t.Fatalf("unexpected fallback source: %+v", subject)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectSubjectQuery(mock sqlmock.Sqlmock, kind string, roles []string, userID string) {
	query, args := subjectSQLExpectation(kind, roles)
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(userID))
}

func expectMissingSubjectQuery(mock sqlmock.Sqlmock, kind string, roles []string) {
	query, args := subjectSQLExpectation(kind, roles)
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
}

func subjectSQLExpectation(kind string, roles []string) (string, []driver.Value) {
	clauses := []string{"is_active = 1", "deleted_at IS NULL", "JSON_VALID(roles)", "JSON_CONTAINS(roles, ?)"}
	for range roles[1:] {
		clauses = append(clauses, "NOT JSON_CONTAINS(roles, ?)")
	}
	if kind == "other" {
		clauses = append(clauses, "JSON_LENGTH(roles) = 1")
	}
	query := "SELECT CAST(user_id AS CHAR) FROM staff WHERE " + strings.Join(clauses, " AND ") + " ORDER BY user_id ASC LIMIT 1"
	args := make([]driver.Value, 0, len(roles))
	for _, role := range roles {
		args = append(args, jsonString(role))
	}
	return query, args
}

type syntheticDirectoryStub struct {
	userIDs   map[string]string
	nicknames []string
}

func (s *syntheticDirectoryStub) FindActiveIsolatedUser(_ context.Context, nickname string) (string, error) {
	s.nicknames = append(s.nicknames, nickname)
	userID, ok := s.userIDs[nickname]
	if !ok {
		return "", ErrSubjectNotFound
	}
	return userID, nil
}
