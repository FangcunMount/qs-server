// Package handler is the transport-owned entrypoint for apiserver REST handlers.
//
// The concrete handlers still live in the legacy interface package during the
// gradual migration. New transport code should import this package so handler
// ownership can move without changing router deps again.
package handler

import legacy "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"

type (
	AnswerSheetHandler       = legacy.AnswerSheetHandler
	AssessmentEntryHandler   = legacy.AssessmentEntryHandler
	CodesHandler             = legacy.CodesHandler
	EvaluationHandler        = legacy.EvaluationHandler
	OperatorClinicianHandler = legacy.OperatorClinicianHandler
	PlanHandler              = legacy.PlanHandler
	QuestionnaireHandler     = legacy.QuestionnaireHandler
	QRCodeHandler            = legacy.QRCodeHandler
	ScaleHandler             = legacy.ScaleHandler
	StatisticsHandler        = legacy.StatisticsHandler
	TesteeHandler            = legacy.TesteeHandler
)

var (
	NewAnswerSheetHandler       = legacy.NewAnswerSheetHandler
	NewAssessmentEntryHandler   = legacy.NewAssessmentEntryHandler
	NewCodesHandler             = legacy.NewCodesHandler
	NewEvaluationHandler        = legacy.NewEvaluationHandler
	NewOperatorClinicianHandler = legacy.NewOperatorClinicianHandler
	NewPlanHandler              = legacy.NewPlanHandler
	NewQuestionnaireHandler     = legacy.NewQuestionnaireHandler
	NewQRCodeHandler            = legacy.NewQRCodeHandler
	NewScaleHandler             = legacy.NewScaleHandler
	NewStatisticsHandler        = legacy.NewStatisticsHandler
	NewTesteeHandler            = legacy.NewTesteeHandler
)
