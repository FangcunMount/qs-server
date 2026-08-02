package answersheet

import (
	"errors"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ID 答卷 ID。
type ID = meta.ID

// NewID 创建答卷 ID。
func NewID() ID {
	return meta.New()
}

// QuestionnaireRef 问卷引用值对象
type QuestionnaireRef struct {
	code    string
	version string
	title   string
}

// NewQuestionnaireRef 创建问卷引用。
func NewQuestionnaireRef(code, version, title string) (QuestionnaireRef, error) {
	ref := QuestionnaireRef{
		code:    strings.TrimSpace(code),
		version: strings.TrimSpace(version),
		title:   strings.TrimSpace(title),
	}
	if err := ref.Validate(); err != nil {
		return QuestionnaireRef{}, err
	}
	return ref, nil
}

// Code 获取问卷编码
func (r QuestionnaireRef) Code() string {
	return r.code
}

// Version 获取问卷版本
func (r QuestionnaireRef) Version() string {
	return r.version
}

// Title 获取问卷标题
func (r QuestionnaireRef) Title() string {
	return r.title
}

// Validate 验证问卷引用
func (r QuestionnaireRef) Validate() error {
	if strings.TrimSpace(r.code) == "" {
		return errors.New("questionnaire code cannot be empty")
	}
	if strings.TrimSpace(r.version) == "" {
		return errors.New("questionnaire version cannot be empty")
	}
	return nil
}

// IsEmpty 是否为空引用
func (r QuestionnaireRef) IsEmpty() bool {
	return r.code == "" && r.version == ""
}

// SubmissionContext 描述一次答卷提交的业务上下文。
type SubmissionContext struct {
	filler      *actor.FillerRef
	testee      *actor.TesteeRef
	orgID       meta.ID
	taskID      string
	admission   Admission
	attribution AttributionSnapshot
}

func NewSubmissionContextWithAttribution(filler *actor.FillerRef, testee *actor.TesteeRef, orgID meta.ID, taskID string, attribution AttributionSnapshot, admission ...Admission) (SubmissionContext, error) {
	ctx, err := NewSubmissionContext(filler, testee, orgID, taskID, admission...)
	if err != nil {
		return SubmissionContext{}, err
	}
	ctx.attribution = attribution
	return ctx, nil
}

// NewSubmissionContext 创建提交上下文。
func NewSubmissionContext(filler *actor.FillerRef, testee *actor.TesteeRef, orgID meta.ID, taskID string, admission ...Admission) (SubmissionContext, error) {
	ctx := SubmissionContext{
		filler: cloneFillerRef(filler),
		testee: cloneTesteeRef(testee),
		orgID:  orgID,
		taskID: strings.TrimSpace(taskID),
	}
	if len(admission) > 0 {
		ctx.admission = admission[0]
	}
	if err := ctx.Validate(); err != nil {
		return SubmissionContext{}, err
	}
	return ctx, nil
}

// ReconstructSubmissionContext 从持久化数据重建提交上下文，允许历史数据缺字段。
func ReconstructSubmissionContext(filler *actor.FillerRef, testee *actor.TesteeRef, orgID meta.ID, taskID string, admission ...Admission) SubmissionContext {
	ctx := SubmissionContext{
		filler: cloneFillerRef(filler),
		testee: cloneTesteeRef(testee),
		orgID:  orgID,
		taskID: taskID,
	}
	if len(admission) > 0 {
		ctx.admission = admission[0]
	}
	return ctx
}

func ReconstructSubmissionContextWithAttribution(filler *actor.FillerRef, testee *actor.TesteeRef, orgID meta.ID, taskID string, attribution AttributionSnapshot, admission ...Admission) SubmissionContext {
	ctx := ReconstructSubmissionContext(filler, testee, orgID, taskID, admission...)
	ctx.attribution = attribution
	return ctx
}

func (c SubmissionContext) Validate() error {
	if c.filler == nil {
		return errors.New("filler is required")
	}
	if c.filler.UserID() <= 0 {
		return errors.New("filler user id is required")
	}
	if strings.TrimSpace(c.filler.FillerType().String()) == "" {
		return errors.New("filler type is required")
	}
	if c.testee == nil || c.testee.TesteeID().IsZero() {
		return errors.New("testee is required")
	}
	if c.orgID.IsZero() {
		return errors.New("org id is required")
	}
	if !c.admission.IsZero() {
		if err := c.admission.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c SubmissionContext) Filler() *actor.FillerRef {
	return cloneFillerRef(c.filler)
}

func (c SubmissionContext) Testee() *actor.TesteeRef {
	return cloneTesteeRef(c.testee)
}

func (c SubmissionContext) TesteeID() meta.ID {
	if c.testee == nil {
		return meta.ZeroID
	}
	return c.testee.TesteeID()
}

func (c SubmissionContext) OrgID() meta.ID {
	return c.orgID
}

func (c SubmissionContext) TaskID() string {
	return c.taskID
}

func (c SubmissionContext) Admission() Admission {
	return c.admission
}

func (c SubmissionContext) HasAdmission() bool {
	return !c.admission.IsZero()
}

func (c SubmissionContext) Attribution() AttributionSnapshot { return c.attribution }

func (c SubmissionContext) clone() SubmissionContext {
	return SubmissionContext{
		filler:      cloneFillerRef(c.filler),
		testee:      cloneTesteeRef(c.testee),
		orgID:       c.orgID,
		taskID:      c.taskID,
		admission:   c.admission,
		attribution: c.attribution,
	}
}

func cloneFillerRef(filler *actor.FillerRef) *actor.FillerRef {
	if filler == nil {
		return nil
	}
	return actor.NewFillerRef(filler.UserID(), filler.FillerType())
}

func cloneTesteeRef(testee *actor.TesteeRef) *actor.TesteeRef {
	if testee == nil {
		return nil
	}
	profileID := testee.ProfileID()
	if profileID != nil {
		return actor.NewTesteeRefWithProfile(testee.TesteeID(), *profileID)
	}
	return actor.NewTesteeRef(testee.TesteeID())
}
