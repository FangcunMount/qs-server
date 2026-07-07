package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// ResultProjection 补充原始 因子-计分结果 使用 算法-家族 semantics。
type ResultProjection interface {
	Apply(result *calculation.Result) *calculation.Result
}
