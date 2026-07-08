// Package hotrank re-exports scale definition hot-rank read-model ports.
package hotrank

import scalehotrank "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/definition/hotrank"

type (
	Entry = scalehotrank.Entry
	Projection = scalehotrank.Projection
	Query = scalehotrank.Query
	ReadModel = scalehotrank.ReadModel
	SubmissionFact = scalehotrank.SubmissionFact
)

