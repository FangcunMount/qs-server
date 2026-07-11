package application_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Evaluation and Interpretation may depend on explicit composition ports, but
// neither module may import the other module's implementation packages.
func TestEvaluationInterpretationCrossModuleImportDebtDoesNotSpread(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cases := []struct {
		name              string
		scanRoots         []string
		forbiddenPrefixes []string
		allowedImporters  []string
	}{
		{
			name: "evaluation_to_interpretation",
			scanRoots: []string{
				"internal/apiserver/domain/evaluation",
				"internal/apiserver/application/evaluation",
				"internal/apiserver/container/modules/evaluation",
				"internal/apiserver/infra/mysql/evaluation",
			},
			forbiddenPrefixes: []string{
				"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation",
				"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation",
				"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation",
			},
			allowedImporters: []string{},
		},
		{
			name: "interpretation_to_evaluation",
			scanRoots: []string{
				"internal/apiserver/application/interpretation",
				"internal/apiserver/container/modules/interpretation",
			},
			forbiddenPrefixes: []string{
				"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation",
				"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation",
				"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation",
			},
			allowedImporters: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collectCrossModuleImporters(t, root, tc.scanRoots, tc.forbiddenPrefixes)
			want := append([]string(nil), tc.allowedImporters...)
			sort.Strings(want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("cross-module import debt changed\n got:\n%s\nwant:\n%s\nshrink the allowlist when removing debt; do not add new importers", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// These allowlists freeze the two application-role debts that later batches
// deliberately remove. New code must not add more report-viewing behavior to
// Evaluation or more operator-only batch behavior to the Worker service.
func TestEvaluationRoleCompatibilityDebtDoesNotSpread(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cases := []struct {
		name      string
		scanRoots []string
		tokens    []string
		want      []string
	}{
		{
			name: "report_viewing_in_evaluation",
			scanRoots: []string{
				"internal/apiserver/application/evaluation",
				"internal/apiserver/container/modules/evaluation",
				"internal/apiserver/infra/mysql/evaluation",
			},
			tokens: []string{"ReportReader", "ReportQueryService", "WaitReport", "ReportResult", "ListReportsDTO"},
			want:   []string{},
		},
		{
			name:      "operator_batch_execution_in_worker_service",
			scanRoots: []string{"internal/apiserver/application/evaluation/execute"},
			tokens:    []string{"EvaluateBatch"},
			want:      []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collectSourceTokenFiles(t, root, tc.scanRoots, tc.tokens)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("role compatibility debt changed\n got:\n%s\nwant:\n%s\nmove existing ownership deliberately and shrink this allowlist; do not add new callers", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// C-facing queries and answer-sheet orchestration must use narrow actor ports.
func TestEvaluationTransportDoesNotUseCombinedSubmissionPort(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	got := collectSourceTokenFiles(t, root, []string{"internal/apiserver/transport/grpc/service"}, []string{"AssessmentSubmissionService"})
	if len(got) != 0 {
		t.Fatalf("gRPC transport must use actor-specific Assessment ports, found combined port in:\n%s", strings.Join(got, "\n"))
	}
}

func TestAssessmentApplicationDoesNotRetainCombinedFacades(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	got := collectSourceTokenFiles(t, root, []string{
		"internal/apiserver/application/evaluation/assessment/interface.go",
		"internal/apiserver/application/evaluation/assessment/submission_service.go",
		"internal/apiserver/application/evaluation/assessment/management_service.go",
	}, []string{"AssessmentSubmissionService", "AssessmentManagementService"})
	if len(got) != 0 {
		t.Fatalf("Assessment application must expose role-specific ports only, found combined facade in:\n%s", strings.Join(got, "\n"))
	}
}

// Backend REST transport must receive only the operator recovery use case;
// all reads are performed by the protected operator-query orchestration.
func TestEvaluationRESTTransportDoesNotUseCombinedManagementPort(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	got := collectSourceTokenFiles(t, root, []string{"internal/apiserver/transport/rest"}, []string{"AssessmentManagementService"})
	if len(got) != 0 {
		t.Fatalf("REST transport must use operator-specific Assessment ports, found combined port in:\n%s", strings.Join(got, "\n"))
	}
}

// Interpretation owns construction of the report-query use case. Evaluation
// must neither construct nor consume that capability.
func TestEvaluationDoesNotConstructReportQueryService(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	got := collectSourceTokenFiles(t, root, []string{
		"internal/apiserver/application/evaluation",
		"internal/apiserver/container/modules/evaluation",
	}, []string{"NewReportQueryService("})
	if len(got) != 0 {
		t.Fatalf("Evaluation must consume the Interpretation-owned report query port, found construction in:\n%s", strings.Join(got, "\n"))
	}
}

func TestProtectedAssessmentQueryDoesNotOwnReportWait(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	got := collectSourceTokenFiles(t, root, []string{"internal/apiserver/application/evaluation/assessment/protected_query_service.go"}, []string{"WaitReport"})
	if len(got) != 0 {
		t.Fatalf("protected Assessment query must not own report waiting, found in:\n%s", strings.Join(got, "\n"))
	}
}

func collectCrossModuleImporters(t *testing.T, root string, scanRoots, forbiddenPrefixes []string) []string {
	t.Helper()
	found := make(map[string]struct{})
	for _, relRoot := range scanRoots {
		scanGoImports(t, filepath.Join(root, filepath.FromSlash(relRoot)), func(path, importPath string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			for _, prefix := range forbiddenPrefixes {
				if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
					found[filepath.ToSlash(mustRel(t, root, path))] = struct{}{}
					return
				}
			}
		})
	}
	result := make([]string, 0, len(found))
	for importer := range found {
		result = append(result, importer)
	}
	sort.Strings(result)
	return result
}

func collectSourceTokenFiles(t *testing.T, root string, scanRoots, tokens []string) []string {
	t.Helper()
	found := make(map[string]struct{})
	for _, relRoot := range scanRoots {
		absRoot := filepath.Join(root, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, token := range tokens {
				if strings.Contains(string(data), token) {
					found[filepath.ToSlash(mustRel(t, root, path))] = struct{}{}
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	result := make([]string, 0, len(found))
	for rel := range found {
		result = append(result, rel)
	}
	sort.Strings(result)
	return result
}
