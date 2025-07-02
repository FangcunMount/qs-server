package questionnaire

import (
	"slices"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// Questionnaire 问卷
type Questionnaire struct {
	id          QuestionnaireID
	code        QuestionnaireCode
	title       string
	description string
	imgUrl      string
	version     QuestionnaireVersion
	status      QuestionnaireStatus
	questions   []question.Question
}

// Option 问卷选项
type Option func(*Questionnaire)

// NewQuestionnaire 创建问卷
func NewQuestionnaire(code QuestionnaireCode, opts ...Option) *Questionnaire {
	q := &Questionnaire{code: code}
	for _, opt := range opts {
		opt(q)
	}
	return q
}

// WithID 设置问卷ID
func WithID(id QuestionnaireID) Option {
	return func(q *Questionnaire) {
		q.id = id
	}
}

// WithTitle 设置问卷标题
func WithTitle(title string) Option {
	return func(q *Questionnaire) {
		q.title = title
	}
}

// WithDescription 设置问卷描述
func WithDescription(description string) Option {
	return func(q *Questionnaire) {
		q.description = description
	}
}

// WithImgUrl 设置问卷图片
func WithImgUrl(imgUrl string) Option {
	return func(q *Questionnaire) {
		q.imgUrl = imgUrl
	}
}

// WithVersion 设置问卷版本
func WithVersion(version QuestionnaireVersion) Option {
	return func(q *Questionnaire) {
		q.version = version
	}
}

// WithStatus 设置问卷状态
func WithStatus(status QuestionnaireStatus) Option {
	return func(q *Questionnaire) {
		q.status = status
	}
}

// SetID 设置问卷ID
func (q *Questionnaire) SetID(id QuestionnaireID) {
	q.id = id
}

// GetID 获取问卷ID
func (q *Questionnaire) GetID() QuestionnaireID {
	return q.id
}

// GetCode 获取问卷编码
func (q *Questionnaire) GetCode() QuestionnaireCode {
	return q.code
}

// GetTitle 获取问卷标题
func (q *Questionnaire) GetTitle() string {
	return q.title
}

// GetDescription 获取问卷描述
func (q *Questionnaire) GetDescription() string {
	return q.description
}

// GetImgUrl 获取问卷图片
func (q *Questionnaire) GetImgUrl() string {
	return q.imgUrl
}

// GetVersion 获取问卷版本
func (q *Questionnaire) GetVersion() QuestionnaireVersion {
	return q.version
}

// GetStatus 获取问卷状态
func (q *Questionnaire) GetStatus() QuestionnaireStatus {
	return q.status
}

// GetQuestions 获取问卷问题
func (q *Questionnaire) GetQuestions() []question.Question {
	return q.questions
}

// ChangeBasicInfo 修改问卷基本信息
func (q *Questionnaire) ChangeBasicInfo(title, description, imgUrl string) {
	q.title = title
	q.description = description
	q.imgUrl = imgUrl
}

// Publish 发布问卷
func (q *Questionnaire) Publish() {
	q.status = STATUS_PUBLISHED
	q.version = q.version.Increment()
}

// Unpublish 下架问卷
func (q *Questionnaire) Unpublish() {
	q.status = STATUS_DRAFT
}

// AddQuestion 添加问题
func (q *Questionnaire) AddQuestion(question question.Question) {
	q.questions = append(q.questions, question)
}

// RemoveQuestion 删除问题
func (q *Questionnaire) RemoveQuestion(question question.Question) {
	for i, theQuestion := range q.questions {
		if theQuestion.GetCode() == question.GetCode() {
			q.questions = slices.Delete(q.questions, i, i+1)
			break
		}
	}
}
