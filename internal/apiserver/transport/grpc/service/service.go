// Package service is the transport-owned entrypoint for apiserver gRPC adapters.
//
// The concrete adapters still live in the legacy interface package during the
// gradual migration. New registry code should import this package so service
// ownership can move without touching callers again.
package service

import legacy "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/service"

var (
	NewActorService         = legacy.NewActorService
	NewAnswerSheetService   = legacy.NewAnswerSheetService
	NewEvaluationService    = legacy.NewEvaluationService
	NewInternalService      = legacy.NewInternalService
	NewPlanCommandService   = legacy.NewPlanCommandService
	NewQuestionnaireService = legacy.NewQuestionnaireService
	NewScaleService         = legacy.NewScaleService
)
