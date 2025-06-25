package questionnaire

import (
	"time"
)

// Questionnaire 问卷聚合根
type Questionnaire struct {
	// 聚合根标识
	id   QuestionnaireID
	code string

	// 基础信息
	title       string
	description string
	status      Status
	createdBy   string
	createdAt   time.Time
	updatedAt   time.Time

	// 问卷内容
	questions []Question
	settings  Settings
	version   int
}

// QuestionnaireID 问卷唯一标识
type QuestionnaireID struct {
	value string
}

// NewQuestionnaireID 创建问卷ID
func NewQuestionnaireID(value string) QuestionnaireID {
	return QuestionnaireID{value: value}
}

// Value 获取ID值
func (id QuestionnaireID) Value() string {
	return id.value
}

// Status 问卷状态
type Status int

const (
	StatusDraft     Status = 1 // 草稿
	StatusPublished Status = 2 // 已发布
	StatusArchived  Status = 3 // 已归档
)

// Question 问题值对象
type Question struct {
	id       string
	type_    QuestionType
	title    string
	required bool
	options  []Option
	settings map[string]interface{}
}

// ID 获取问题ID
func (q Question) ID() string {
	return q.id
}

// Type 获取问题类型
func (q Question) Type() QuestionType {
	return q.type_
}

// Title 获取问题标题
func (q Question) Title() string {
	return q.title
}

// Required 是否必填
func (q Question) Required() bool {
	return q.required
}

// Options 获取选项列表
func (q Question) Options() []Option {
	// 返回副本，保护内部状态
	result := make([]Option, len(q.options))
	copy(result, q.options)
	return result
}

// Settings 获取设置
func (q Question) Settings() map[string]interface{} {
	// 返回副本，保护内部状态
	result := make(map[string]interface{})
	for k, v := range q.settings {
		result[k] = v
	}
	return result
}

// QuestionType 问题类型
type QuestionType string

const (
	QuestionTypeSingle   QuestionType = "single"   // 单选
	QuestionTypeMultiple QuestionType = "multiple" // 多选
	QuestionTypeText     QuestionType = "text"     // 文本
	QuestionTypeRating   QuestionType = "rating"   // 评分
)

// Option 选项值对象
type Option struct {
	id    string
	text  string
	value string
}

// ID 获取选项ID
func (o Option) ID() string {
	return o.id
}

// Text 获取选项文本
func (o Option) Text() string {
	return o.text
}

// Value 获取选项值
func (o Option) Value() string {
	return o.value
}

// Settings 问卷设置值对象
type Settings struct {
	allowAnonymous bool
	showProgress   bool
	randomOrder    bool
	timeLimit      *time.Duration
}

// AllowAnonymous 是否允许匿名
func (s Settings) AllowAnonymous() bool {
	return s.allowAnonymous
}

// ShowProgress 是否显示进度
func (s Settings) ShowProgress() bool {
	return s.showProgress
}

// RandomOrder 是否随机顺序
func (s Settings) RandomOrder() bool {
	return s.randomOrder
}

// TimeLimit 获取时间限制
func (s Settings) TimeLimit() *time.Duration {
	return s.timeLimit
}

// NewQuestionnaire 创建新问卷（工厂方法）
func NewQuestionnaire(code, title, description, createdBy string) *Questionnaire {
	now := time.Now()
	return &Questionnaire{
		id:          NewQuestionnaireID(generateID()),
		code:        code,
		title:       title,
		description: description,
		status:      StatusDraft,
		createdBy:   createdBy,
		createdAt:   now,
		updatedAt:   now,
		questions:   make([]Question, 0),
		settings:    Settings{},
		version:     1,
	}
}

// ID 获取问卷ID
func (q *Questionnaire) ID() QuestionnaireID {
	return q.id
}

// Code 获取问卷代码
func (q *Questionnaire) Code() string {
	return q.code
}

// Title 获取标题
func (q *Questionnaire) Title() string {
	return q.title
}

// Description 获取描述
func (q *Questionnaire) Description() string {
	return q.description
}

// Status 获取状态
func (q *Questionnaire) Status() Status {
	return q.status
}

// CreatedBy 获取创建者
func (q *Questionnaire) CreatedBy() string {
	return q.createdBy
}

// CreatedAt 获取创建时间
func (q *Questionnaire) CreatedAt() time.Time {
	return q.createdAt
}

// UpdatedAt 获取更新时间
func (q *Questionnaire) UpdatedAt() time.Time {
	return q.updatedAt
}

// Questions 获取问题列表
func (q *Questionnaire) Questions() []Question {
	// 返回副本，保护内部状态
	result := make([]Question, len(q.questions))
	copy(result, q.questions)
	return result
}

// Settings 获取设置
func (q *Questionnaire) Settings() Settings {
	return q.settings
}

// Version 获取版本
func (q *Questionnaire) Version() int {
	return q.version
}

// UpdateBasicInfo 更新基础信息（业务操作）
func (q *Questionnaire) UpdateBasicInfo(title, description string) error {
	if title == "" {
		return ErrEmptyTitle
	}

	q.title = title
	q.description = description
	q.updatedAt = time.Now()

	return nil
}

// AddQuestion 添加问题（业务操作）
func (q *Questionnaire) AddQuestion(question Question) error {
	if q.status == StatusPublished {
		return ErrCannotModifyPublishedQuestionnaire
	}

	q.questions = append(q.questions, question)
	q.updatedAt = time.Now()
	q.version++

	return nil
}

// Publish 发布问卷（业务操作）
func (q *Questionnaire) Publish() error {
	if len(q.questions) == 0 {
		return ErrCannotPublishEmptyQuestionnaire
	}

	if q.status == StatusPublished {
		return ErrAlreadyPublished
	}

	q.status = StatusPublished
	q.updatedAt = time.Now()

	return nil
}

// Archive 归档问卷（业务操作）
func (q *Questionnaire) Archive() error {
	if q.status == StatusArchived {
		return ErrAlreadyArchived
	}

	q.status = StatusArchived
	q.updatedAt = time.Now()

	return nil
}

// 辅助函数
func generateID() string {
	// 简单实现，实际应该使用 UUID 或其他生成策略
	return time.Now().Format("20060102150405")
}
