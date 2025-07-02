package questionnaire

import (
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

// NewQuestionnaire 创建问卷
func NewQuestionnaire(code QuestionnaireCode, title string) *Questionnaire {
	q := &Questionnaire{code: code, title: title}
	return q
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

// SetID 设置问卷ID
func (q *Questionnaire) SetID(id QuestionnaireID) {
	q.id = id
}

// SetTitle 设置问卷标题
func (q *Questionnaire) SetTitle(title string) {
	q.title = title
}

// SetDescription 设置问卷描述
func (q *Questionnaire) SetDescription(description string) {
	q.description = description
}

// SetImgUrl 设置问卷图片
func (q *Questionnaire) SetImgUrl(imgUrl string) {
	q.imgUrl = imgUrl
}

// SetVersion 设置问卷版本
func (q *Questionnaire) SetVersion(version QuestionnaireVersion) {
	q.version = version
}

// SetStatus 设置问卷状态
func (q *Questionnaire) SetStatus(status QuestionnaireStatus) {
	q.status = status
}

// SetQuestions 设置问卷问题
func (q *Questionnaire) SetQuestions(questions []question.Question) {
	q.questions = questions
}

// IsPublished 判断问卷是否已发布
func (q *Questionnaire) IsPublished() bool {
	return q.status == STATUS_PUBLISHED
}

// IsArchived 判断问卷是否已归档
func (q *Questionnaire) IsArchived() bool {
	return q.status == STATUS_ARCHIVED
}
