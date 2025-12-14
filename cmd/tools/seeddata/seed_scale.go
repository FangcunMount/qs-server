package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/log"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	qApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	scaleDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	qDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	qInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// seedScales 创建完整的医学量表（问卷 + 因子）并发布
func seedScales(ctx context.Context, deps *dependencies, state *seedContext) error {
	logger := deps.Logger
	config := deps.Config

	if len(config.Scales) == 0 {
		logger.Infow("No scales to seed")
		return nil
	}

	logger.Infow("Seeding scales via application services", "count", len(config.Scales))

	// 问卷服务（复用问卷逻辑）
	qRepo := qInfra.NewRepository(deps.MongoDB)
	qContentSvc := qApp.NewContentService(qRepo, qDomain.QuestionManager{})
	qLifecycleSvc := qApp.NewLifecycleService(qRepo, qDomain.Validator{}, qDomain.NewLifecycle(), nil)
	qQuerySvc := qApp.NewQueryService(qRepo)

	// 量表服务
	scaleRepo := scaleInfra.NewRepository(deps.MongoDB)
	scaleLifecycle := scaleApp.NewLifecycleService(scaleRepo, nil)
	factorSvc := scaleApp.NewFactorService(scaleRepo)
	scaleQuery := scaleApp.NewQueryService(scaleRepo)

	for i, sc := range config.Scales {
		scaleCode := sc.Code
		if scaleCode == "" {
			return fmt.Errorf("scale[%d] code is empty", i)
		}

		qCode := sc.QuestionnaireCode
		if qCode == "" {
			qCode = scaleCode
		}

		qc := QuestionnaireConfig{
			Code:        qCode,
			Name:        firstNonEmpty(sc.Name, sc.Title),
			Description: sc.Description,
			ImgUrl:      sc.Icon,
			Version:     sc.QuestionnaireVersion,
			Questions:   sc.Questions,
		}

		qVersion, err := ensureQuestionnaire(ctx, qc, qContentSvc, qLifecycleSvc, qQuerySvc, logger)
		if err != nil {
			return fmt.Errorf("scale[%s] questionnaire upsert failed: %w", scaleCode, err)
		}

		scaleTitle := firstNonEmpty(sc.Title, sc.Name)
		if scaleTitle == "" {
			scaleTitle = qCode
		}

		existing, _ := scaleQuery.GetByCode(ctx, scaleCode)
		if existing == nil {
			if _, err := scaleLifecycle.Create(ctx, scaleApp.CreateScaleDTO{
				Code:                 scaleCode,
				Title:                scaleTitle,
				Description:          sc.Description,
				QuestionnaireCode:    qCode,
				QuestionnaireVersion: qVersion,
			}); err != nil {
				return fmt.Errorf("create scale %s failed: %w", scaleCode, err)
			}
		} else {
			if _, err := scaleLifecycle.UpdateBasicInfo(ctx, scaleApp.UpdateScaleBasicInfoDTO{
				Code:        scaleCode,
				Title:       scaleTitle,
				Description: sc.Description,
			}); err != nil {
				return fmt.Errorf("update scale %s basic info failed: %w", scaleCode, err)
			}
			if _, err := scaleLifecycle.UpdateQuestionnaire(ctx, scaleApp.UpdateScaleQuestionnaireDTO{
				Code:                 scaleCode,
				QuestionnaireCode:    qCode,
				QuestionnaireVersion: qVersion,
			}); err != nil {
				return fmt.Errorf("update scale %s questionnaire failed: %w", scaleCode, err)
			}
		}

		factorDTOs := buildFactorDTOs(sc, logger)
		if len(factorDTOs) == 0 {
			logger.Warnw("Scale has no factors", "code", scaleCode)
		} else {
			if _, err := factorSvc.ReplaceFactors(ctx, scaleCode, factorDTOs); err != nil {
				return fmt.Errorf("update scale %s factors failed: %w", scaleCode, err)
			}
		}

		// 发布量表：先检查状态，若已发布或已归档则跳过
		latestScale, err := scaleQuery.GetByCode(ctx, scaleCode)
		if err != nil {
			return fmt.Errorf("get scale %s for publish check failed: %w", scaleCode, err)
		}
		if latestScale.Status != "已发布" && latestScale.Status != "已归档" {
			if _, err := scaleLifecycle.Publish(ctx, scaleCode); err != nil {
				return fmt.Errorf("publish scale %s failed: %w", scaleCode, err)
			}
		} else {
			logger.Debugw("Scale already published/archived, skipping publish", "code", scaleCode, "status", latestScale.Status)
		}

		state.ScaleIDsByCode[scaleCode] = scaleCode
		logger.Infow("Scale upserted", "code", scaleCode, "questionnaire", qCode, "version", qVersion, "index", i+1)
	}

	logger.Infow("Scales seeded successfully", "count", len(config.Scales))
	return nil
}

