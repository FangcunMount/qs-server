package main

import (
	"context"
	"fmt"
	"math"
	"strings"
)

const (
	desiredQuestionnaireVersion = "0.0.1" // 发布后会变为 1.0.1
	publishedVersion            = "1.0.1"
)

// seedQuestionnaires 通过 API 创建并发布问卷
func seedQuestionnaires(ctx context.Context, deps *dependencies, state *seedContext) error {
	logger := deps.Logger
	config := deps.Config
	apiClient := deps.APIClient

	if len(config.Questionnaires) == 0 {
		logger.Infow("No questionnaires to seed")
		return nil
	}

	if apiClient == nil {
		return fmt.Errorf("API client is required")
	}

	logger.Infow("Seeding questionnaires via API", "count", len(config.Questionnaires))

	for i, qc := range config.Questionnaires {
		code := qc.Code
		if code == "" {
			return fmt.Errorf("questionnaire[%d] code is empty", i)
		}
		title := pickQuestionnaireTitle(qc)
		if title == "" {
			return fmt.Errorf("questionnaire[%s] title is empty", code)
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
			Type:        "Survey", // 调查问卷类型
		}

		if existing == nil {
			logger.Debugw("Creating questionnaire", "code", code, "title", title)
			_, err := apiClient.CreateQuestionnaire(ctx, createReq)
			if err != nil {
				logger.Errorw("Create questionnaire failed", "code", code, "title", title, "error", err)
				return fmt.Errorf("create questionnaire %s failed: %w", code, err)
			}
		} else {
			logger.Debugw("Questionnaire exists, updating", "code", code, "title", title)
			_, err := apiClient.UpdateQuestionnaireBasicInfo(ctx, code, createReq)
			if err != nil {
				logger.Errorw("Update questionnaire basic info failed", "code", code, "error", err)
				return fmt.Errorf("update questionnaire %s basic info failed: %w", code, err)
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
				logger.Errorw("Update questionnaire questions failed", "code", code, "error", err)
				return fmt.Errorf("update questionnaire %s questions failed: %w", code, err)
			}
		}

		// 发布问卷：若无题目则跳过发布
		if len(questionDTOs) == 0 {
			logger.Warnw("Skip publish questionnaire with no questions", "code", code)
		} else {
			_, err := apiClient.PublishQuestionnaire(ctx, code)
			if err != nil {
				// 如果已经发布，忽略错误
				if strings.Contains(err.Error(), "already published") || strings.Contains(err.Error(), "invalid status") {
					logger.Debugw("Questionnaire already published, skipping", "code", code)
				} else {
					logger.Errorw("Publish questionnaire failed", "code", code, "error", err)
					return fmt.Errorf("publish questionnaire %s failed: %w", code, err)
				}
			}
		}

		state.QuestionnaireVersionsByCode[code] = publishedVersion
		logger.Infow("Questionnaire upserted", "code", code, "index", i+1)
	}

	logger.Infow("Questionnaires seeded successfully", "count", len(config.Questionnaires))
	return nil
}

// buildQuestionDTOsForAPI 将配置转换为 API 请求的 DTO
func buildQuestionDTOsForAPI(questions []QuestionConfig) []QuestionDTO {
	dtos := make([]QuestionDTO, 0, len(questions))
	for _, q := range questions {
		opts := make([]OptionDTO, 0, len(q.Options))
		for _, opt := range q.Options {
			score := int(math.Round(opt.Score.Float64()))
			opts = append(opts, OptionDTO{
				Label: opt.OptionContent(),
				Value: opt.Code,
				Score: score,
			})
		}

		qTypeNormalized := strings.ToLower(strings.TrimSpace(q.Type))
		if (qTypeNormalized == "radio" || qTypeNormalized == "checkbox" || qTypeNormalized == "check_box" || qTypeNormalized == "check-box") && len(opts) == 1 {
			// 自动补齐一个占位选项，防止因数据缺失无法发布
			opts = append(opts, OptionDTO{
				Label: "（自动补齐）无/不适用",
				Value: q.Code + "_auto",
				Score: 0,
			})
		}

		// 题干为空时补占位，避免发布校验失败
		stem := q.QuestionText()
		if stem == "" {
			stem = "（占位题干，需补充）" + q.Code
		}

		required := q.Required || q.ValidateRules.Required == "1"
		description := firstNonEmpty(q.Description, q.Tips)

		dtos = append(dtos, QuestionDTO{
			Code:        q.Code,
			Stem:        stem,
			Type:        pickQuestionTypeForAPI(q.Type, opts),
			Options:     opts,
			Required:    required,
			Description: description,
		})
	}
	return dtos
}

func pickQuestionnaireTitle(qc QuestionnaireConfig) string {
	if qc.Name != "" {
		return qc.Name
	}
	if qc.Title != "" {
		return qc.Title
	}
	return qc.Code
}

func pickQuestionTypeForAPI(typ string, opts []OptionDTO) string {
	if typ != "" {
		// 统一转换为小写处理，兼容大小写
		typLower := strings.ToLower(strings.TrimSpace(typ))
		switch typLower {
		case "radio":
			return "radio"
		case "scoreradio":
			return "radio"
		case "checkbox", "check_box", "check-box":
			return "checkbox"
		case "date":
			return "text"
		case "section":
			return "section"
		case "text":
			return "text"
		case "textarea":
			return "textarea"
		case "number":
			return "number"
		default:
			// 如果无法识别，尝试直接返回小写版本
			return typLower
		}
	}
	if len(opts) == 0 {
		return "section"
	}
	return "radio"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
