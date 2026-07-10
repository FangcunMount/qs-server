package container

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	evalmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	iammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/iam"
	reportmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	planmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/plan"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// Module is the lifecycle contract for loaded container modules.
type Module = modules.Module

// ModuleInfo describes a loaded container module.
type ModuleInfo = modules.ModuleInfo

type (
	SurveyModule            = surveymod.Module
	QuestionnaireSubModule  = surveymod.QuestionnaireSubModule
	AnswerSheetSubModule    = surveymod.AnswerSheetSubModule
	AssessmentModelModule   = ammod.Module
	TypologyModelModule     = ammod.Typology
	ActorModule             = actormod.Module
	EvaluationModule        = evalmod.Module
	ReportModule            = reportmod.Module
	PlanModule              = planmod.Module
	StatisticsModule        = statmod.Module
	IAMModule               = iammod.Module
	IAMModuleRuntimeOptions = iammod.RuntimeOptions
)

// NewIAMModule creates the IAM integration module.
func NewIAMModule(ctx context.Context, opts *options.IAMOptions) (*IAMModule, error) {
	return iammod.New(ctx, opts)
}

// NewIAMModuleWithRuntimeOptions creates the IAM integration module with runtime limiters.
func NewIAMModuleWithRuntimeOptions(ctx context.Context, opts *options.IAMOptions, runtime IAMModuleRuntimeOptions) (*IAMModule, error) {
	return iammod.NewWithRuntimeOptions(ctx, opts, runtime)
}