// ensureQuestionnaire 复用问卷种子逻辑，返回发布后的版本号
func ensureQuestionnaire(
	ctx context.Context,
	qc QuestionnaireConfig,
	contentSvc qApp.QuestionnaireContentService,
	lifecycleSvc qApp.QuestionnaireLifecycleService,
	querySvc qApp.QuestionnaireQueryService,
	logger log.Logger,
) (string, error) {
	code := qc.Code
	if code == "" {
		return "", fmt.Errorf("questionnaire code is empty")
	}
	title := pickQuestionnaireTitle(qc)
	if title == "" {
		return "", fmt.Errorf("questionnaire[%s] title is empty", code)
	}
	qImg := firstNonEmpty(qc.ImgUrl, qc.Icon)
	version := qc.Version
	if version == "" {
		version = desiredQuestionnaireVersion
	}

	existing, _ := querySvc.GetByCode(ctx, code)
	if existing == nil {
		if _, err := lifecycleSvc.Create(ctx, qApp.CreateQuestionnaireDTO{
			Code:        code,
			Title:       title,
			Description: qc.Description,
			ImgUrl:      qImg,
			Version:     version,
			Type:        string(qDomain.TypeMedicalScale),
		}); err != nil {
			return "", fmt.Errorf("create questionnaire %s failed: %w", code, err)
		}
	} else {
		if _, err := lifecycleSvc.UpdateBasicInfo(ctx, qApp.UpdateQuestionnaireBasicInfoDTO{
			Code:        code,
			Title:       title,
			Description: qc.Description,
			ImgUrl:      qImg,
			Type:        string(qDomain.TypeMedicalScale),
		}); err != nil {
			return "", fmt.Errorf("update questionnaire %s basic info failed: %w", code, err)
		}
	}

	questionDTOs := buildQuestionDTOs(qc.Questions)
	if len(questionDTOs) == 0 {
		logger.Warnw("Questionnaire has no questions", "code", code)
	} else {
		if _, err := contentSvc.BatchUpdateQuestions(ctx, code, questionDTOs); err != nil {
			return "", fmt.Errorf("update questionnaire %s questions failed: %w", code, err)
		}
	}

	// 发布问卷：先检查状态，若已发布或已归档则跳过
	latestQ, err := querySvc.GetByCode(ctx, code)
	if err != nil {
		return "", fmt.Errorf("get questionnaire %s for publish check failed: %w", code, err)
	}
	if len(questionDTOs) == 0 {
		logger.Warnw("Skip publish questionnaire with no questions", "code", code)
	} else {
		if _, err := lifecycleSvc.Publish(ctx, code); err != nil {
			if errors.IsCode(err, errorCode.ErrQuestionnaireInvalidStatus) {
				logger.Debugw("Questionnaire already published/archived, skipping publish", "code", code, "status", latestQ.Status)
			} else {
				return "", fmt.Errorf("publish questionnaire %s failed: %w", code, err)
			}
		}
	}

	return publishedVersion, nil
}

