package norm

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
)

// Subject 携带人口学信息 用于 常模区间选择。
type Subject struct {
	AgeMonths *int
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

// MatchKind records whether resolution selected a demographic cohort or the
// explicitly unscoped fallback row/band.
type MatchKind string

const (
	MatchKindSpecific MatchKind = "specific"
	MatchKindGeneric  MatchKind = "generic"
)

// ErrorKind is the stable failure contract used by evaluation execution.
type ErrorKind string

const (
	ErrorKindSubjectMissing     ErrorKind = "norm_subject_missing"
	ErrorKindCohortNotFound     ErrorKind = "norm_cohort_not_found"
	ErrorKindRawScoreOutOfRange ErrorKind = "norm_raw_score_out_of_range"
	ErrorKindInvalid            ErrorKind = "norm_invalid"
)

// Resolution is a successful, explainable norm lookup.
type Resolution struct {
	Score     NormScore
	MatchKind MatchKind
}

// ResolutionError keeps the machine-stable failure kind alongside the factor
// and the deterministic list of missing subject fields.
type ResolutionError struct {
	Kind          ErrorKind
	FactorCode    string
	MissingFields []string
	Cause         error
}

func (e *ResolutionError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{string(e.Kind)}
	if e.FactorCode != "" {
		parts = append(parts, "factor="+e.FactorCode)
	}
	if len(e.MissingFields) > 0 {
		parts = append(parts, "missing="+strings.Join(e.MissingFields, ","))
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *ResolutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RunFailureKind exposes the stable kind for evaluation-run mapping without
// forcing application/execute to import this package.
func (e *ResolutionError) RunFailureKind() string {
	if e == nil {
		return ""
	}
	return string(e.Kind)
}

// ErrorKindOf extracts a stable norm failure kind from an error chain.
func ErrorKindOf(err error) (ErrorKind, bool) {
	var target *ResolutionError
	if !errors.As(err, &target) {
		return "", false
	}
	return target.Kind, true
}

// ResolveNormScore resolves one factor using the published precedence:
// direct specific, direct generic, parametric specific, parametric generic.
func ResolveNormScore(tables *NormTables, factorCode string, rawScore float64, subject Subject) (Resolution, error) {
	if err := ValidateTables(tables); err != nil {
		return Resolution{}, resolutionError(ErrorKindInvalid, factorCode, nil, err)
	}
	table, ok := factorTable(tables, factorCode)
	if !ok {
		return Resolution{}, resolutionError(ErrorKindInvalid, factorCode, nil, fmt.Errorf("norm factor is not defined"))
	}
	if score, kind, matched := lookupDirect(table, rawScore, subject); matched {
		return Resolution{Score: score, MatchKind: kind}, nil
	}
	if score, kind, matched := lookupParametric(table, rawScore, subject); matched {
		return Resolution{Score: score, MatchKind: kind}, nil
	}
	if missing := missingSubjectFields(table, rawScore, subject); len(missing) > 0 {
		return Resolution{}, resolutionError(ErrorKindSubjectMissing, factorCode, missing, nil)
	}
	if len(table.Lookup) > 0 && len(table.Bands) == 0 && !hasRawScoreCandidate(table.Lookup, rawScore) {
		return Resolution{}, resolutionError(ErrorKindRawScoreOutOfRange, factorCode, nil, nil)
	}
	return Resolution{}, resolutionError(ErrorKindCohortNotFound, factorCode, nil, nil)
}

func resolutionError(kind ErrorKind, factorCode string, missing []string, cause error) error {
	return &ResolutionError{Kind: kind, FactorCode: factorCode, MissingFields: missing, Cause: cause}
}

// NewInvalidError marks invalid catalog/runtime material without disguising it
// as an infrastructure dependency failure.
func NewInvalidError(factorCode string, cause error) error {
	return resolutionError(ErrorKindInvalid, factorCode, nil, cause)
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

func lookupDirect(table FactorNormTable, rawScore float64, subject Subject) (NormScore, MatchKind, bool) {
	var generic *NormLookupEntry
	for _, entry := range table.Lookup {
		if rawScore < entry.RawMin || rawScore > entry.RawMax {
			continue
		}
		if entryMatchesSubject(entry, subject) {
			return NormScore{TScore: entry.TScore, Percentile: entry.Percentile, StandardScore: cloneFloat64(entry.StandardScore), Reference: referenceFromLookup(entry)}, MatchKindSpecific, true
		}
		if entry.MinAgeMonths == 0 && entry.MaxAgeMonths == 0 && entry.Gender == "" && generic == nil {
			copy := entry
			generic = &copy
		}
	}
	if generic != nil {
		return NormScore{TScore: generic.TScore, Percentile: generic.Percentile, StandardScore: cloneFloat64(generic.StandardScore), Reference: referenceFromLookup(*generic)}, MatchKindGeneric, true
	}
	return NormScore{}, "", false
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
		if subject.AgeMonths == nil {
			return false
		}
		if entry.MinAgeMonths > 0 && *subject.AgeMonths < entry.MinAgeMonths {
			return false
		}
		if entry.MaxAgeMonths > 0 && *subject.AgeMonths > entry.MaxAgeMonths {
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

func lookupParametric(table FactorNormTable, rawScore float64, subject Subject) (NormScore, MatchKind, bool) {
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
		return scoreFromBand(band, rawScore), MatchKindSpecific, true
	}
	if generic != nil {
		return scoreFromBand(*generic, rawScore), MatchKindGeneric, true
	}
	return NormScore{}, "", false
}

func hasRawScoreCandidate(entries []NormLookupEntry, rawScore float64) bool {
	for _, entry := range entries {
		if rawScore >= entry.RawMin && rawScore <= entry.RawMax {
			return true
		}
	}
	return false
}

func missingSubjectFields(table FactorNormTable, rawScore float64, subject Subject) []string {
	missing := make(map[string]struct{}, 2)
	for _, entry := range table.Lookup {
		if rawScore < entry.RawMin || rawScore > entry.RawMax || isGenericLookup(entry) {
			continue
		}
		collectMissingScope(entry.MinAgeMonths, entry.MaxAgeMonths, entry.Gender, subject, missing)
	}
	for _, band := range table.Bands {
		if isGenericBand(band) {
			continue
		}
		collectMissingScope(band.MinAgeMonths, band.MaxAgeMonths, band.Gender, subject, missing)
	}
	fields := make([]string, 0, len(missing))
	for field := range missing {
		fields = append(fields, field)
	}
	sort.Slice(fields, func(i, j int) bool {
		order := map[string]int{"age_months": 0, "gender": 1}
		return order[fields[i]] < order[fields[j]]
	})
	return fields
}

func collectMissingScope(minAge, maxAge int, gender string, subject Subject, missing map[string]struct{}) {
	if (minAge > 0 || maxAge > 0) && subject.AgeMonths == nil {
		missing["age_months"] = struct{}{}
	}
	if gender != "" && subject.Gender == "" {
		missing["gender"] = struct{}{}
	}
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

func isGenericLookup(entry NormLookupEntry) bool {
	return entry.MinAgeMonths == 0 && entry.MaxAgeMonths == 0 && entry.Gender == ""
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
		if subject.AgeMonths == nil {
			return false
		}
		if band.MinAgeMonths > 0 && *subject.AgeMonths < band.MinAgeMonths {
			return false
		}
		if band.MaxAgeMonths > 0 && *subject.AgeMonths > band.MaxAgeMonths {
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
	seen := make(map[string]struct{}, len(tables.Factors))
	for factorIndex, factor := range tables.Factors {
		if factor.FactorCode == "" {
			return fmt.Errorf("norm factor %d code is required", factorIndex)
		}
		if _, exists := seen[factor.FactorCode]; exists {
			return fmt.Errorf("norm factor %s is duplicated", factor.FactorCode)
		}
		seen[factor.FactorCode] = struct{}{}
		if len(factor.Lookup) == 0 && len(factor.Bands) == 0 {
			return fmt.Errorf("norm factor %s requires lookup rows or bands", factor.FactorCode)
		}
		for index, entry := range factor.Lookup {
			if !finite(entry.RawMin) || !finite(entry.RawMax) || entry.RawMin > entry.RawMax {
				return fmt.Errorf("norm factor %s lookup %d has invalid raw-score range", factor.FactorCode, index)
			}
			if err := validateRuntimeScope(entry.MinAgeMonths, entry.MaxAgeMonths, entry.Gender); err != nil {
				return fmt.Errorf("norm factor %s lookup %d: %w", factor.FactorCode, index, err)
			}
			if !finite(entry.TScore) || !finite(entry.Percentile) || entry.Percentile < 0 || entry.Percentile > 100 || (entry.StandardScore != nil && !finite(*entry.StandardScore)) {
				return fmt.Errorf("norm factor %s lookup %d has invalid derived score", factor.FactorCode, index)
			}
		}
		for left := 0; left < len(factor.Lookup); left++ {
			for right := left + 1; right < len(factor.Lookup); right++ {
				if runtimeRawRangesOverlap(factor.Lookup[left], factor.Lookup[right]) && runtimeLookupScopesAmbiguous(factor.Lookup[left], factor.Lookup[right]) {
					return fmt.Errorf("norm factor %s lookup rows %d and %d overlap for the same demographic scope", factor.FactorCode, left, right)
				}
			}
		}
		for index, band := range factor.Bands {
			if err := validateRuntimeScope(band.MinAgeMonths, band.MaxAgeMonths, band.Gender); err != nil {
				return fmt.Errorf("norm factor %s band %d: %w", factor.FactorCode, index, err)
			}
			if band.Mean == nil || band.StdDev == nil || !finite(*band.Mean) || !finite(*band.StdDev) || *band.StdDev <= 0 {
				return fmt.Errorf("norm factor %s band %d requires finite mean and positive std_dev", factor.FactorCode, index)
			}
		}
		for left := 0; left < len(factor.Bands); left++ {
			for right := left + 1; right < len(factor.Bands); right++ {
				if runtimeBandScopesAmbiguous(factor.Bands[left], factor.Bands[right]) {
					return fmt.Errorf("norm factor %s bands %d and %d overlap for the same demographic scope", factor.FactorCode, left, right)
				}
			}
		}
	}
	return nil
}

func validateRuntimeScope(minAge, maxAge int, gender string) error {
	if minAge < 0 || maxAge < 0 || (maxAge > 0 && minAge > maxAge) {
		return fmt.Errorf("invalid age range")
	}
	if gender != "" && gender != "male" && gender != "female" {
		return fmt.Errorf("invalid gender %q", gender)
	}
	return nil
}

func runtimeRawRangesOverlap(left, right NormLookupEntry) bool {
	return left.RawMin <= right.RawMax && right.RawMin <= left.RawMax
}

func runtimeLookupScopesAmbiguous(left, right NormLookupEntry) bool {
	leftGeneric, rightGeneric := isGenericLookup(left), isGenericLookup(right)
	if leftGeneric || rightGeneric {
		return leftGeneric && rightGeneric
	}
	return runtimeScopesOverlap(left.MinAgeMonths, left.MaxAgeMonths, left.Gender, right.MinAgeMonths, right.MaxAgeMonths, right.Gender)
}

func runtimeBandScopesAmbiguous(left, right NormBand) bool {
	leftGeneric, rightGeneric := isGenericBand(left), isGenericBand(right)
	if leftGeneric || rightGeneric {
		return leftGeneric && rightGeneric
	}
	return runtimeScopesOverlap(left.MinAgeMonths, left.MaxAgeMonths, left.Gender, right.MinAgeMonths, right.MaxAgeMonths, right.Gender)
}

func runtimeScopesOverlap(leftMin, leftMax int, leftGender string, rightMin, rightMax int, rightGender string) bool {
	if leftGender != "" && rightGender != "" && leftGender != rightGender {
		return false
	}
	if leftMax == 0 {
		leftMax = math.MaxInt
	}
	if rightMax == 0 {
		rightMax = math.MaxInt
	}
	return leftMin <= rightMax && rightMin <= leftMax
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
