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
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
)

func TestBusinessModuleInfoUsesCanonicalRegistrationNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		module modules.Module
		want   modules.PackageName
	}{
		{"survey", &survey.Module{}, modules.PackageSurvey},
		{"modelcatalog", &modelcatalog.Module{}, modules.PackageModelCatalog},
		{"evaluation", &evaluation.Module{}, modules.PackageEvaluation},
		{"interpretation", &report.Module{}, modules.PackageInterpretation},
		{"actor", &actor.Module{}, modules.PackageActor},
		{"plan", &plan.Module{}, modules.PackagePlan},
		{"statistics", &statistics.Module{}, modules.PackageStatistics},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := tc.module.ModuleInfo()
			if info.Name != string(tc.want) {
				t.Fatalf("module registration name = %q, want %q", info.Name, tc.want)
			}
			if info.Version == "" || info.Description == "" {
				t.Fatalf("module lifecycle metadata is incomplete: %#v", info)
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

func TestRegisteredBusinessModuleOrderMatchesInitializeSequence(t *testing.T) {
	t.Parallel()

	want := []string{"survey", "interpretation", "modelcatalog", "actor", "evaluation", "plan", "statistics"}
	if got := modules.RegisteredBusinessModuleOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredBusinessModuleOrder() = %v, want %v", got, want)
	}
}
