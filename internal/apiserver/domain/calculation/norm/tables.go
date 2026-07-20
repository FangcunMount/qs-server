package norm

import (
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
)

// Subject 携带人口学信息 用于 常模区间选择。
type Subject struct {
	AgeMonths int
	Gender    string
}

// NormTables 保存常模 配置 用于 因子_常模 lookups。
type NormTables struct {
	FormVariant      string
	NormTableVersion string
	Factors          []FactorNormTable
	TScoreRules      []TScoreInterpretRule
}

type FactorNormTable struct {
	FactorCode string
	Bands      []NormBand
	Lookup     []NormLookupEntry
}

type NormBand struct {
	MinAgeMonths int
	MaxAgeMonths int
	Gender       string
	Mean         *float64
	StdDev       *float64
}

type NormLookupEntry struct {
	RawMin        float64
	RawMax        float64
	MinAgeMonths  int
	MaxAgeMonths  int
	Gender        string
	TScore        float64
	Percentile    float64
	StandardScore *float64
}

type TScoreInterpretRule struct {
	FactorCode string
	Ranges     []TScoreRange
}

type TScoreRange struct {
	MinT         float64
	MaxT         float64
	MaxInclusive bool
	UnboundedMax bool
	Level        string
	Conclusion   string
	Suggestion   string
}

type NormScore struct {
	TScore        float64
	Percentile    float64
	StandardScore *float64
	Reference     NormReference
}

// NormReference identifies the selected norm cohort. Zero-value cohort fields
// represent a generic lookup entry.
type NormReference struct {
	MinAgeMonths int
	MaxAgeMonths int
	Gender       string
}

// LookupNormScore 解析原始分 到 T 分 和 百分位 using 配置化 tables。
func LookupNormScore(tables *NormTables, factorCode string, rawScore float64, subject Subject) (NormScore, bool) {
	if tables == nil {
		return NormScore{}, false
	}
	table, ok := factorTable(tables, factorCode)
	if !ok {
		return NormScore{}, false
	}
	if score, ok := lookupDirect(table, rawScore, subject); ok {
		return score, true
	}
	if score, ok := lookupParametric(table, rawScore, subject); ok {
		return score, true
	}
	return NormScore{}, false
}

// InterpretTScore 映射 T 分到临床解释。
// Matching uses the shared ScoreRange endpoint contract (half-open by default;
// explicit max_inclusive / unbounded_max; legacy last-inclusive when unset).
func InterpretTScore(tables *NormTables, factorCode string, tScore float64) (level, text, suggestion string, ok bool) {
	if tables == nil {
		return "", "", "", false
	}
	for _, rule := range tables.TScoreRules {
		if rule.FactorCode != factorCode {
			continue
		}
		bounds := make([]scorerange.Bound, len(rule.Ranges))
		for i, item := range rule.Ranges {
			bounds[i] = scorerange.Bound{
				Min: item.MinT, Max: item.MaxT, MaxInclusive: item.MaxInclusive, UnboundedMax: item.UnboundedMax,
			}
		}
		index, matched := scorerange.MatchBounds(tScore, bounds)
		if !matched {
			continue
		}
		item := rule.Ranges[index]
		return item.Level, item.Conclusion, item.Suggestion, true
	}
	return "", "", "", false
}

func factorTable(tables *NormTables, factorCode string) (FactorNormTable, bool) {
	for _, table := range tables.Factors {
		if table.FactorCode == factorCode {
			return table, true
		}
	}
	return FactorNormTable{}, false
}

func lookupDirect(table FactorNormTable, rawScore float64, subject Subject) (NormScore, bool) {
	var generic *NormLookupEntry
	for _, entry := range table.Lookup {
		if rawScore < entry.RawMin || rawScore > entry.RawMax {
			continue
		}
		if entryMatchesSubject(entry, subject) {
			return NormScore{TScore: entry.TScore, Percentile: entry.Percentile, StandardScore: cloneFloat64(entry.StandardScore), Reference: referenceFromLookup(entry)}, true
		}
		if entry.MinAgeMonths == 0 && entry.MaxAgeMonths == 0 && entry.Gender == "" && generic == nil {
			copy := entry
			generic = &copy
		}
	}
	if generic != nil {
		return NormScore{TScore: generic.TScore, Percentile: generic.Percentile, StandardScore: cloneFloat64(generic.StandardScore), Reference: referenceFromLookup(*generic)}, true
	}
	return NormScore{}, false
}

