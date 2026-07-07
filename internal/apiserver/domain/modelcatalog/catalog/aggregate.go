package catalog

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// AssessmentModel is the draft-model aggregate for backend configuration.
type AssessmentModel struct {
	ID             string
	Code           string
	Kind           identity.Kind
	SubKind        identity.SubKind
	Algorithm      identity.Algorithm
	ProductChannel identity.ProductChannel
	Title          string
	Description    string
	Category       string
	Tags           []string
	Status         ModelStatus
	Binding        QuestionnaireBinding
	Definition     DefinitionPayload
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	PublishedAt    *time.Time
	ArchivedAt     *time.Time
}

// NewAssessmentModelInput carries fields required to create a draft model.
type NewAssessmentModelInput struct {
	Code           string
	Kind           identity.Kind
	SubKind        identity.SubKind
	Algorithm      identity.Algorithm
	ProductChannel identity.ProductChannel
	Title          string
	Description    string
	Category       string
	Tags           []string
	Now            time.Time
}

// NewAssessmentModel creates a draft assessment model with default product channel.
func NewAssessmentModel(input NewAssessmentModelInput) (*AssessmentModel, error) {
	if input.Code == "" {
		return nil, fmt.Errorf("%w: code is required", ErrInvalidArgument)
	}
	if input.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidArgument)
	}
	if !input.Kind.IsValid() {
		return nil, fmt.Errorf("%w: kind is invalid", ErrInvalidArgument)
	}
	productChannel, err := identity.CompleteProductChannel(input.Kind, input.ProductChannel)
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
		Tags:           append([]string(nil), input.Tags...),
		Status:         ModelStatusDraft,
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

// UpdateBasicInfo updates editable metadata on a draft model.
func (m *AssessmentModel) UpdateBasicInfo(title, description string, subKind identity.SubKind, algorithm identity.Algorithm, productChannel identity.ProductChannel, category string, tags []string, now time.Time) error {
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
		resolved, err := identity.CompleteProductChannel(m.Kind, productChannel)
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

// BindQuestionnaire attaches a questionnaire version to the draft model.
func (m *AssessmentModel) BindQuestionnaire(binding QuestionnaireBinding, now time.Time) error {
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
	if err := m.ensureEditable(); err != nil {
		return err
	}
	if payload.IsEmpty() {
		return fmt.Errorf("%w: definition payload is required", ErrInvalidArgument)
	}
	m.Definition = payload
	m.touch(now)
	return nil
}

// MarkPublished transitions the model to published status.
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
	m.Status = ModelStatusPublished
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
	m.Status = ModelStatusDraft
	m.PublishedAt = nil
	m.touch(now)
	return nil
}

// MarkArchived transitions the model to archived status.
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
	m.Status = ModelStatusArchived
	m.ArchivedAt = &now
	m.touch(now)
	return nil
}
