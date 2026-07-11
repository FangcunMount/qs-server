// Package evaluationcompat contains the temporary in-process Evaluation fact
// bridge used by Preview and legacy report characterization paths.
package evaluationcompat

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
)

type (
	Outcome               = evaloutcome.Outcome
	PersonalityTypeDetail = outcometypology.PersonalityTypeDetail
	TraitProfileDetail    = outcometypology.TraitProfileDetail
	MBTIResultDetail      = typologylegacy.MBTIResultDetail
)

var (
	RestoreExecution               = evaloutcome.RestoreExecution
	RestoreReportInput             = evaloutcome.RestoreReportInput
	ModelRouteFromInput            = evaloutcome.ModelRouteFromInput
	ModelRefFromAssessment         = evaloutcome.ModelRefFromAssessment
	ScoringProjectionFromExecution = evaloutcome.ScoringProjectionFromExecution
)