func entryMatchesSubject(entry NormLookupEntry, subject Subject) bool {
	if entry.MinAgeMonths == 0 && entry.MaxAgeMonths == 0 && entry.Gender == "" {
		return false
	}
	if entry.Gender != "" {
		if subject.Gender == "" || entry.Gender != subject.Gender {
			return false
		}
	}
	if entry.MinAgeMonths > 0 || entry.MaxAgeMonths > 0 {
		if subject.AgeMonths <= 0 {
			return false
		}
		if entry.MinAgeMonths > 0 && subject.AgeMonths < entry.MinAgeMonths {
			return false
		}
		if entry.MaxAgeMonths > 0 && subject.AgeMonths > entry.MaxAgeMonths {
			return false
		}
	}
	return true
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func lookupParametric(table FactorNormTable, rawScore float64, subject Subject) (NormScore, bool) {
	var generic *NormBand
	for _, band := range table.Bands {
		if band.Mean == nil || band.StdDev == nil || *band.StdDev == 0 {
			continue
		}
		if isGenericBand(band) {
			if generic == nil {
				copy := band
				generic = &copy
			}
			continue
		}
		if !bandMatchesSubject(band, subject) {
			continue
		}
		return scoreFromBand(band, rawScore), true
	}
	if generic != nil {
		return scoreFromBand(*generic, rawScore), true
	}
	return NormScore{}, false
}

func scoreFromBand(band NormBand, rawScore float64) NormScore {
	tScore := 50 + 10*((rawScore-*band.Mean) / *band.StdDev)
	return NormScore{
		TScore:     roundScore(tScore),
		Percentile: percentileFromTScore(tScore),
		Reference:  referenceFromBand(band),
	}
}

func referenceFromLookup(entry NormLookupEntry) NormReference {
	return NormReference{MinAgeMonths: entry.MinAgeMonths, MaxAgeMonths: entry.MaxAgeMonths, Gender: entry.Gender}
}

func referenceFromBand(band NormBand) NormReference {
	return NormReference{MinAgeMonths: band.MinAgeMonths, MaxAgeMonths: band.MaxAgeMonths, Gender: band.Gender}
}

func isGenericBand(band NormBand) bool {
	return band.MinAgeMonths == 0 && band.MaxAgeMonths == 0 && band.Gender == ""
}

// bandMatchesSubject applies the same strict demographic rule as entryMatchesSubject:
// a cohort-scoped band cannot match when the required subject fields are missing.
func bandMatchesSubject(band NormBand, subject Subject) bool {
	if isGenericBand(band) {
		return true
	}
	if band.Gender != "" {
		if subject.Gender == "" || band.Gender != subject.Gender {
			return false
		}
	}
	if band.MinAgeMonths > 0 || band.MaxAgeMonths > 0 {
		if subject.AgeMonths <= 0 {
			return false
		}
		if band.MinAgeMonths > 0 && subject.AgeMonths < band.MinAgeMonths {
			return false
		}
		if band.MaxAgeMonths > 0 && subject.AgeMonths > band.MaxAgeMonths {
			return false
		}
	}
	return true
}

func percentileFromTScore(tScore float64) float64 {
	// Approximate 正态 CDF 用于 T ~ N(50,10)。
	z := (tScore - 50) / 10
	return roundScore(normalCDF(z) * 100)
}

func normalCDF(z float64) float64 {
	return 0.5 * (1 + math.Erf(z/math.Sqrt2))
}

func roundScore(v float64) float64 {
	return math.Round(v*10) / 10
}

// ValidateTables 返回error when 常模表 是 结构无效。
func ValidateTables(tables *NormTables) error {
	if tables == nil {
		return fmt.Errorf("norm tables are nil")
	}
	if len(tables.Factors) == 0 {
		return fmt.Errorf("norm tables require at least one factor")
	}
	return nil
}
