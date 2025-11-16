package questionnaire

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question"
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

type QuestionnaireOption func(*Questionnaire)

// NewQuestionnaire 创建问卷
func NewQuestionnaire(code QuestionnaireCode, title string, opts ...QuestionnaireOption) *Questionnaire {
	q := &Questionnaire{code: code, title: title}
	for _, opt := range opts {
		opt(q)
	}
	return q
}

// WithID 设置问卷ID
func WithID(id QuestionnaireID) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.id = id
	}
}

// WithTitle 设置问卷标题
func WithTitle(title string) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.title = title
	}
}

// WithDescription 设置问卷描述
func WithDescription(description string) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.description = description
	}
}

// WithImgUrl 设置问卷图片
func WithImgUrl(imgUrl string) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.imgUrl = imgUrl
	}
}

// WithVersion 设置问卷版本
func WithVersion(version QuestionnaireVersion) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.version = version
	}
}

// WithStatus 设置问卷状态
func WithStatus(status QuestionnaireStatus) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.status = status
	}
}

// WithQuestions 设置问卷问题
func WithQuestions(questions []question.Question) QuestionnaireOption {
	return func(q *Questionnaire) {
		q.questions = questions
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

// IsPublished 判断问卷是否已发布
func (q *Questionnaire) IsPublished() bool {
	return q.status == STATUS_PUBLISHED
}

// IsArchived 判断问卷是否已归档
func (q *Questionnaire) IsArchived() bool {
	return q.status == STATUS_ARCHIVED
}
