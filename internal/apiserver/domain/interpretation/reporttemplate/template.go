// Package reporttemplate owns Interpretation report-template release assets.
// ModelCatalog publish versions freeze TemplateID+TemplateVersion references;
// rollback only affects subsequent selection, not historical artifacts.
package reporttemplate

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Status is the lifecycle state of one immutable template release.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusDisabled  Status = "disabled"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusDisabled:
		return true
	default:
		return false
	}
}

// ReportTemplate is one published-or-draft release of report-producing assets.
type ReportTemplate struct {
	id                  meta.ID
	manifest            ReleaseManifest
	manifestFingerprint string
	status              Status
	createdAt           time.Time
	updatedAt           time.Time
	publishedAt         *time.Time
	publishedBy         string
	disabledAt          *time.Time
	disabledBy          string
}

// CreateInput constructs a draft template release.
type CreateInput struct {
	ID        meta.ID
	Manifest  ReleaseManifest
	CreatedAt time.Time
}

// NewDraft validates and creates a draft template release.
func NewDraft(input CreateInput) (*ReportTemplate, error) {
	if input.ID.IsZero() {
		return nil, fmt.Errorf("report template id is required")
	}
	manifest := input.Manifest.Clone()
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	fingerprint, err := manifest.Fingerprint()
	if err != nil {
		return nil, err
	}
	if input.CreatedAt.IsZero() {
		return nil, fmt.Errorf("report template created_at is required")
	}
	return &ReportTemplate{
		id: input.ID, manifest: manifest, manifestFingerprint: fingerprint,
		status: StatusDraft, createdAt: input.CreatedAt, updatedAt: input.CreatedAt,
	}, nil
}

// Rehydrate restores a persisted template release.
func Rehydrate(input PersistedInput) (*ReportTemplate, error) {
	draft, err := NewDraft(CreateInput{
		ID: input.ID, Manifest: input.Manifest, CreatedAt: input.CreatedAt,
	})
	if err != nil {
		return nil, err
	}
	if !input.Status.IsValid() {
		return nil, fmt.Errorf("report template status is invalid")
	}
	if input.ManifestFingerprint != draft.manifestFingerprint {
		return nil, fmt.Errorf("report template manifest fingerprint mismatch")
	}
	if input.UpdatedAt.IsZero() || input.UpdatedAt.Before(input.CreatedAt) {
		return nil, fmt.Errorf("report template updated_at is invalid")
	}
	if err := validatePersistedLifecycle(input); err != nil {
		return nil, err
	}
	draft.status = input.Status
	draft.updatedAt = input.UpdatedAt
	draft.publishedAt = cloneTime(input.PublishedAt)
	draft.publishedBy = strings.TrimSpace(input.PublishedBy)
	draft.disabledAt = cloneTime(input.DisabledAt)
	draft.disabledBy = strings.TrimSpace(input.DisabledBy)
	return draft, nil
}

func validatePersistedLifecycle(input PersistedInput) error {
	published := input.PublishedAt != nil && !input.PublishedAt.IsZero() && strings.TrimSpace(input.PublishedBy) != ""
	disabled := input.DisabledAt != nil && !input.DisabledAt.IsZero() && strings.TrimSpace(input.DisabledBy) != ""
	switch input.Status {
	case StatusDraft:
		if published || disabled || input.PublishedAt != nil || input.PublishedBy != "" || input.DisabledAt != nil || input.DisabledBy != "" {
			return fmt.Errorf("draft report template must not contain publish or disable audit fields")
		}
	case StatusPublished:
		if !published || disabled || input.DisabledAt != nil || input.DisabledBy != "" {
			return fmt.Errorf("published report template lifecycle audit is invalid")
		}
	case StatusDisabled:
		if !published || !disabled || input.DisabledAt.Before(*input.PublishedAt) {
			return fmt.Errorf("disabled report template lifecycle audit is invalid")
		}
	}
	return nil
}

// PersistedInput is the storage shape for ReportTemplate.
type PersistedInput struct {
	ID                  meta.ID
	Manifest            ReleaseManifest
	ManifestFingerprint string
	Status              Status
	CreatedAt           time.Time
	UpdatedAt           time.Time
	PublishedAt         *time.Time
	PublishedBy         string
	DisabledAt          *time.Time
	DisabledBy          string
}

func (t *ReportTemplate) ID() meta.ID                             { return t.id }
func (t *ReportTemplate) TemplateID() string                      { return t.manifest.TemplateID }
func (t *ReportTemplate) TemplateVersion() policy.TemplateVersion { return t.manifest.TemplateVersion }
func (t *ReportTemplate) Manifest() ReleaseManifest               { return t.manifest.Clone() }
func (t *ReportTemplate) ManifestFingerprint() string             { return t.manifestFingerprint }
func (t *ReportTemplate) Status() Status                          { return t.status }
func (t *ReportTemplate) CreatedAt() time.Time                    { return t.createdAt }
func (t *ReportTemplate) UpdatedAt() time.Time                    { return t.updatedAt }
func (t *ReportTemplate) PublishedAt() *time.Time                 { return cloneTime(t.publishedAt) }
func (t *ReportTemplate) PublishedBy() string                     { return t.publishedBy }
func (t *ReportTemplate) DisabledAt() *time.Time                  { return cloneTime(t.disabledAt) }
func (t *ReportTemplate) DisabledBy() string                      { return t.disabledBy }

func (t *ReportTemplate) IsPublished() bool { return t != nil && t.status == StatusPublished }

// Publish marks a draft release as published. Published releases are immutable.
func (t *ReportTemplate) Publish(actor string, at time.Time) error {
	if t == nil {
		return fmt.Errorf("report template is required")
	}
	if t.status != StatusDraft {
		return fmt.Errorf("only draft report templates can be published")
	}
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("publish actor is required")
	}
	if at.IsZero() {
		return fmt.Errorf("publish time is required")
	}
	t.status = StatusPublished
	t.updatedAt = at
	publishedAt := at
	t.publishedAt = &publishedAt
	t.publishedBy = strings.TrimSpace(actor)
	return nil
}

// Disable retires a published release from subsequent selection.
func (t *ReportTemplate) Disable(actor string, at time.Time) error {
	if t == nil {
		return fmt.Errorf("report template is required")
	}
	if t.status != StatusPublished {
		return fmt.Errorf("only published report templates can be disabled")
	}
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("disable actor is required")
	}
	if at.IsZero() {
		return fmt.Errorf("disable time is required")
	}
	t.status = StatusDisabled
	t.updatedAt = at
	disabledAt := at
	t.disabledAt = &disabledAt
	t.disabledBy = strings.TrimSpace(actor)
	return nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
