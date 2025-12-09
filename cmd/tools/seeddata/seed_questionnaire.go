package main

import (
	"context"
	"fmt"
	"math"

	qApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	qDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	qInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
)

const (
	desiredQuestionnaireVersion = "0.0.1" // 发布后会变为 1.0.1
	publishedVersion            = "1.0.1"
)

// seedQuestionnaires 创建并发布问卷
func seedQuestionnaires(ctx context.Context, deps *dependencies, state *seedContext) error {
	logger := deps.Logger
	config := deps.Config

	if len(config.Questionnaires) == 0 {
		logger.Infow("No questionnaires to seed")
		return nil
	}

	logger.Infow("Seeding questionnaires via application services", "count", len(config.Questionnaires))

	repo := qInfra.NewRepository(deps.MongoDB)
	contentSvc := qApp.NewContentService(repo, qDomain.QuestionManager{})
	lifecycleSvc := qApp.NewLifecycleService(repo, qDomain.Validator{}, qDomain.NewLifecycle(), nil)
	querySvc := qApp.NewQueryService(repo)

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

		// 判断是否已存在
		existing, _ := querySvc.GetByCode(ctx, code)
		if existing == nil {
			_, err := lifecycleSvc.Create(ctx, qApp.CreateQuestionnaireDTO{
				Code:        code,
				Title:       title,
				Description: qc.Description,
				ImgUrl:      qImg,
				Version:     desiredQuestionnaireVersion,
			})
			if err != nil {
				return fmt.Errorf("create questionnaire %s failed: %w", code, err)
			}
		} else {
			if _, err := lifecycleSvc.UpdateBasicInfo(ctx, qApp.UpdateQuestionnaireBasicInfoDTO{
				Code:        code,
				Title:       title,
				Description: qc.Description,
				ImgUrl:      qImg,
			}); err != nil {
				return fmt.Errorf("update questionnaire %s basic info failed: %w", code, err)
			}
		}

		questionDTOs := buildQuestionDTOs(qc.Questions)
		if len(questionDTOs) == 0 {
			logger.Warnw("Questionnaire has no questions", "code", code)
		} else {
			if _, err := contentSvc.BatchUpdateQuestions(ctx, code, questionDTOs); err != nil {
				return fmt.Errorf("update questionnaire %s questions failed: %w", code, err)
			}
		}

		// 发布问卷：先检查状态，若已发布或已归档则跳过
		latestQ, err := querySvc.GetByCode(ctx, code)
		if err != nil {
			return fmt.Errorf("get questionnaire %s for publish check failed: %w", code, err)
		}
		if latestQ.Status != "已发布" && latestQ.Status != "已归档" {
			if _, err := lifecycleSvc.Publish(ctx, code); err != nil {
				return fmt.Errorf("publish questionnaire %s failed: %w", code, err)
			}
		} else {
			logger.Debugw("Questionnaire already published/archived, skipping publish", "code", code, "status", latestQ.Status)
		}

		state.QuestionnaireIDsByCode[code] = publishedVersion
		logger.Infow("Questionnaire upserted", "code", code, "index", i+1)
	}

	logger.Infow("Questionnaires seeded successfully", "count", len(config.Questionnaires))
	return nil
}

// buildQuestionDTOs 将配置转换为应用层 DTO
func buildQuestionDTOs(questions []QuestionConfig) []qApp.QuestionDTO {
	dtos := make([]qApp.QuestionDTO, 0, len(questions))
	for _, q := range questions {
		opts := make([]qApp.OptionDTO, 0, len(q.Options))
		for _, opt := range q.Options {
			score := int(math.Round(opt.Score.Float64()))
			opts = append(opts, qApp.OptionDTO{
				Label: opt.OptionContent(),
				Value: opt.Code,
				Score: score,
			})
		}

		required := q.Required || q.ValidateRules.Required == "1"
		description := firstNonEmpty(q.Description, q.Tips)

		dtos = append(dtos, qApp.QuestionDTO{
			Code:        q.Code,
			Stem:        q.QuestionText(),
			Type:        pickQuestionType(q.Type, opts),
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

func pickQuestionType(typ string, opts []qApp.OptionDTO) string {
	if typ != "" {
		return typ
	}
	if len(opts) == 0 {
		return string(qDomain.TypeSection)
	}
	return string(qDomain.TypeRadio)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
