package factor

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func toFactorDomain(
	code, title, factorType string,
	isTotalScore, isShow bool,
	questionCodes []string,
	scoringStrategy string,
	scoringParams *shared.ScoringParamsDTO,
	maxScore *float64,
	interpretRules []shared.InterpretRuleDTO,
) (*scaledefinition.Factor, error) {
	strategy := scaledefinition.ScoringStrategySum
	if scoringStrategy != "" {
		strategy = scaledefinition.ScoringStrategyCode(scoringStrategy)
	}

	fType := scaledefinition.ParseFactorType(factorType)

	scoringParamsDomain := scoringParamsFromDTO(scoringParams)
	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode(code),
		title,
		scaledefinition.WithFactorType(fType),
		scaledefinition.WithIsTotalScore(isTotalScore),
		scaledefinition.WithIsShow(isShow),
		scaledefinition.WithQuestionCodes(metaCodesFromStrings(questionCodes)),
		scaledefinition.WithScoringStrategy(strategy),
		scaledefinition.WithScoringParams(scoringParamsDomain),
		scaledefinition.WithMaxScore(maxScore),
		scaledefinition.WithInterpretRules(shared.InterpretRulesFromDTOs(interpretRules)),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建因子失败")
	}
	return factor, nil
}

func scoringParamsFromDTO(scoringParams *shared.ScoringParamsDTO) *scaledefinition.ScoringParams {
	if scoringParams == nil {
		return scaledefinition.NewScoringParams()
	}
	return scaledefinition.NewScoringParams().
		WithCntOptionContents(scoringParams.CntOptionContents)
}

func metaCodesFromStrings(codes []string) []meta.Code {
	result := make([]meta.Code, 0, len(codes))
	for _, code := range codes {
		result = append(result, meta.NewCode(code))
	}
	return result
}
