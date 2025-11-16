package questionnaire

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/domain/questionnaire"
)

// GetQuestionnaireRequest 获取问卷请求
type GetQuestionnaireRequest struct {
	Code string `json:"code" validate:"required"`
}

// GetQuestionnaireResponse 获取问卷响应
type GetQuestionnaireResponse struct {
	Code        string         `json:"code"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Questions   []*QuestionDTO `json:"questions"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// QuestionDTO 问题数据传输对象
type QuestionDTO struct {
	Code            string               `json:"code"`
	Title           string               `json:"title"`
	Type            string               `json:"type"`
	Tips            string               `json:"tips"`
	Placeholder     string               `json:"placeholder"`
	Options         []*QuestionOptionDTO `json:"options,omitempty"`
	ValidationRules []*ValidationRuleDTO `json:"validation_rules,omitempty"`
}

// QuestionOptionDTO 问题选项数据传输对象
type QuestionOptionDTO struct {
	Code    string `json:"code"`
	Content string `json:"content"`
	Score   int32  `json:"score"`
}

// ValidationRuleDTO 验证规则数据传输对象
type ValidationRuleDTO struct {
	RuleType    string `json:"rule_type"`
	TargetValue string `json:"target_value"`
	Message     string `json:"message"`
}

// ValidateQuestionnaireCodeRequest 验证问卷代码请求
type ValidateQuestionnaireCodeRequest struct {
	Code string `json:"code" validate:"required"`
}

// ValidateQuestionnaireCodeResponse 验证问卷代码响应
type ValidateQuestionnaireCodeResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// ToDTO 将领域实体转换为 DTO
func ToDTO(entity *questionnaire.Questionnaire) *GetQuestionnaireResponse {
	if entity == nil {
		return nil
	}

	questions := make([]*QuestionDTO, 0, len(entity.Questions))
	for _, q := range entity.Questions {
		questionDTO := &QuestionDTO{
			Code:        q.Code,
			Title:       q.Title,
			Type:        q.Type,
			Tips:        q.Tips,
			Placeholder: q.Placeholder,
		}

		// 转换选项
		if len(q.Options) > 0 {
			questionDTO.Options = make([]*QuestionOptionDTO, 0, len(q.Options))
			for _, opt := range q.Options {
				questionDTO.Options = append(questionDTO.Options, &QuestionOptionDTO{
					Code:    opt.Code,
					Content: opt.Content,
					Score:   opt.Score,
				})
			}
		}

		// 转换验证规则
		if len(q.ValidationRules) > 0 {
			questionDTO.ValidationRules = make([]*ValidationRuleDTO, 0, len(q.ValidationRules))
			for _, rule := range q.ValidationRules {
				questionDTO.ValidationRules = append(questionDTO.ValidationRules, &ValidationRuleDTO{
					RuleType:    rule.RuleType,
					TargetValue: rule.TargetValue,
					Message:     rule.Message,
				})
			}
		}

		questions = append(questions, questionDTO)
	}

	return &GetQuestionnaireResponse{
		Code:        entity.Code,
		Title:       entity.Title,
		Description: entity.Description,
		Status:      entity.Status,
		Questions:   questions,
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
	}
}

// FromDTO 将 DTO 转换为领域实体
func FromDTO(dto *GetQuestionnaireRequest) *questionnaire.Questionnaire {
	if dto == nil {
		return nil
	}

	return &questionnaire.Questionnaire{
		Code: dto.Code,
	}
}
