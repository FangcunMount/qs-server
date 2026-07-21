package scoring

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

var (
	// ErrInterpretationAssetsMissing classifies a current ReportInput without frozen presentation assets.
	ErrInterpretationAssetsMissing = errors.New("factor interpretation assets missing")
	// ErrOutcomeCodeMissing classifies a current Outcome without its frozen decision code.
	ErrOutcomeCodeMissing = errors.New("factor outcome code missing")
	// ErrOutcomePresentationMiss classifies a frozen OutcomeCode without presentation copy.
	ErrOutcomePresentationMiss = errors.New("factor outcome presentation miss")
)

// interpretScaleFactor resolves current presentation exclusively from the frozen
// OutcomeCode and InterpretationAssets. It never re-matches scores or invents copy.
func interpretScaleFactor(model *ReportModel, fs FactorReportScore) (string, string, error) {
	hasAssets := model != nil && model.Assets != nil && model.Assets.IsMaterialized()
	if !hasAssets {
		return "", "", fmt.Errorf("%w: factor=%q", ErrInterpretationAssetsMissing, fs.FactorCode)
	}
	code := outcomeCodeFromFactorScore(fs)
	if code == "" {
		return "", "", fmt.Errorf("%w: factor=%q", ErrOutcomeCodeMissing, fs.FactorCode)
	}
	if conclusion, suggestion, ok := presentationFromOutcomeCode(*model.Assets, code); ok {
		return conclusion, suggestion, nil
	}
	return "", "", fmt.Errorf("%w: factor=%q outcome_code=%q", ErrOutcomePresentationMiss, fs.FactorCode, code)
}

func outcomeCodeFromFactorScore(fs FactorReportScore) string {
	if fs.Level != nil && fs.Level.Code != "" {
		return fs.Level.Code
	}
	return ""
}

func presentationFromOutcomeCode(assets interpretationassets.Assets, code string) (conclusion, suggestion string, ok bool) {
	if code == "" {
		return "", "", false
	}
	pres, found := assets.FindOutcome(code)
	if !found {
		return "", "", false
	}
	conclusion = firstNonEmpty(pres.Summary, pres.Title, pres.Description)
	suggestion = pres.Description
	if pres.Summary != "" && pres.Description != "" && pres.Summary != pres.Description {
		conclusion = pres.Summary
	}
	if conclusion == "" {
		return "", "", false
	}
	return conclusion, suggestion, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
