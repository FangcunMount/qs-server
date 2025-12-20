package main

import (
	"context"
	"fmt"
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
			// 验证问题数据
			questionCodes := make(map[string]bool)
			for i, q := range questionDTOs {
				if q.Code == "" {
					logger.Errorw("Question code is empty", "index", i, "questionnaire", code)
					return fmt.Errorf("questionnaire %s: question[%d] code is empty", code, i)
				}
				if questionCodes[q.Code] {
					logger.Errorw("Duplicate question code", "code", q.Code, "questionnaire", code)
					return fmt.Errorf("questionnaire %s: duplicate question code %s", code, q.Code)
				}
				questionCodes[q.Code] = true

				// 验证选择题的选项
				if q.Type == "radio" || q.Type == "checkbox" {
					if len(q.Options) < 2 {
						logger.Errorw("Question has insufficient options", "code", q.Code, "type", q.Type, "options_count", len(q.Options))
						return fmt.Errorf("questionnaire %s: question %s (%s) has only %d options, need at least 2", code, q.Code, q.Type, len(q.Options))
					}
					optionCodes := make(map[string]bool)
					for j, opt := range q.Options {
						if opt.Code == "" {
							logger.Errorw("Option code is empty", "question", q.Code, "option_index", j)
							return fmt.Errorf("questionnaire %s: question %s option[%d] code is empty", code, q.Code, j)
						}
						if opt.Content == "" {
							logger.Errorw("Option content is empty", "question", q.Code, "option_index", j, "code", opt.Code)
							return fmt.Errorf("questionnaire %s: question %s option[%d] content is empty", code, q.Code, j)
						}
						if optionCodes[opt.Code] {
							logger.Errorw("Duplicate option code", "question", q.Code, "code", opt.Code)
							return fmt.Errorf("questionnaire %s: question %s has duplicate option code %s", code, q.Code, opt.Code)
						}
						optionCodes[opt.Code] = true
					}
				}
			}

			logger.Debugw("Updating questionnaire questions", "code", code, "questions_count", len(questionDTOs))
			batchReq := BatchUpdateQuestionsRequest{
				Questions: questionDTOs,
			}
			if err := apiClient.BatchUpdateQuestions(ctx, code, batchReq); err != nil {
				logger.Errorw("Update questionnaire questions failed", "code", code, "error", err, "questions_count", len(questionDTOs))
				return fmt.Errorf("update questionnaire %s questions failed: %w", code, err)
			}
		}

		// 发布问卷：若无题目则跳过发布
		if len(questionDTOs) == 0 {
			logger.Warnw("Skip publish questionnaire with no questions", "code", code)
		} else {
			// 先确保问卷是草稿状态
			latestQ, err := apiClient.GetQuestionnaire(ctx, code)
			if err != nil {
				logger.Errorw("Get questionnaire before publish failed", "code", code, "error", err)
				return fmt.Errorf("get questionnaire %s for publish check failed: %w", code, err)
			}

			// 如果问卷是已发布状态，先下架（变为草稿）
			if latestQ.Status == "已发布" {
				logger.Debugw("Questionnaire is published, unpublishing first", "code", code)
				_, err := apiClient.UnpublishQuestionnaire(ctx, code)
				if err != nil {
					logger.Errorw("Unpublish questionnaire failed", "code", code, "error", err)
					return fmt.Errorf("unpublish questionnaire %s failed: %w", code, err)
				}
			}

			// 保存草稿（确保状态为草稿）
			if latestQ.Status != "草稿" {
				logger.Debugw("Saving questionnaire as draft", "code", code)
				_, err := apiClient.SaveDraftQuestionnaire(ctx, code)
				if err != nil {
					// 如果已经是草稿状态，SaveDraft 可能会失败，忽略错误
					if !strings.Contains(err.Error(), "只能保存草稿状态的问卷") {
						logger.Warnw("Save draft failed, continuing", "code", code, "error", err)
					}
				}
			}

			// 发布问卷
			_, err = apiClient.PublishQuestionnaire(ctx, code)
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
		qTypeNormalized := strings.ToLower(strings.TrimSpace(q.Type))

		// 构建选项列表
		opts := make([]OptionDTO, 0, len(q.Options))

		// Section 类型不应该有选项
		if qTypeNormalized != "section" {
			for _, opt := range q.Options {
				// 跳过空的选项（code 或 content 为空）
				optContent := opt.OptionContent()
				optCode := opt.Code
				if optCode == "" || optContent == "" {
					continue
				}

				score := opt.Score.Float64() // API 期望 float64
				opts = append(opts, OptionDTO{
					Code:    optCode,
					Content: optContent,
					Score:   score,
				})
			}

			// 选择题类型必须至少 2 个选项
			if qTypeNormalized == "radio" || qTypeNormalized == "checkbox" || qTypeNormalized == "check_box" || qTypeNormalized == "check-box" {
				if len(opts) == 0 {
					// 如果没有选项，添加两个占位选项
					opts = append(opts, OptionDTO{
						Code:    q.Code + "_opt1",
						Content: "（占位选项1）",
						Score:   0,
					})
					opts = append(opts, OptionDTO{
						Code:    q.Code + "_opt2",
						Content: "（占位选项2）",
						Score:   0,
					})
				} else if len(opts) == 1 {
					// 如果只有 1 个选项，补齐第二个
					opts = append(opts, OptionDTO{
						Code:    q.Code + "_auto",
						Content: "（自动补齐）无/不适用",
						Score:   0,
					})
				}
			}
		}

		// 题干为空时补占位，避免发布校验失败
		stem := q.QuestionText()
		if stem == "" {
			stem = "（占位题干，需补充）" + q.Code
		}

		tips := firstNonEmpty(q.Description, q.Tips)

		dtos = append(dtos, QuestionDTO{
			Code:    q.Code,
			Type:    pickQuestionTypeForAPI(q.Type, opts),
			Stem:    stem,
			Tips:    tips,
			Options: opts,
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
		case "radio", "scoreradio":
			return "Radio" // API 期望首字母大写
		case "checkbox", "check_box", "check-box":
			return "Checkbox" // API 期望首字母大写
		case "date":
			return "Text" // 日期类型映射为文本
		case "section":
			return "Section" // API 期望首字母大写
		case "text":
			return "Text" // API 期望首字母大写
		case "textarea":
			return "Textarea" // API 期望首字母大写
		case "number":
			return "Number" // API 期望首字母大写
		default:
			// 如果原值已经是首字母大写，直接返回；否则尝试首字母大写
			if len(typ) > 0 && typ[0] >= 'A' && typ[0] <= 'Z' {
				return typ
			}
			// 首字母大写
			if len(typLower) > 0 {
				return strings.ToUpper(typLower[:1]) + typLower[1:]
			}
			return typ
		}
	}
	if len(opts) == 0 {
		return "Section"
	}
	return "Radio"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
