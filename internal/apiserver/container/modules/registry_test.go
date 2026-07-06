package modules_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	report "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
)

func TestModuleDescriptorsExposeCanonicalNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  modules.PackageName
		want modules.PackageName
	}{
		{"survey", survey.Describe().Name, modules.PackageSurvey},
		{"modelcatalog", modelcatalog.Describe().Name, modules.PackageModelCatalog},
		{"evaluation", evaluation.Describe().Name, modules.PackageEvaluation},
		{"interpretation", report.Describe().Name, modules.PackageInterpretation},
		{"actor", actor.Describe().Name, modules.PackageActor},
		{"plan", plan.Describe().Name, modules.PackagePlan},
		{"statistics", statistics.Describe().Name, modules.PackageStatistics},
		{"platform", platform.Describe().Name, modules.PackagePlatform},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("name = %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestAllPackagesIncludesEveryBusinessPackageAndPlatform(t *testing.T) {
	t.Parallel()

	want := []modules.PackageName{
		modules.PackageSurvey,
		modules.PackageModelCatalog,
		modules.PackageEvaluation,
		modules.PackageInterpretation,
		modules.PackageActor,
		modules.PackagePlan,
		modules.PackageStatistics,
		modules.PackagePlatform,
		modules.PackageIAM,
	}
	if !reflect.DeepEqual(modules.AllPackages, want) {
		t.Fatalf("AllPackages = %v, want %v", modules.AllPackages, want)
	}
}

func TestLegacyRegisteredModuleOrderMatchesInitializeSequence(t *testing.T) {
	t.Parallel()

	want := []string{"survey", "modelcatalog", "scale", "personalitymodel", "actor", "interpretation", "evaluation", "plan", "statistics"}
	if got := modules.LegacyRegisteredModuleOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("LegacyRegisteredModuleOrder() = %v, want %v", got, want)
	}
}
