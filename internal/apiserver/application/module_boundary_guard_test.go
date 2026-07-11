package application_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This allowlist freezes the current Evaluation/Interpretation boundary debt.
// Refactoring batches must shrink it; new importer files are rejected.
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
			allowedImporters: []string{
				"internal/apiserver/application/interpretation/reporting/audience_profile_builders.go",
				"internal/apiserver/application/interpretation/reporting/factor_scoring_report.go",
				"internal/apiserver/application/interpretation/reporting/materialize/materialize.go",
				"internal/apiserver/application/interpretation/reporting/norm_task_report.go",
				"internal/apiserver/application/interpretation/reporting/projection/event_projection.go",
				"internal/apiserver/application/interpretation/reporting/projection/events.go",
				"internal/apiserver/application/interpretation/reporting/projection/mechanism_events.go",
				"internal/apiserver/application/interpretation/reporting/projection/report_projection.go",
				"internal/apiserver/application/interpretation/reporting/registry/mechanism_key.go",
				"internal/apiserver/application/interpretation/reporting/registry/registry.go",
				"internal/apiserver/application/interpretation/reporting/registry/report_builder.go",
				"internal/apiserver/application/interpretation/reporting/registry/report_strategy.go",
				"internal/apiserver/application/interpretation/reporting/registry/routing_context.go",
				"internal/apiserver/application/interpretation/reporting/typology/legacy_detail.go",
				"internal/apiserver/application/interpretation/reporting/typology/report_builder.go",
				"internal/apiserver/application/interpretation/reporting/typology/report_context.go",
				"internal/apiserver/application/interpretation/reporting/typology/report_generic.go",
				"internal/apiserver/application/interpretation/reporting/typology/report_generic_mechanism.go",
				"internal/apiserver/application/interpretation/reporting/typology/report_input_mapper.go",
				"internal/apiserver/application/interpretation/reporting/typology/report_registry.go",
				"internal/apiserver/application/interpretation/reporting/writer/generator.go",
				"internal/apiserver/application/interpretation/reporting/writer/types.go",
				"internal/apiserver/application/interpretation/service.go",
				"internal/apiserver/container/modules/interpretation/assemble.go",
				"internal/apiserver/container/modules/interpretation/bootstrap.go",
				"internal/apiserver/container/modules/interpretation/wire.go",
			},
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
