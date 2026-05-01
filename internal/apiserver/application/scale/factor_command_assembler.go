package scale

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func toFactorDomain(
	code, title, factorType string,
	isTotalScore, isShow bool,
	questionCodes []string,
	scoringStrategy string,
	scoringParams *ScoringParamsDTO,
	maxScore *float64,
	interpretRules []InterpretRuleDTO,
) (*domainScale.Factor, error) {
	strategy := domainScale.ScoringStrategySum
	if scoringStrategy != "" {
		strategy = domainScale.ScoringStrategyCode(scoringStrategy)
	}

	fType := domainScale.FactorTypePrimary
	if factorType != "" {
		fType = domainScale.FactorType(factorType)
	}

	scoringParamsDomain := scoringParamsFromDTO(scoringParams)
	if strategy == domainScale.ScoringStrategyCnt && len(scoringParamsDomain.GetCntOptionContents()) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "cnt 计分策略必须提供 cnt_option_contents 参数")
	}

	factor, err := domainScale.NewFactor(
		domainScale.NewFactorCode(code),
		title,
		domainScale.WithFactorType(fType),
		domainScale.WithIsTotalScore(isTotalScore),
		domainScale.WithIsShow(isShow),
		domainScale.WithQuestionCodes(metaCodesFromStrings(questionCodes)),
		domainScale.WithScoringStrategy(strategy),
		domainScale.WithScoringParams(scoringParamsDomain),
		domainScale.WithMaxScore(maxScore),
		domainScale.WithInterpretRules(interpretRulesFromDTOs(interpretRules)),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建因子失败")
	}
	return factor, nil
}

func scoringParamsFromDTO(scoringParams *ScoringParamsDTO) *domainScale.ScoringParams {
	if scoringParams == nil {
		return domainScale.NewScoringParams()
	}
	return domainScale.NewScoringParams().
		WithCntOptionContents(scoringParams.CntOptionContents)
}

func metaCodesFromStrings(codes []string) []meta.Code {
	result := make([]meta.Code, 0, len(codes))
	for _, code := range codes {
		result = append(result, meta.NewCode(code))
	}
	return result
}
