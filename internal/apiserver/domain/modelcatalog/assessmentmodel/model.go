package assessmentmodel

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

// AssessmentModel 是后台可编辑测评模型配置聚合。
type AssessmentModel struct {
	// 唯一标识
	ID   string
	Code string
	// 类型
	Kind    binding.Kind
	SubKind binding.SubKind
	// 算法
	Algorithm binding.Algorithm

	// 标题
	Title       string
	Description string
	// 分类
	Category       string
	ProductChannel binding.ProductChannel
	Stages         []string
	ApplicableAges []string
	Reporters      []string
	Tags           []string

	// 状态
	Status Status
	// 绑定问卷
	Binding binding.QuestionnaireBinding
	// 定义
	Definition DefinitionPayload
	// DefinitionV2 是目标领域定义模型；Definition 保留为 legacy payload 兼容投影。
	DefinitionV2 *definition.Definition
	// 版本
	// Version is the persisted compatibility field for the draft configuration
	// revision. Business versioning is anchored by QuestionnaireBinding.Version.
	// New domain code should call Revision() when it means config revision.
	Version int64
	// 创建时间
	CreatedAt time.Time
	// 更新时间
	UpdatedAt time.Time
	// 发布时间
	PublishedAt *time.Time
	ArchivedAt  *time.Time
}

// NewInput 携带创建 draft assessment model 所需字段。
type NewInput struct {
	Code           string
	Kind           binding.Kind
	SubKind        binding.SubKind
	Algorithm      binding.Algorithm
	ProductChannel binding.ProductChannel
	Title          string
	Description    string
	Category       string
	Stages         []string
	ApplicableAges []string
	Reporters      []string
	Tags           []string
	Now            time.Time
}

// New 创建 draft assessment model，并补齐默认产品通道。
func New(input NewInput) (*AssessmentModel, error) {
	if input.Code == "" {
		return nil, fmt.Errorf("%w: code is required", ErrInvalidArgument)
	}
	if input.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidArgument)
	}
	if !input.Kind.IsValid() {
		return nil, fmt.Errorf("%w: kind is invalid", ErrInvalidArgument)
	}
	productChannel, err := binding.CompleteProductChannel(input.Kind, input.ProductChannel)
	if err != nil {
		return nil, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &AssessmentModel{
		Code:           input.Code,
		Kind:           input.Kind,
		SubKind:        input.SubKind,
		Algorithm:      input.Algorithm,
		ProductChannel: productChannel,
		Title:          input.Title,
		Description:    input.Description,
		Category:       input.Category,
		Stages:         append([]string(nil), input.Stages...),
		ApplicableAges: append([]string(nil), input.ApplicableAges...),
		Reporters:      append([]string(nil), input.Reporters...),
		Tags:           append([]string(nil), input.Tags...),
		Status:         StatusDraft,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (m *AssessmentModel) IsArchived() bool {
	return m != nil && m.Status.IsArchived()
}

func (m *AssessmentModel) IsPublished() bool {
	return m != nil && m.Status.IsPublished()
}

func (m *AssessmentModel) IsDraft() bool {
	return m != nil && m.Status.IsDraft()
}

// Revision returns the draft configuration revision kept in the persisted
// version field for compatibility.
func (m *AssessmentModel) Revision() int64 {
	if m == nil {
		return 0
	}
	return m.Version
}

func (m *AssessmentModel) ensureEditable() error {
	if m == nil {
		return fmt.Errorf("%w: model is nil", ErrInvalidArgument)
	}
	if m.IsArchived() {
		return fmt.Errorf("%w: archived model cannot be edited", ErrInvalidState)
	}
	return nil
}

func (m *AssessmentModel) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.UpdatedAt = now
	m.Version++
}

// UpdateBasicInfo updates editable metadata on the draft model.
func (m *AssessmentModel) UpdateBasicInfo(title, description string, subKind binding.SubKind, algorithm binding.Algorithm, productChannel binding.ProductChannel, category string, tags []string, now time.Time) error {
	if err := m.ensureEditable(); err != nil {
		return err
	}
	if title == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidArgument)
	}
	m.Title = title
	m.Description = description
	if subKind != "" {
		m.SubKind = subKind
	}
	if algorithm != "" {
		m.Algorithm = algorithm
	}
	if productChannel != "" {
		resolved, err := binding.CompleteProductChannel(m.Kind, productChannel)
		if err != nil {
			return err
		}
		m.ProductChannel = resolved
	}
	m.Category = category
	m.Tags = append([]string(nil), tags...)
	m.touch(now)
	return nil
}

