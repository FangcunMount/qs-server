package norm

import (
	"fmt"
	"math"
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
	TScore        float64
	Percentile    float64
	StandardScore *float64
}

type TScoreInterpretRule struct {
	FactorCode string
	Ranges     []TScoreRange
}

type TScoreRange struct {
	MinT       float64
	MaxT       float64
	Level      string
	Conclusion string
	Suggestion string
}

type NormScore struct {
	TScore        float64
	Percentile    float64
	StandardScore *float64
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
	if score, ok := lookupDirect(table, rawScore); ok {
		return score, true
	}
	if score, ok := lookupParametric(table, rawScore, subject); ok {
		return score, true
	}
	return NormScore{}, false
}

// InterpretTScore 映射T 分 到 临床解释 用于 因子。
func InterpretTScore(tables *NormTables, factorCode string, tScore float64) (level, conclusion, suggestion string, ok bool) {
	if tables == nil {
		return "", "", "", false
	}
	for _, rule := range tables.TScoreRules {
		if rule.FactorCode != factorCode {
			continue
		}
		for _, item := range rule.Ranges {
			if tScore >= item.MinT && tScore <= item.MaxT {
				return item.Level, item.Conclusion, item.Suggestion, true
			}
		}
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

func lookupDirect(table FactorNormTable, rawScore float64) (NormScore, bool) {
	for _, entry := range table.Lookup {
		if rawScore >= entry.RawMin && rawScore <= entry.RawMax {
			return NormScore{TScore: entry.TScore, Percentile: entry.Percentile, StandardScore: cloneFloat64(entry.StandardScore)}, true
		}
	}
	return NormScore{}, false
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func lookupParametric(table FactorNormTable, rawScore float64, subject Subject) (NormScore, bool) {
	for _, band := range table.Bands {
		if !bandMatchesSubject(band, subject) {
			continue
		}
		if band.Mean == nil || band.StdDev == nil || *band.StdDev == 0 {
			continue
		}
		tScore := 50 + 10*((rawScore-*band.Mean) / *band.StdDev)
		return NormScore{
			TScore:     roundScore(tScore),
			Percentile: percentileFromTScore(tScore),
		}, true
	}
	return NormScore{}, false
}

func bandMatchesSubject(band NormBand, subject Subject) bool {
	if band.Gender != "" && subject.Gender != "" && band.Gender != subject.Gender {
		return false
	}
	if subject.AgeMonths > 0 {
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
