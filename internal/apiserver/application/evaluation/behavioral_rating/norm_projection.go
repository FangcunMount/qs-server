package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
)

// brief2NormProjection applies Brief-2 norm/T-score tables on top of raw scale scores.
// Delegates to calculation/norm.Projection; retained as orchestration wrapper for behavioral_rating.
type brief2NormProjection struct {
	tables               *brief2norm.NormTables
	subject              brief2norm.Subject
	primaryDimensionCode string
}

func (p brief2NormProjection) apply(result *calculation.Result) *calculation.Result {
	return calcnorm.Projection{
		Tables:               p.tables,
		Subject:              calcnorm.Subject(p.subject),
		PrimaryDimensionCode: p.primaryDimensionCode,
	}.Apply(result)
}