// UpdateAudienceMetadata updates scale-oriented catalog dimensions that are
// exposed by legacy scale REST contracts and now live on AssessmentModel.
func (m *AssessmentModel) UpdateAudienceMetadata(stages, applicableAges, reporters []string, now time.Time) error {
	if err := m.ensureEditable(); err != nil {
		return err
	}
	m.Stages = append([]string(nil), stages...)
	m.ApplicableAges = append([]string(nil), applicableAges...)
	m.Reporters = append([]string(nil), reporters...)
	m.touch(now)
	return nil
}

// BindQuestionnaire attaches a questionnaire version to the draft model.
func (m *AssessmentModel) BindQuestionnaire(binding binding.QuestionnaireBinding, now time.Time) error {
	if err := m.ensureEditable(); err != nil {
		return err
	}
	if binding.QuestionnaireCode == "" {
		return fmt.Errorf("%w: questionnaire code is required", ErrInvalidArgument)
	}
	if binding.QuestionnaireVersion == "" {
		return fmt.Errorf("%w: questionnaire version is required", ErrInvalidArgument)
	}
	m.Binding = binding
	m.touch(now)
	return nil
}

// UpdateDefinition replaces the draft definition payload.
func (m *AssessmentModel) UpdateDefinition(payload DefinitionPayload, now time.Time) error {
	return m.UpdateDefinitionWithV2(payload, nil, now)
}

// UpdateDefinitionWithV2 replaces the draft definition payload and optionally stores
// the target definition model beside the legacy payload.
func (m *AssessmentModel) UpdateDefinitionWithV2(payload DefinitionPayload, definitionV2 *definition.Definition, now time.Time) error {
	if err := m.ensureEditable(); err != nil {
		return err
	}
	if payload.IsEmpty() {
		return fmt.Errorf("%w: definition payload is required", ErrInvalidArgument)
	}
	m.Definition = payload
	m.DefinitionV2 = definitionV2
	m.touch(now)
	return nil
}

// ForkDraftFromPublished derives a working draft from a published head without
// changing the active published runtime snapshot.
func (m *AssessmentModel) ForkDraftFromPublished(now time.Time) error {
	if m == nil || !m.IsPublished() {
		return nil
	}
	m.Status = StatusDraft
	m.touch(now)
	return nil
}

// MarkPublished transitions model to published status.
func (m *AssessmentModel) MarkPublished(now time.Time) error {
	if m == nil {
		return fmt.Errorf("%w: model is nil", ErrInvalidArgument)
	}
	if m.IsArchived() {
		return fmt.Errorf("%w: archived model cannot be published", ErrInvalidState)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.Status = StatusPublished
	m.PublishedAt = &now
	m.touch(now)
	return nil
}

// MarkUnpublished transitions a published model back to draft.
func (m *AssessmentModel) MarkUnpublished(now time.Time) error {
	if m == nil {
		return fmt.Errorf("%w: model is nil", ErrInvalidArgument)
	}
	if m.IsArchived() {
		return fmt.Errorf("%w: archived model cannot be unpublished", ErrInvalidState)
	}
	if !m.IsPublished() {
		return fmt.Errorf("%w: model is not published", ErrInvalidState)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.Status = StatusDraft
	m.PublishedAt = nil
	m.touch(now)
	return nil
}

// MarkArchived transitions model to archived status.
func (m *AssessmentModel) MarkArchived(now time.Time) error {
	if m == nil {
		return fmt.Errorf("%w: model is nil", ErrInvalidArgument)
	}
	if m.IsArchived() {
		return fmt.Errorf("%w: model is already archived", ErrInvalidState)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.Status = StatusArchived
	m.ArchivedAt = &now
	m.touch(now)
	return nil
}
