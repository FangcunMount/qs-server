package norm

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// Ref 指向常模表版本，不在测评模型内嵌常模数据。
type Ref struct {
	FactorCode       string
	NormTableVersion string
}

// Norm is immutable reference material addressed by TableVersion.
type Norm struct {
	TableVersion string
	FormVariant  string
	Kind         identity.Kind
	Algorithm    identity.Algorithm
	Factors      []FactorTable
}

type FactorTable struct {
	FactorCode string
	Bands      []Band
	Lookup     []LookupEntry
}

type Band struct {
	MinAgeMonths int
	MaxAgeMonths int
	Gender       string
	Mean         *float64
	StdDev       *float64
}

type LookupEntry struct {
	RawScoreMin   float64
	RawScoreMax   float64
	TScore        float64
	Percentile    float64
	StandardScore *float64
}
