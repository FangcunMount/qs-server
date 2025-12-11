package questionnaire

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Questionnaire 问卷聚合根
type Questionnaire struct {
	id   meta.ID
	code meta.Code

	// —— 基本信息
	typ   QuestionnaireType // 问卷分类：调查问卷/医学量表
	title  string // 问卷标题
	desc   string // 问卷描述
	imgUrl string // 问卷封面图片URL

	// —— 通用状态
	version Version // 问卷版本号
	status  Status  // 问卷状态： 草稿/已发布/已归档

	// —— 问题列表
	questions []Question // 问卷中的所有问题
}

// ===================== Questionnaires 构造相关 =================

// QuestionnaireOption 统一的构造选项
type QuestionnaireOption func(*Questionnaire)

// NewQuestionnaire 创建问卷
func NewQuestionnaire(c meta.Code, t string, opts ...QuestionnaireOption) (*Questionnaire, error) {
	if c.Value() == "" {
		return nil, errors.WithCode(code.ErrQuestionnaireInvalidCode, "code cannot be empty")
	}
	if t == "" {
		return nil, errors.WithCode(code.ErrQuestionnaireInvalidTitle, "title cannot be empty")
	}

	// 设置必填字段
	q := &Questionnaire{code: c, title: t}

	// 应用可选字段
	for _, opt := range opts {
		opt(q)
	}

	// 设置默认类型并校验
	if q.typ == "" {
		q.typ = DefaultQuestionnaireType()
	}
	if !q.typ.IsValid() {
		return nil, errors.WithCode(code.ErrQuestionnaireInvalidInput, "invalid questionnaire type")
	}

	return q, nil
}

// With*** 构造选项
func WithID(id meta.ID) QuestionnaireOption      { return func(q *Questionnaire) { q.id = id } }
func WithTitle(title string) QuestionnaireOption { return func(q *Questionnaire) { q.title = title } }
func WithDesc(d string) QuestionnaireOption      { return func(q *Questionnaire) { q.desc = d } }
func WithImgUrl(url string) QuestionnaireOption  { return func(q *Questionnaire) { q.imgUrl = url } }
func WithVersion(v Version) QuestionnaireOption  { return func(q *Questionnaire) { q.version = v } }
func WithStatus(s Status) QuestionnaireOption    { return func(q *Questionnaire) { q.status = s } }
func WithType(t QuestionnaireType) QuestionnaireOption {
	return func(q *Questionnaire) { q.typ = t }
}
func WithQuestions(ques []Question) QuestionnaireOption {
	return func(q *Questionnaire) { q.questions = ques }
}

// ===================== 对外暴露方法，供外部调用 =====================

// ----------------- 基本信息相关 ----------------

// 获取问卷基本信息
func (q *Questionnaire) GetID() meta.ID         { return q.id }
func (q *Questionnaire) GetCode() meta.Code     { return q.code }
func (q *Questionnaire) GetType() QuestionnaireType {
	if q.typ == "" {
		return DefaultQuestionnaireType()
	}
	return q.typ
}
func (q *Questionnaire) GetTitle() string       { return q.title }
func (q *Questionnaire) GetDescription() string { return q.desc }
func (q *Questionnaire) GetImgUrl() string      { return q.imgUrl }
func (q *Questionnaire) GetVersion() Version    { return q.version }
func (q *Questionnaire) GetStatus() Status      { return q.status }

// Status 问卷状态判断
func (q *Questionnaire) IsDraft() bool     { return q.status == STATUS_DRAFT }
func (q *Questionnaire) IsPublished() bool { return q.status == STATUS_PUBLISHED }
func (q *Questionnaire) IsArchived() bool  { return q.status == STATUS_ARCHIVED }

// CanBePublished 检查问卷是否可以发布
// 使用 Validator 进行完整的业务规则验证
func (q *Questionnaire) CanBePublished() bool {
	// 基本状态检查
	if q.IsArchived() || q.IsPublished() {
		return false
	}

	// 使用 Validator 进行详细验证
	validator := Validator{}
	validationErrors := validator.ValidateForPublish(q)
	return len(validationErrors) == 0
}

// ----------------- Question 相关 ----------------

// GetQuestions 获取问卷中的所有问题
func (q *Questionnaire) GetQuestions() []Question {
	return q.questions
}

// GetQuestionByCode 根据问题编码获取问题
func (q *Questionnaire) GetQuestionByCode(c meta.Code) (Question, bool) {
	for _, que := range q.GetQuestions() {
		if que.GetCode() == c {
			return que, true
		}
	}
	return nil, false
}

// QuestionCount 获取问卷中的问题个数
func (q *Questionnaire) QuestionCount() int {
	return len(q.questions)
}

// ===================== 包内私有方法，供领域服务调用 =====================

// updateStatus 更新状态
func (q *Questionnaire) updateStatus(newStatus Status) error {
	if q.status == STATUS_ARCHIVED && newStatus != STATUS_ARCHIVED {
		return errors.WithCode(code.ErrQuestionnaireArchived, "archived questionnaire cannot change status")
	}

	q.status = newStatus
	return nil
}

// updateBasicInfo 更新基本信息
func (q *Questionnaire) updateBasicInfo(title, desc, imgUrl string) error {
	if title == "" {
		return errors.WithCode(code.ErrQuestionnaireInvalidTitle, "title cannot be empty")
	}

	q.title, q.desc, q.imgUrl = title, desc, imgUrl
	return nil
}

// updateType 更新问卷类型
func (q *Questionnaire) updateType(newType QuestionnaireType) error {
	normalized := NormalizeQuestionnaireType(newType.String())
	if newType != "" && normalized != newType {
		return errors.WithCode(code.ErrQuestionnaireInvalidInput, "invalid questionnaire type")
	}
	q.typ = normalized
	return nil
}

// addQuestion 添加问题
func (q *Questionnaire) addQuestion(que Question) error {
	// 幂等性检查
	for _, queExisted := range q.questions {
		if queExisted.GetCode() == que.GetCode() {
			return errors.WithCode(code.ErrQuestionAlreadyExists, "question code already exists")
		}
	}

	// 向问题列表尾部追加
	q.questions = append(q.questions, que)
	return nil
}

// removeQuestion 移除问题
func (q *Questionnaire) removeQuestion(c meta.Code) error {
	for i, que := range q.questions {
		if que.GetCode() == c {
			q.questions = append(q.questions[:i], q.questions[i+1:]...)
			return nil
		}
	}

	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "question not found")
}

// updateVersion 更新版本
func (q *Questionnaire) updateVersion(newVersion Version) error {
	if newVersion.IsEmpty() {
		return errors.WithCode(code.ErrQuestionnaireInvalidInput, "version cannot be empty")
	}

	q.version = newVersion
	return nil
}
