package norm

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"

// Ref 指向常模表版本，不在测评模型内嵌常模数据。
type Ref = factor.NormRef

// Norm 是常模资料的领域占位，后续由 norm repository 或外部资料源承载。
type Norm struct {
	TableVersion string
	FactorCode   string
	Entries      []Entry
}

type Entry struct {
	RawScoreMin float64
	RawScoreMax float64
	Level       string
	Percentile  *float64
}
