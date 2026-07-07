package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// ResultProjection enriches a raw factor-scoring result with algorithm-family semantics.
type ResultProjection interface {
	Apply(result *calculation.Result) *calculation.Result
}
