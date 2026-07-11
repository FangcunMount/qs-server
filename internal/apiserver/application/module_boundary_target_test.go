//go:build refactor_target

package application_test

import (
	"strings"
	"testing"
)

func TestTargetEvaluationInterpretationHaveNoCrossModuleImplementationImports(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cases := []struct {
		name              string
		scanRoots         []string
		forbiddenPrefixes []string
	}{
		{
			name:      "evaluation_to_interpretation",
			scanRoots: []string{"internal/apiserver/application/evaluation", "internal/apiserver/container/modules/evaluation"},
			forbiddenPrefixes: []string{
				"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation",
				"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation",
				"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation",
			},
		},
		{
			name:      "interpretation_to_evaluation",
			scanRoots: []string{"internal/apiserver/application/interpretation", "internal/apiserver/container/modules/interpretation"},
			forbiddenPrefixes: []string{
				"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation",
				"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation",
				"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collectCrossModuleImporters(t, root, tc.scanRoots, tc.forbiddenPrefixes)
			if len(got) != 0 {
				t.Fatalf("target boundary still has implementation imports:\n%s", strings.Join(got, "\n"))
			}
		})
	}
}