// buildFactorDTOs 将配置转换为因子 DTO，尽量保留原始配置
func buildFactorDTOs(sc ScaleConfig, logger log.Logger) []scaleApp.FactorDTO {
	dtos := make([]scaleApp.FactorDTO, 0, len(sc.Factors))
	groupInterp := mergeInterpretationGroup(sc.Interpretation)
	hasTotal := false

	for _, f := range sc.Factors {
		isTotal := f.IsTotalScore == "1"
		if isTotal {
			hasTotal = true
		}
		factorGroup := mergeInterpretationGroupWithFallback(f.InterpretRule, f.Interpretations)
		interpretRules := toInterpretRules(factorGroup, groupInterp, logger)

		scoringStrategy := scaleDomain.ScoringStrategySum
		if f.CalcRule.Formula == "avg" {
			scoringStrategy = scaleDomain.ScoringStrategyAvg
		} else if f.CalcRule.Formula == "cnt" {
			scoringStrategy = scaleDomain.ScoringStrategyCnt
		} else if f.CalcRule.Formula != "" && f.CalcRule.Formula != "sum" {
			// 其他未知公式，记录警告并使用默认 sum 策略
			logger.Warnw("Unknown scoring formula, using sum as fallback",
				"scale_code", sc.Code,
				"factor_code", f.Code,
				"formula", f.CalcRule.Formula)
		}

		rawCalc, _ := json.Marshal(f.CalcRule)
		scoringParams := map[string]string{
			"raw_calc_rule": string(rawCalc),
		}
		if f.Type != "" {
			scoringParams["raw_factor_type"] = f.Type
		}

		dtos = append(dtos, scaleApp.FactorDTO{
			Code:            f.Code,
			Title:           firstNonEmpty(f.Title, f.Name, f.Description),
			FactorType:      string(scaleDomain.FactorTypePrimary),
			IsTotalScore:    isTotal,
			QuestionCodes:   pickQuestionCodes(f),
			ScoringStrategy: string(scoringStrategy),
			ScoringParams:   scoringParams,
			InterpretRules:  interpretRules,
		})
	}

	// 若缺少总分因子，自动补充一个占位总分因子，避免发布校验失败
	if !hasTotal {
		autoCode := sc.Code + "_total_auto"
		dtos = append(dtos, scaleApp.FactorDTO{
			Code:            autoCode,
			Title:           "总分(自动补齐)",
			FactorType:      string(scaleDomain.FactorTypePrimary),
			IsTotalScore:    true,
			QuestionCodes:   collectQuestionCodes(sc),
			ScoringStrategy: string(scaleDomain.ScoringStrategySum),
			ScoringParams:   map[string]string{"auto": "true"},
			InterpretRules: []scaleApp.InterpretRuleDTO{
				{MinScore: 0, MaxScore: 9999, RiskLevel: string(scaleDomain.RiskLevelNone), Conclusion: "暂无解读", Suggestion: ""},
			},
		})
		logger.Warnw("Added auto total factor", "scale", sc.Code, "factor", autoCode)
	}
	return dtos
}

// pickQuestionCodes 返回因子关联的题目编码（兼容旧字段）
func pickQuestionCodes(f FactorConfig) []string {
	if len(f.QuestionCodes) > 0 {
		return f.QuestionCodes
	}
	if len(f.SourceCodes) > 0 {
		return f.SourceCodes
	}
	return []string{}
}

// mergeInterpretationGroup 归并不同命名的解读配置
func mergeInterpretationGroup(group InterpretationGroupConfig) InterpretationGroupConfig {
	if len(group.Items) == 0 && len(group.Interpretation) > 0 {
		group.Items = group.Interpretation
	}
	return group
}

// mergeInterpretationGroupWithFallback 兼容老的 interpretations 数组
func mergeInterpretationGroupWithFallback(group InterpretationGroupConfig, fallback []InterpretationConfig) InterpretationGroupConfig {
	group = mergeInterpretationGroup(group)
	if len(group.Items) == 0 && len(fallback) > 0 {
		group.Items = fallback
	}
	return group
}

// toInterpretRules 将配置转换为应用层 DTO
func toInterpretRules(factorGroup InterpretationGroupConfig, scaleGroup InterpretationGroupConfig, logger log.Logger) []scaleApp.InterpretRuleDTO {
	items := factorGroup.Items
	if len(items) == 0 {
		items = scaleGroup.Items
	}

	rules := make([]scaleApp.InterpretRuleDTO, 0, len(items))
	for _, interp := range items {
		min := parseFloat(interp.MinScore, interp.Start)
		max := parseFloat(interp.MaxScore, interp.End)
		if max <= min {
			max = min + 0.0001
		}
		text := firstNonEmpty(interp.Description, interp.Content)
		rules = append(rules, scaleApp.InterpretRuleDTO{
			MinScore:   min,
			MaxScore:   max,
			RiskLevel:  string(scaleDomain.RiskLevelNone),
			Conclusion: text,
			Suggestion: text,
		})
	}
	if len(rules) == 0 {
		logger.Warnw("Interpretation rules missing, inserting default placeholder")
		rules = append(rules, scaleApp.InterpretRuleDTO{
			MinScore:   0,
			MaxScore:   9999,
			RiskLevel:  string(scaleDomain.RiskLevelNone),
			Conclusion: "暂无解读",
			Suggestion: "",
		})
	}
	return rules
}

func parseFloat(ptr *float64, raw string) float64 {
	if ptr != nil {
		return *ptr
	}
	if raw == "" {
		return 0
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return val
}

// collectQuestionCodes 收集量表题目编码，用于自动补齐总分因子
func collectQuestionCodes(sc ScaleConfig) []string {
	codes := make([]string, 0, len(sc.Questions))
	for _, q := range sc.Questions {
		if q.Code != "" {
			codes = append(codes, q.Code)
		}
	}
	return codes
}
