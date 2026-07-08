package modules

// Package modules defines the target container module language.
//
// Business assembly and bootstrap live under modules/*/wire.go;
// container/module_init.go wires integration inputs at the composition root.

// Module is the lifecycle contract for container business modules.
type Module interface {
	CheckHealth() error
	Cleanup() error
	ModuleInfo() ModuleInfo
}

// ModuleInfo describes a loaded container module.
type ModuleInfo struct {
	Name        string
	Version     string
	Description string
}

// PackageName identifies a container module package directory under modules/.
type PackageName string

const (
	PackageSurvey PackageName = "survey"
	// value matches the modules/ directory (modelcatalog); business display name is "model-catalog".
	PackageModelCatalog   PackageName = "modelcatalog"
	PackageEvaluation     PackageName = "evaluation"
	PackageInterpretation PackageName = "interpretation"
	PackageActor          PackageName = "actor"
	PackagePlan           PackageName = "plan"
	PackageStatistics     PackageName = "statistics"
	PackagePlatform       PackageName = "platform"
	PackageIAM            PackageName = "iam"
)

// BusinessPackages are core and supporting business modules.
var BusinessPackages = []PackageName{
	PackageSurvey,
	PackageModelCatalog,
	PackageEvaluation,
	PackageInterpretation,
	PackageActor,
	PackagePlan,
	PackageStatistics,
}

// AllPackages includes business modules and integration packages.
var AllPackages = append(append([]PackageName{}, BusinessPackages...), PackagePlatform, PackageIAM)

// LegacyInitStep documents one Initialize() business-module phase.
type LegacyInitStep struct {
	InitMethod    string
	RegisterNames []string
}

// LegacyInitializeSequence is the current Container.Initialize business-module order.
var LegacyInitializeSequence = []LegacyInitStep{
	{InitMethod: "initSurveyModule", RegisterNames: []string{"survey"}},
	{InitMethod: "initModelCatalogModule", RegisterNames: []string{"modelcatalog", "scale", "personalitymodel"}},
	{InitMethod: "initActorModule", RegisterNames: []string{"actor"}},
	{InitMethod: "initReportModule", RegisterNames: []string{"interpretation"}},
	{InitMethod: "initEvaluationModule", RegisterNames: []string{"evaluation"}},
	{InitMethod: "initPlanModule", RegisterNames: []string{"plan"}},
	{InitMethod: "initStatisticsModule", RegisterNames: []string{"statistics"}},
}

// LegacyRegisteredModuleOrder flattens registerModule keys from LegacyInitializeSequence.
func LegacyRegisteredModuleOrder() []string {
	names := make([]string, 0)
	for _, step := range LegacyInitializeSequence {
		names = append(names, step.RegisterNames...)
	}
	return names
}

// LegacyBootstrapFiles are flat bootstrap_*.go files allowed at container root.
// Platform bootstrap lives under modules/platform/bootstrap_*.go.
var LegacyBootstrapFiles = []string{}

// MigratedModulePackages host assembly logic under modules/.
var MigratedModulePackages = []PackageName{
	PackageSurvey,
	PackageModelCatalog,
	PackageEvaluation,
	PackageInterpretation,
	PackageActor,
	PackagePlan,
	PackageStatistics,
}

// MigratedModuleAssembleFiles lists assembly entry files for migrated packages.
var MigratedModuleAssembleFiles = map[PackageName][]string{
	PackageSurvey:         {"assemble.go", "scale_infra.go", "wire.go", "install.go"},
	PackageModelCatalog:   {"module_aggregate.go", "module_ports.go", "assemble_scoring.go", "assemble_typology.go", "assemble_norming.go", "assemble_taskperformance.go", "default_descriptors.go", "wire.go", "install.go"},
	PackageActor:          {"assemble.go", "wire.go", "install.go"},
	PackagePlan:           {"assemble.go", "wire.go", "install.go"},
	PackageStatistics:     {"assemble.go", "wire.go", "install.go"},
	PackageEvaluation:     {"assemble.go", "descriptors.go", "wire.go", "install.go"},
	PackageInterpretation: {"assemble.go", "wire.go", "install.go"},
}

// MigratedModuleBootstrapFiles lists bootstrap entry files for migrated packages.
// Container module_init.go calls Wire() to keep integration inputs separate from module Deps.
var MigratedModuleBootstrapFiles = map[PackageName]string{
	PackageSurvey:         "bootstrap.go",
	PackageModelCatalog:   "bootstrap.go",
	PackageEvaluation:     "bootstrap.go",
	PackageInterpretation: "bootstrap.go",
	PackageActor:          "bootstrap.go",
	PackagePlan:           "bootstrap.go",
	PackageStatistics:     "bootstrap.go",
}

// MigratedModuleTransportExportFiles lists transport export entry files per package.
var MigratedModuleTransportExportFiles = map[PackageName][]string{
	PackageSurvey:       {"exports_rest.go", "exports_grpc.go"},
	PackageModelCatalog: {"exports_rest.go", "exports_grpc.go"},
	PackageActor:        {"exports_rest.go", "exports_grpc.go"},
	PackageEvaluation:   {"exports_rest.go", "exports_grpc.go"},
	PackagePlan:         {"exports_rest.go", "exports_grpc.go"},
	PackageStatistics:   {"exports_rest.go", "exports_grpc.go"},
	PackagePlatform:     {"exports_rest.go", "exports_grpc.go"},
}

// PlatformBootstrapFiles lists platform integration bootstrap entry files.
var PlatformBootstrapFiles = []string{
	"bootstrap_codes.go",
	"bootstrap_qrcode.go",
	"bootstrap_notification.go",
	"wire.go",
	"install.go",
}
