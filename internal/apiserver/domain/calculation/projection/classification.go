package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// ClassificationProjection 补充scored 结果 使用 类型学 decision semantics。
// (type 编码, 特质画像, match percent, etc.)。
type ClassificationProjection interface {
	Apply(result *calculation.Result) (*calculation.Result, error)
}

// IdentityClassificationProjection 是pass-通过 用于 tests 和 no-op 装配。
type IdentityClassificationProjection struct{}

func (IdentityClassificationProjection) Apply(result *calculation.Result) (*calculation.Result, error) {
	return result, nil
}
