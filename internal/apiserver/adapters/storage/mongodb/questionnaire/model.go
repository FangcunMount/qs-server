package questionnaire

import (
	"fmt"
	"time"
)

// Document MongoDB 问卷文档模型
type Document struct {
	ID        string             `bson:"_id"`
	Questions []QuestionDocument `bson:"questions"`
	Settings  SettingsDocument   `bson:"settings"`
	Version   int                `bson:"version"`
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

// QuestionDocument 问题文档
type QuestionDocument struct {
	ID       string                 `bson:"id"`
	Type     string                 `bson:"type"`
	Title    string                 `bson:"title"`
	Required bool                   `bson:"required"`
	Options  []OptionDocument       `bson:"options"`
	Settings map[string]interface{} `bson:"settings"`
	Order    int                    `bson:"order"`
}

// OptionDocument 选项文档
type OptionDocument struct {
	ID    string `bson:"id"`
	Text  string `bson:"text"`
	Value string `bson:"value"`
	Order int    `bson:"order"`
}

// SettingsDocument 设置文档
type SettingsDocument struct {
	AllowAnonymous bool   `bson:"allow_anonymous"`
	ShowProgress   bool   `bson:"show_progress"`
	RandomOrder    bool   `bson:"random_order"`
	TimeLimit      *int64 `bson:"time_limit,omitempty"` // 秒数
}

// TableName 返回集合名称
func (Document) TableName() string {
	return "questionnaire_docs"
}

// GetID 获取文档ID
func (d *Document) GetID() string {
	return d.ID
}

// SetID 设置文档ID
func (d *Document) SetID(id string) {
	d.ID = id
}

// GetVersion 获取版本号
func (d *Document) GetVersion() int {
	return d.Version
}

// SetVersion 设置版本号
func (d *Document) SetVersion(version int) {
	d.Version = version
}

// GetCreatedAt 获取创建时间
func (d *Document) GetCreatedAt() time.Time {
	return d.CreatedAt
}

// SetCreatedAt 设置创建时间
func (d *Document) SetCreatedAt(t time.Time) {
	d.CreatedAt = t
}

// GetUpdatedAt 获取更新时间
func (d *Document) GetUpdatedAt() time.Time {
	return d.UpdatedAt
}

// SetUpdatedAt 设置更新时间
func (d *Document) SetUpdatedAt(t time.Time) {
	d.UpdatedAt = t
}

// UpdateTimestamp 更新时间戳
func (d *Document) UpdateTimestamp() {
	now := time.Now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
}

// GetQuestionCount 获取问题数量
func (d *Document) GetQuestionCount() int {
	return len(d.Questions)
}

// GetQuestionByID 根据ID获取问题
func (d *Document) GetQuestionByID(id string) *QuestionDocument {
	for i := range d.Questions {
		if d.Questions[i].ID == id {
			return &d.Questions[i]
		}
	}
	return nil
}

// AddQuestion 添加问题
func (d *Document) AddQuestion(question QuestionDocument) {
	d.Questions = append(d.Questions, question)
	d.UpdateTimestamp()
}

// RemoveQuestion 删除问题
func (d *Document) RemoveQuestion(id string) bool {
	for i, question := range d.Questions {
		if question.ID == id {
			d.Questions = append(d.Questions[:i], d.Questions[i+1:]...)
			d.UpdateTimestamp()
			return true
		}
	}
	return false
}

// UpdateQuestion 更新问题
func (d *Document) UpdateQuestion(question QuestionDocument) bool {
	for i := range d.Questions {
		if d.Questions[i].ID == question.ID {
			d.Questions[i] = question
			d.UpdateTimestamp()
			return true
		}
	}
	return false
}

// SortQuestionsByOrder 根据顺序排序问题
func (d *Document) SortQuestionsByOrder() {
	// 使用简单的冒泡排序
	n := len(d.Questions)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if d.Questions[j].Order > d.Questions[j+1].Order {
				d.Questions[j], d.Questions[j+1] = d.Questions[j+1], d.Questions[j]
			}
		}
	}
}

// GetOptionByID 根据ID获取选项（从指定问题中）
func (q *QuestionDocument) GetOptionByID(id string) *OptionDocument {
	for i := range q.Options {
		if q.Options[i].ID == id {
			return &q.Options[i]
		}
	}
	return nil
}

// AddOption 添加选项
func (q *QuestionDocument) AddOption(option OptionDocument) {
	q.Options = append(q.Options, option)
}

// RemoveOption 删除选项
func (q *QuestionDocument) RemoveOption(id string) bool {
	for i, option := range q.Options {
		if option.ID == id {
			q.Options = append(q.Options[:i], q.Options[i+1:]...)
			return true
		}
	}
	return false
}

// UpdateOption 更新选项
func (q *QuestionDocument) UpdateOption(option OptionDocument) bool {
	for i := range q.Options {
		if q.Options[i].ID == option.ID {
			q.Options[i] = option
			return true
		}
	}
	return false
}

// SortOptionsByOrder 根据顺序排序选项
func (q *QuestionDocument) SortOptionsByOrder() {
	n := len(q.Options)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if q.Options[j].Order > q.Options[j+1].Order {
				q.Options[j], q.Options[j+1] = q.Options[j+1], q.Options[j]
			}
		}
	}
}

// GetOptionCount 获取选项数量
func (q *QuestionDocument) GetOptionCount() int {
	return len(q.Options)
}

// IsMultiChoice 是否为多选题
func (q *QuestionDocument) IsMultiChoice() bool {
	return q.Type == "multiple_choice" || q.Type == "checkbox"
}

// IsSingleChoice 是否为单选题
func (q *QuestionDocument) IsSingleChoice() bool {
	return q.Type == "single_choice" || q.Type == "radio"
}

// IsTextInput 是否为文本输入题
func (q *QuestionDocument) IsTextInput() bool {
	return q.Type == "text" || q.Type == "textarea"
}

// Validate 验证文档有效性
func (d *Document) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("document ID cannot be empty")
	}

	if len(d.Questions) == 0 {
		return fmt.Errorf("document must have at least one question")
	}

	// 验证问题
	questionIDs := make(map[string]bool)
	for i, question := range d.Questions {
		if question.ID == "" {
			return fmt.Errorf("question at index %d has empty ID", i)
		}

		if questionIDs[question.ID] {
			return fmt.Errorf("duplicate question ID: %s", question.ID)
		}
		questionIDs[question.ID] = true

		if question.Title == "" {
			return fmt.Errorf("question %s has empty title", question.ID)
		}

		if question.Type == "" {
			return fmt.Errorf("question %s has empty type", question.ID)
		}

		// 验证选项
		if question.IsMultiChoice() || question.IsSingleChoice() {
			if len(question.Options) == 0 {
				return fmt.Errorf("choice question %s must have at least one option", question.ID)
			}

			optionIDs := make(map[string]bool)
			for j, option := range question.Options {
				if option.ID == "" {
					return fmt.Errorf("option at index %d in question %s has empty ID", j, question.ID)
				}

				if optionIDs[option.ID] {
					return fmt.Errorf("duplicate option ID %s in question %s", option.ID, question.ID)
				}
				optionIDs[option.ID] = true

				if option.Text == "" {
					return fmt.Errorf("option %s in question %s has empty text", option.ID, question.ID)
				}
			}
		}
	}

	return nil
}
