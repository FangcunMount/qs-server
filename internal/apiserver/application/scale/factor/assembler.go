package factor

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
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
) (*domscale.Factor, error) {
	strategy := domscale.ScoringStrategySum
	if scoringStrategy != "" {
		strategy = domscale.ScoringStrategyCode(scoringStrategy)
	}

	fType := domscale.FactorTypePrimary
	if factorType != "" {
		fType = domscale.FactorType(factorType)
	}

	scoringParamsDomain := scoringParamsFromDTO(scoringParams)
	factor, err := domscale.NewFactor(
		domscale.NewFactorCode(code),
		title,
		domscale.WithFactorType(fType),
		domscale.WithIsTotalScore(isTotalScore),
		domscale.WithIsShow(isShow),
		domscale.WithQuestionCodes(metaCodesFromStrings(questionCodes)),
		domscale.WithScoringStrategy(strategy),
		domscale.WithScoringParams(scoringParamsDomain),
		domscale.WithMaxScore(maxScore),
		domscale.WithInterpretRules(shared.InterpretRulesFromDTOs(interpretRules)),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建因子失败")
	}
	return factor, nil
}

func scoringParamsFromDTO(scoringParams *shared.ScoringParamsDTO) *domscale.ScoringParams {
	if scoringParams == nil {
		return domscale.NewScoringParams()
	}
	return domscale.NewScoringParams().
		WithCntOptionContents(scoringParams.CntOptionContents)
}

func metaCodesFromStrings(codes []string) []meta.Code {
	result := make([]meta.Code, 0, len(codes))
	for _, code := range codes {
		result = append(result, meta.NewCode(code))
	}
	return result
}
