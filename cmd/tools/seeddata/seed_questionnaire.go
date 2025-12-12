package main

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	qApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	qDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	qInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
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
		existing, err := querySvc.GetByCode(ctx, code)
		if err != nil && !errors.IsCode(err, errorCode.ErrQuestionnaireNotFound) {
			return fmt.Errorf("query questionnaire %s failed: %w", code, err)
		}

		createQuestionnaire := func() error {
			logger.Debugw("Creating questionnaire", "code", code, "title", title)
			_, err := lifecycleSvc.Create(ctx, qApp.CreateQuestionnaireDTO{
				Code:        code,
				Title:       title,
				Description: qc.Description,
				ImgUrl:      qImg,
				Version:     desiredQuestionnaireVersion,
				Type:        string(qDomain.TypeSurvey),
			})
			return err
		}

		if existing == nil {
			if err := createQuestionnaire(); err != nil {
				// 可能是重复键或其他数据库问题，尝试查询后走更新路径
				existingAfter, _ := querySvc.GetByCode(ctx, code)
				if existingAfter == nil {
					logger.Errorw("Create questionnaire failed", "code", code, "title", title, "error", err)
					return fmt.Errorf("create questionnaire %s failed: %w", code, err)
				}
			}
		} else {
			logger.Debugw("Questionnaire exists, will update", "code", code, "title", title)
		}

		if _, err := lifecycleSvc.UpdateBasicInfo(ctx, qApp.UpdateQuestionnaireBasicInfoDTO{
			Code:        code,
			Title:       title,
			Description: qc.Description,
			ImgUrl:      qImg,
			Type:        string(qDomain.TypeSurvey),
		}); err != nil {
			logger.Errorw("Update questionnaire basic info failed", "code", code, "error", err)
			return fmt.Errorf("update questionnaire %s basic info failed: %w", code, err)
		}

		questionDTOs := buildQuestionDTOs(qc.Questions)
		if len(questionDTOs) == 0 {
			logger.Warnw("Questionnaire has no questions", "code", code)
		} else {
			if _, err := contentSvc.BatchUpdateQuestions(ctx, code, questionDTOs); err != nil {
				logger.Errorw("Update questionnaire questions failed", "code", code, "error", err)
				return fmt.Errorf("update questionnaire %s questions failed: %w", code, err)
			}
		}

		// 发布问卷：若无题目则跳过发布，避免中断整体导入
		if len(questionDTOs) == 0 {
			logger.Warnw("Skip publish questionnaire with no questions", "code", code)
		} else {
			latestQ, err := querySvc.GetByCode(ctx, code)
			if err != nil {
				logger.Errorw("Get questionnaire after update failed", "code", code, "error", err)
				return fmt.Errorf("get questionnaire %s for publish check failed: %w", code, err)
			}
			if _, err := lifecycleSvc.Publish(ctx, code); err != nil {
				if errors.IsCode(err, errorCode.ErrQuestionnaireInvalidStatus) {
					logger.Debugw("Questionnaire already published/archived, skipping publish", "code", code, "status", latestQ.Status)
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

		qTypeNormalized := strings.ToLower(strings.TrimSpace(q.Type))
		if (qTypeNormalized == "radio" || qTypeNormalized == "checkbox" || qTypeNormalized == "check_box" || qTypeNormalized == "check-box") && len(opts) == 1 {
			// 自动补齐一个占位选项，防止因数据缺失无法发布
			opts = append(opts, qApp.OptionDTO{
				Label: "（自动补齐）无/不适用",
				Value: q.Code + "_auto",
				Score: 0,
			})
		}

		// 题干为空时补占位，避免发布校验失败，同时保留原数据
		stem := q.QuestionText()
		if stem == "" {
			stem = "（占位题干，需补充）" + q.Code
		}

		required := q.Required || q.ValidateRules.Required == "1"
		description := firstNonEmpty(q.Description, q.Tips)

		dtos = append(dtos, qApp.QuestionDTO{
			Code:        q.Code,
			Stem:        stem,
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
		switch strings.ToLower(strings.TrimSpace(typ)) {
		case "radio":
			return string(qDomain.TypeRadio)
		case "scoreradio": // 源数据的计分单选映射为普通单选
			return string(qDomain.TypeRadio)
		case "checkbox", "check_box", "check-box":
			return string(qDomain.TypeCheckbox)
		case "date":
			// 源数据中的日期类型映射为文本输入
			return string(qDomain.TypeText)
		case "section":
			return string(qDomain.TypeSection)
		case "text":
			return string(qDomain.TypeText)
		case "textarea":
			return string(qDomain.TypeTextarea)
		case "number":
			return string(qDomain.TypeNumber)
		default:
			return typ
		}
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
