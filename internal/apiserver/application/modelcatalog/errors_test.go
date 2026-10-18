package modelcatalog

import (
	"encoding/json"
	"testing"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestMapDraftWriteErrorMapsRevisionConflictTo409(t *testing.T) {
	t.Parallel()
	err := MapDraftWriteError(domain.ErrRevisionConflict)
	if got := baseerrors.ParseCoder(err).Code(); got != code.ErrConflict {
		t.Fatalf("code = %d, want %d", got, code.ErrConflict)
	}
}

func TestValidationFailedErrorMapsDomainIssues(t *testing.T) {
	t.Parallel()
	err := NewValidationFailedError([]domain.DomainValidationIssue{{
		Field:   "definition_v2.measure",
		Code:    "measure.required",
		Message: "measure is required",
		Level:   domain.ValidationLevelError,
	}})
	validation, ok := ValidationFailedFrom(err)
	if !ok {
		t.Fatalf("error = %T, want *ValidationFailedError", err)
	}
	if validation.Result.Passed {
		t.Fatalf("result = %#v, want failed validation", validation.Result)
	}
	if got := validation.Result.Issues[0]; got.Field != "definition_v2.measure" || got.Code != "measure.required" || got.Level != "error" {
		t.Fatalf("issue = %#v", got)
	}
}

func TestValidationResponseCanonicalContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		issues []ValidationIssue
		passed bool
	}{
		{name: "no issues", passed: true},
		{name: "warning", issues: []ValidationIssue{{Field: "measure", Message: "review", Level: "warning"}}, passed: true},
		{name: "error", issues: []ValidationIssue{{Field: "measure", Message: "required", Code: "required", Level: "error"}}},
		{name: "unspecified severity", issues: []ValidationIssue{{Field: "measure", Message: "required"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := NewValidationResult(tc.issues)
			if result.Passed != tc.passed || len(result.Issues) != len(tc.issues) {
				t.Fatalf("validation result = %#v", result)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if len(response) != 2 || response["passed"] == nil || response["issues"] == nil {
				t.Fatalf("validation contract = %s; want passed/issues", raw)
			}
		})
	}
}
