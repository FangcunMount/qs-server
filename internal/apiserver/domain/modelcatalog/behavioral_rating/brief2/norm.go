package brief2

import (
	"fmt"
	"math"
)

// Subject carries demographics used for norm-band selection.
type Subject struct {
	AgeMonths int
	Gender    string
}

// NormTables holds Brief-2 norm configuration parsed from payload.
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
	RawMin     float64
	RawMax     float64
	TScore     float64
	Percentile float64
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
	TScore     float64
	Percentile float64
}

// LookupNormScore resolves raw score to T-score and percentile using configured tables.
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

// InterpretTScore maps a T-score to clinical interpretation for a factor.
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
			return NormScore{TScore: entry.TScore, Percentile: entry.Percentile}, true
		}
	}
	return NormScore{}, false
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
	// Approximate normal CDF for T ~ N(50,10).
	z := (tScore - 50) / 10
	return roundScore(normalCDF(z) * 100)
}

func normalCDF(z float64) float64 {
	return 0.5 * (1 + math.Erf(z/math.Sqrt2))
}

func roundScore(v float64) float64 {
	return math.Round(v*10) / 10
}

// ValidateTables returns an error when norm tables are structurally invalid.
func ValidateTables(tables *NormTables) error {
	if tables == nil {
		return fmt.Errorf("brief2 norm tables are nil")
	}
	if len(tables.Factors) == 0 {
		return fmt.Errorf("brief2 norm tables require at least one factor")
	}
	return nil
}
