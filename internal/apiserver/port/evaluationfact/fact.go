// Package evaluationfact exposes the immutable scoring fact contract consumed
// outside Evaluation. Consumers depend on this port instead of Evaluation's
// application or domain package layout.
package evaluationfact

import (
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

type (
	ID              = domainoutcome.ID
	Record          = domainoutcome.Record
	Repository      = domainoutcome.Repository
	Execution       = domainoutcome.Execution
	ModelRef        = domainoutcome.ModelRef
	Summary         = domainoutcome.Summary
	Detail          = domainoutcome.Detail
	ScoreValue      = domainoutcome.ScoreValue
	ScoreKind       = domainoutcome.ScoreKind
	ResultLevel     = domainoutcome.ResultLevel
	DimensionResult = domainoutcome.DimensionResult
	ProfileResult   = domainoutcome.ProfileResult
	ValidityResult  = domainoutcome.ValidityResult
	NewRecordInput  = domainoutcome.NewRecordInput
	ModelIdentity   = domainoutcome.ModelIdentity
	RuntimeIdentity = domainoutcome.RuntimeIdentity
)

var (
	NewExecution = domainoutcome.NewExecution
	NewRecord    = domainoutcome.NewRecord
)

const (
	ScoreKindRawTotal     = domainoutcome.ScoreKindRawTotal
	ScoreKindMatchPercent = domainoutcome.ScoreKindMatchPercent
)
