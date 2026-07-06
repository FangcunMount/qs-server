// Package scale 是量表应用层的稳定入口：对外类型别名与工厂函数，实现按子包拆分后的装配兼容。
package scale

import (
	applifecycle "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale/lifecycle"
	appports "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale/ports"
	appshared "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale/shared"
)

// --- Driving ports（稳定 API 面）---

type (
	ScaleLifecycleService          = appports.ScaleLifecycleService
	ScaleFactorService             = appports.ScaleFactorService
	ScaleQueryService              = appports.ScaleQueryService
	ScaleCategoryService           = appports.ScaleCategoryService
	AssessmentScaleContextResolver = appports.AssessmentScaleContextResolver
	ScaleQRCodeGenerator           = appports.ScaleQRCodeGenerator
	ScaleQRCodeQueryService        = appports.ScaleQRCodeQueryService
)

// QuestionnaireBindingSyncer 问卷发布后同步量表绑定版本（实现位于 lifecycle 子包）。
type QuestionnaireBindingSyncer = applifecycle.QuestionnaireBindingSyncer

// --- DTO / 结果模型（来自 shared）---

type (
	CreateScaleDTO                = appshared.CreateScaleDTO
	UpdateScaleBasicInfoDTO       = appshared.UpdateScaleBasicInfoDTO
	UpdateScaleQuestionnaireDTO   = appshared.UpdateScaleQuestionnaireDTO
	ScoringParamsDTO              = appshared.ScoringParamsDTO
	AddFactorDTO                  = appshared.AddFactorDTO
	UpdateFactorDTO               = appshared.UpdateFactorDTO
	FactorDTO                     = appshared.FactorDTO
	InterpretRuleDTO              = appshared.InterpretRuleDTO
	UpdateFactorInterpretRulesDTO = appshared.UpdateFactorInterpretRulesDTO
	ListScalesDTO                 = appshared.ListScalesDTO
	ScaleListFilter               = appshared.ScaleListFilter
	ListHotScalesDTO              = appshared.ListHotScalesDTO
	AssessmentScaleContextResult  = appshared.AssessmentScaleContextResult
	ScaleCategoriesResult         = appshared.ScaleCategoriesResult
	CategoryOption                = appshared.CategoryOption
	StageOption                   = appshared.StageOption
	ApplicableAgeOption           = appshared.ApplicableAgeOption
	ReporterOption                = appshared.ReporterOption
	TagOption                     = appshared.TagOption
	ScaleResult                   = appshared.ScaleResult
	FactorResult                  = appshared.FactorResult
	InterpretRuleResult           = appshared.InterpretRuleResult
	ScaleListResult               = appshared.ScaleListResult
	ScaleSummaryResult            = appshared.ScaleSummaryResult
	ScaleSummaryListResult        = appshared.ScaleSummaryListResult
	HotScaleSummaryResult         = appshared.HotScaleSummaryResult
	HotScaleListResult            = appshared.HotScaleListResult
)
