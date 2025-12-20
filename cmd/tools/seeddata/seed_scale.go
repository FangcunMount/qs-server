package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
)

// seedScales 通过 API 创建完整的医学量表（问卷 + 因子）并发布
func seedScales(ctx context.Context, deps *dependencies, state *seedContext) error {
	logger := deps.Logger
	config := deps.Config
	apiClient := deps.APIClient

	if len(config.Scales) == 0 {
		logger.Infow("No scales to seed")
		return nil
	}

	if apiClient == nil {
		return fmt.Errorf("API client is required")
	}

	logger.Infow("Seeding scales via API", "count", len(config.Scales))

	// 初始化分类映射器
	categoryMapper := NewScaleCategoryMapper()

	for i, sc := range config.Scales {
		scaleCode := sc.Code
		if scaleCode == "" {
			return fmt.Errorf("scale[%d] code is empty", i)
		}

		qCode := sc.QuestionnaireCode
		if qCode == "" {
			qCode = scaleCode
		}

		scaleTitle := firstNonEmpty(sc.Title, sc.Name)
		if scaleTitle == "" {
			scaleTitle = scaleCode
		}

		// 1. 创建或更新问卷
		qc := QuestionnaireConfig{
			Code:        qCode,
			Name:        scaleTitle,
			Description: sc.Description,
			ImgUrl:      sc.Icon,
			Version:     sc.QuestionnaireVersion,
			Questions:   sc.Questions,
		}

		qVersion, err := ensureQuestionnaireViaAPI(ctx, apiClient, qc, logger)
		if err != nil {
			return fmt.Errorf("scale[%s] questionnaire upsert failed: %w", scaleCode, err)
		}

		// 2. 获取量表分类信息
		categoryInfo := categoryMapper.MapScaleCategory(scaleTitle)

		// 3. 创建或更新量表
		existingScale, err := apiClient.GetScale(ctx, scaleCode)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			logger.Warnw("Failed to check existing scale", "code", scaleCode, "error", err)
		}

		createScaleReq := CreateScaleRequest{
			Title:                scaleTitle,
			Description:          sc.Description,
			Category:             categoryInfo.Category,
			Stages:               categoryInfo.Stages,
			ApplicableAges:       categoryInfo.ApplicableAges,
			Reporters:            categoryInfo.Reporters,
			Tags:                 categoryInfo.Tags,
			QuestionnaireCode:    qCode,
			QuestionnaireVersion: qVersion,
		}

		if existingScale == nil {
			logger.Debugw("Creating scale", "code", scaleCode, "title", scaleTitle)
			_, err := apiClient.CreateScale(ctx, createScaleReq)
			if err != nil {
				return fmt.Errorf("create scale %s failed: %w", scaleCode, err)
			}
		} else {
			logger.Debugw("Scale exists, updating", "code", scaleCode, "title", scaleTitle)
			// 更新基本信息
			_, err := apiClient.UpdateScaleBasicInfo(ctx, scaleCode, createScaleReq)
			if err != nil {
				return fmt.Errorf("update scale %s basic info failed: %w", scaleCode, err)
			}
			// 更新关联问卷
			_, err = apiClient.UpdateScaleQuestionnaire(ctx, scaleCode, qCode, qVersion)
			if err != nil {
				return fmt.Errorf("update scale %s questionnaire failed: %w", scaleCode, err)
			}
		}

		// 4. 批量更新因子
		factorDTOs := buildFactorDTOsForAPI(sc, logger)
		if len(factorDTOs) == 0 {
			logger.Warnw("Scale has no factors", "code", scaleCode)
		} else {
			batchReq := BatchUpdateFactorsRequest{
				Factors: factorDTOs,
			}
			if err := apiClient.BatchUpdateFactors(ctx, scaleCode, batchReq); err != nil {
				return fmt.Errorf("update scale %s factors failed: %w", scaleCode, err)
			}
		}

		// 5. 发布量表
		latestScale, err := apiClient.GetScale(ctx, scaleCode)
		if err != nil {
			return fmt.Errorf("get scale %s for publish check failed: %w", scaleCode, err)
		}
		if latestScale.Status != "已发布" && latestScale.Status != "已归档" {
			_, err := apiClient.PublishScale(ctx, scaleCode)
			if err != nil {
				if strings.Contains(err.Error(), "already published") || strings.Contains(err.Error(), "invalid status") {
					logger.Debugw("Scale already published/archived, skipping publish", "code", scaleCode, "status", latestScale.Status)
				} else {
					return fmt.Errorf("publish scale %s failed: %w", scaleCode, err)
				}
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

// ensureQuestionnaireViaAPI 通过 API 确保问卷存在并发布，返回发布后的版本号
func ensureQuestionnaireViaAPI(
	ctx context.Context,
	apiClient *APIClient,
	qc QuestionnaireConfig,
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

	// 检查是否已存在
	existing, err := apiClient.GetQuestionnaire(ctx, code)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		logger.Warnw("Failed to check existing questionnaire", "code", code, "error", err)
	}

	createReq := CreateQuestionnaireRequest{
		Title:       title,
		Description: qc.Description,
		ImgUrl:      qImg,
		Type:        "MedicalScale", // 医学量表类型
	}

	if existing == nil {
		logger.Debugw("Creating questionnaire", "code", code, "title", title)
		_, err := apiClient.CreateQuestionnaire(ctx, createReq)
		if err != nil {
			return "", fmt.Errorf("create questionnaire %s failed: %w", code, err)
		}
	} else {
		logger.Debugw("Questionnaire exists, updating", "code", code, "title", title)
		_, err := apiClient.UpdateQuestionnaireBasicInfo(ctx, code, createReq)
		if err != nil {
			return "", fmt.Errorf("update questionnaire %s basic info failed: %w", code, err)
		}
	}

	// 批量更新问题
	questionDTOs := buildQuestionDTOsForAPI(qc.Questions)
	if len(questionDTOs) == 0 {
		logger.Warnw("Questionnaire has no questions", "code", code)
	} else {
		batchReq := BatchUpdateQuestionsRequest{
			Questions: questionDTOs,
		}
		if err := apiClient.BatchUpdateQuestions(ctx, code, batchReq); err != nil {
			return "", fmt.Errorf("update questionnaire %s questions failed: %w", code, err)
		}
	}

	// 发布问卷
	if len(questionDTOs) == 0 {
		logger.Warnw("Skip publish questionnaire with no questions", "code", code)
	} else {
		_, err := apiClient.PublishQuestionnaire(ctx, code)
		if err != nil {
			if strings.Contains(err.Error(), "already published") || strings.Contains(err.Error(), "invalid status") {
				logger.Debugw("Questionnaire already published, skipping publish", "code", code)
			} else {
				return "", fmt.Errorf("publish questionnaire %s failed: %w", code, err)
			}
		}
	}

	return publishedVersion, nil
}

// buildFactorDTOsForAPI 将配置转换为 API 请求的因子 DTO
func buildFactorDTOsForAPI(sc ScaleConfig, logger log.Logger) []FactorDTO {
	dtos := make([]FactorDTO, 0, len(sc.Factors))
	groupInterp := mergeInterpretationGroup(sc.Interpretation)
	hasTotal := false

	for _, f := range sc.Factors {
		isTotal := f.IsTotalScore == "1"
		if isTotal {
			hasTotal = true
		}
		factorGroup := mergeInterpretationGroupWithFallback(f.InterpretRule, f.Interpretations)
		interpretRules := toInterpretRulesForAPI(factorGroup, groupInterp, logger)

		scoringStrategy := "sum"
		if f.CalcRule.Formula == "avg" {
			scoringStrategy = "avg"
		} else if f.CalcRule.Formula == "cnt" {
			scoringStrategy = "cnt"
		} else if f.CalcRule.Formula != "" && f.CalcRule.Formula != "sum" {
			logger.Warnw("Unknown scoring formula, using sum as fallback",
				"scale_code", sc.Code,
				"factor_code", f.Code,
				"formula", f.CalcRule.Formula)
		}

		// 构建 ScoringParamsDTO
		var scoringParams *ScoringParamsDTO
		if scoringStrategy == "cnt" {
			cntOptionContents := make([]string, 0)
			if f.CalcRule.AppendParams != nil {
				if contents, ok := f.CalcRule.AppendParams["cnt_option_contents"]; ok {
					if contentsArray, ok := contents.([]interface{}); ok {
						for _, item := range contentsArray {
							if str, ok := item.(string); ok {
								cntOptionContents = append(cntOptionContents, str)
							}
						}
					} else if contentsArray, ok := contents.([]string); ok {
						cntOptionContents = contentsArray
					}
				}
			}
			scoringParams = &ScoringParamsDTO{
				CntOptionContents: cntOptionContents,
			}
		}

		dtos = append(dtos, FactorDTO{
			Code:            f.Code,
			Title:           firstNonEmpty(f.Title, f.Name, f.Description),
			FactorType:      "primary",
			IsTotalScore:    isTotal,
			QuestionCodes:   pickQuestionCodes(f),
			ScoringStrategy: scoringStrategy,
			ScoringParams:   scoringParams,
			InterpretRules:  interpretRules,
		})
	}

	// 若缺少总分因子，自动补充一个占位总分因子
	if !hasTotal {
		autoCode := sc.Code + "_total_auto"
		dtos = append(dtos, FactorDTO{
			Code:            autoCode,
			Title:           "总分(自动补齐)",
			FactorType:      "primary",
			IsTotalScore:    true,
			QuestionCodes:   collectQuestionCodes(sc),
			ScoringStrategy: "sum",
			ScoringParams:   nil,
			InterpretRules: []InterpretRuleDTO{
				{MinScore: 0, MaxScore: 9999, RiskLevel: "none", Conclusion: "暂无解读", Suggestion: ""},
			},
		})
		logger.Warnw("Added auto total factor", "scale", sc.Code, "factor", autoCode)
	}
	return dtos
}

// pickQuestionCodes 返回因子关联的题目编码
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

// toInterpretRulesForAPI 将配置转换为 API 请求的解读规则 DTO
func toInterpretRulesForAPI(factorGroup InterpretationGroupConfig, scaleGroup InterpretationGroupConfig, logger log.Logger) []InterpretRuleDTO {
	items := factorGroup.Items
	if len(items) == 0 {
		items = scaleGroup.Items
	}

	rules := make([]InterpretRuleDTO, 0, len(items))
	for _, interp := range items {
		min := parseFloat(interp.MinScore, interp.Start)
		max := parseFloat(interp.MaxScore, interp.End)
		if max <= min {
			max = min + 0.0001
		}
		text := firstNonEmpty(interp.Description, interp.Content)

		// 解析风险等级
		riskLevel := parseRiskLevel(interp.RiskLevel, interp.Level)

		rules = append(rules, InterpretRuleDTO{
			MinScore:   min,
			MaxScore:   max,
			RiskLevel:  riskLevel,
			Conclusion: text,
			Suggestion: text,
		})
	}
	if len(rules) == 0 {
		logger.Warnw("Interpretation rules missing, inserting default placeholder")
		rules = append(rules, InterpretRuleDTO{
			MinScore:   0,
			MaxScore:   9999,
			RiskLevel:  "none",
			Conclusion: "暂无解读",
			Suggestion: "",
		})
	}
	return rules
}

// parseRiskLevel 解析风险等级
func parseRiskLevel(riskLevel, level string) string {
	if riskLevel != "" {
		normalized := normalizeRiskLevel(riskLevel)
		if normalized != "" {
			return normalized
		}
	}

	if level != "" {
		normalized := normalizeRiskLevel(level)
		if normalized != "" {
			return normalized
		}
	}

	return "none"
}

// normalizeRiskLevel 规范化风险等级字符串
func normalizeRiskLevel(raw string) string {
	switch raw {
	case "none", "正常", "无风险":
		return "none"
	case "low", "轻度", "低风险":
		return "low"
	case "medium", "中度", "中风险":
		return "medium"
	case "high", "重度", "高风险":
		return "high"
	case "severe", "严重", "极高风险":
		return "severe"
	default:
		return ""
	}
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
