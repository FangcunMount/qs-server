package modelcatalog

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

type NormPO struct {
	mongoBase.BaseDocument `bson:",inline"`

	TableVersion string         `bson:"table_version"`
	FormVariant  string         `bson:"form_variant,omitempty"`
	Kind         string         `bson:"kind,omitempty"`
	Algorithm    string         `bson:"algorithm,omitempty"`
	Factors      []NormFactorPO `bson:"factors,omitempty"`
}

type NormFactorPO struct {
	FactorCode string         `bson:"factor_code"`
	Bands      []NormBandPO   `bson:"bands,omitempty"`
	Lookup     []NormLookupPO `bson:"lookup,omitempty"`
}

type NormBandPO struct {
	MinAgeMonths int      `bson:"min_age_months,omitempty"`
	MaxAgeMonths int      `bson:"max_age_months,omitempty"`
	Gender       string   `bson:"gender,omitempty"`
	Mean         *float64 `bson:"mean,omitempty"`
	StdDev       *float64 `bson:"std_dev,omitempty"`
}

type NormLookupPO struct {
	RawScoreMin   float64  `bson:"raw_score_min"`
	RawScoreMax   float64  `bson:"raw_score_max"`
	TScore        float64  `bson:"t_score"`
	Percentile    float64  `bson:"percentile"`
	StandardScore *float64 `bson:"standard_score,omitempty"`
}

func (NormPO) CollectionName() string { return "assessment_norms" }

func (p *NormPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
}

func (p *NormPO) ToBsonM() (bson.M, error) {
	data, err := bson.Marshal(p)
	if err != nil {
		return nil, err
	}
	var out bson.M
	if err := bson.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func normToPO(value *norm.Norm) *NormPO {
	if value == nil {
		return nil
	}
	out := &NormPO{TableVersion: value.TableVersion, FormVariant: value.FormVariant, Kind: string(value.Kind), Algorithm: string(value.Algorithm)}
	if value.Factors != nil {
		out.Factors = make([]NormFactorPO, 0, len(value.Factors))
		for _, factor := range value.Factors {
			item := NormFactorPO{FactorCode: factor.FactorCode}
			if factor.Bands != nil {
				item.Bands = make([]NormBandPO, 0, len(factor.Bands))
				for _, band := range factor.Bands {
					item.Bands = append(item.Bands, NormBandPO{MinAgeMonths: band.MinAgeMonths, MaxAgeMonths: band.MaxAgeMonths, Gender: band.Gender, Mean: cloneFloat64(band.Mean), StdDev: cloneFloat64(band.StdDev)})
				}
			}
			if factor.Lookup != nil {
				item.Lookup = make([]NormLookupPO, 0, len(factor.Lookup))
				for _, lookup := range factor.Lookup {
					item.Lookup = append(item.Lookup, NormLookupPO{RawScoreMin: lookup.RawScoreMin, RawScoreMax: lookup.RawScoreMax, TScore: lookup.TScore, Percentile: lookup.Percentile, StandardScore: cloneFloat64(lookup.StandardScore)})
				}
			}
			out.Factors = append(out.Factors, item)
		}
	}
	return out
}

func normFromPO(value *NormPO) *norm.Norm {
	if value == nil {
		return nil
	}
	out := &norm.Norm{TableVersion: value.TableVersion, FormVariant: value.FormVariant, Kind: identity.Kind(value.Kind), Algorithm: identity.Algorithm(value.Algorithm)}
	if value.Factors != nil {
		out.Factors = make([]norm.FactorTable, 0, len(value.Factors))
		for _, factor := range value.Factors {
			item := norm.FactorTable{FactorCode: factor.FactorCode}
			if factor.Bands != nil {
				item.Bands = make([]norm.Band, 0, len(factor.Bands))
				for _, band := range factor.Bands {
					item.Bands = append(item.Bands, norm.Band{MinAgeMonths: band.MinAgeMonths, MaxAgeMonths: band.MaxAgeMonths, Gender: band.Gender, Mean: cloneFloat64(band.Mean), StdDev: cloneFloat64(band.StdDev)})
				}
			}
			if factor.Lookup != nil {
				item.Lookup = make([]norm.LookupEntry, 0, len(factor.Lookup))
				for _, lookup := range factor.Lookup {
					item.Lookup = append(item.Lookup, norm.LookupEntry{RawScoreMin: lookup.RawScoreMin, RawScoreMax: lookup.RawScoreMax, TScore: lookup.TScore, Percentile: lookup.Percentile, StandardScore: cloneFloat64(lookup.StandardScore)})
				}
			}
			out.Factors = append(out.Factors, item)
		}
	}
	return out
}
