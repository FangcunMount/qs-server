package questionnaire

import (
	"time"

	questionnairepb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
)

// Questionnaire 问卷实体
type Questionnaire struct {
	Code        string      `json:"code"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	Questions   []*Question `json:"questions"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Question 问题实体
type Question struct {
	Code            string            `json:"code"`
	Title           string            `json:"title"`
	Type            string            `json:"type"`
	Tips            string            `json:"tips"`
	Placeholder     string            `json:"placeholder"`
	Options         []*QuestionOption `json:"options,omitempty"`
	ValidationRules []*ValidationRule `json:"validation_rules,omitempty"`
}

// QuestionOption 问题选项
type QuestionOption struct {
	Code    string `json:"code"`
	Content string `json:"content"`
	Score   int32  `json:"score"`
}

// ValidationRule 验证规则
type ValidationRule struct {
	RuleType    string `json:"rule_type"`
	TargetValue string `json:"target_value"`
	Message     string `json:"message"`
}

// FromProto 从 protobuf 转换为领域实体
func FromProto(proto *questionnairepb.Questionnaire) *Questionnaire {
	if proto == nil {
		return nil
	}

	questions := make([]*Question, 0, len(proto.Questions))
	for _, q := range proto.Questions {
		question := &Question{
			Code:        q.Code,
			Title:       q.Title,
			Type:        q.Type,
			Tips:        q.Tips,
			Placeholder: q.Placeholder,
		}

		// 转换选项
		if len(q.Options) > 0 {
			question.Options = make([]*QuestionOption, 0, len(q.Options))
			for _, opt := range q.Options {
				question.Options = append(question.Options, &QuestionOption{
					Code:    opt.Code,
					Content: opt.Content,
					Score:   opt.Score,
				})
			}
		}

		// 转换验证规则
		if len(q.ValidationRules) > 0 {
			question.ValidationRules = make([]*ValidationRule, 0, len(q.ValidationRules))
			for _, rule := range q.ValidationRules {
				question.ValidationRules = append(question.ValidationRules, &ValidationRule{
					RuleType:    rule.RuleType,
					TargetValue: rule.TargetValue,
				})
			}
		}

		questions = append(questions, question)
	}

	// 解析时间字符串（假设格式为 RFC3339）
	var createdAt, updatedAt time.Time
	if proto.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, proto.CreatedAt); err == nil {
			createdAt = t
		}
	}
	if proto.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, proto.UpdatedAt); err == nil {
			updatedAt = t
		}
	}

	return &Questionnaire{
		Code:        proto.Code,
		Title:       proto.Title,
		Description: proto.Description,
		Status:      proto.Status,
		Questions:   questions,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}
