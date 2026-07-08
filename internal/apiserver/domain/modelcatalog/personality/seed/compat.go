// Package seed re-exports legacy personality seed constants.
package seed

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"

const (
	MBTIModelCode          = legacy.MBTIModelCode
	MBTIModelVersion       = legacy.MBTIModelVersion
	MBTIModelTitle         = legacy.MBTIModelTitle
	MBTIQuestionnaireCode  = legacy.MBTIQuestionnaireCode
	MBTIQuestionnaireTitle = legacy.MBTIQuestionnaireTitle

	SBTIModelCode          = legacy.SBTIModelCode
	SBTIModelVersion       = legacy.SBTIModelVersion
	SBTIModelTitle         = legacy.SBTIModelTitle
	SBTIQuestionnaireCode  = legacy.SBTIQuestionnaireCode
	SBTIQuestionnaireTitle = legacy.SBTIQuestionnaireTitle
)
