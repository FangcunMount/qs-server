package reporttemplate

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Actor struct {
	OperatorUserID int64
}

type CreateDraftCommand struct {
	Actor           Actor
	TemplateID      string
	TemplateVersion policy.TemplateVersion
}

type PublishCommand struct {
	Actor           Actor
	TemplateID      string
	TemplateVersion policy.TemplateVersion
}

type DisableCommand struct {
	Actor           Actor
	TemplateID      string
	TemplateVersion policy.TemplateVersion
}

type Service interface {
	Get(ctx context.Context, templateID string, version policy.TemplateVersion) (*domainreporttemplate.ReportTemplate, error)
	List(ctx context.Context, templateID string, limit int) ([]*domainreporttemplate.ReportTemplate, error)
	CreateDraft(ctx context.Context, command CreateDraftCommand) (*domainreporttemplate.ReportTemplate, error)
	Publish(ctx context.Context, command PublishCommand) (*domainreporttemplate.ReportTemplate, error)
	Disable(ctx context.Context, command DisableCommand) (*domainreporttemplate.ReportTemplate, error)
}

func (s *service) List(ctx context.Context, templateID string, limit int) ([]*domainreporttemplate.ReportTemplate, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("report template service is not configured")
	}
	return s.repo.ListByTemplateID(ctx, templateID, limit)
}

func (s *service) Get(ctx context.Context, templateID string, version policy.TemplateVersion) (*domainreporttemplate.ReportTemplate, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("report template service is not configured")
	}
	return s.repo.FindByKey(ctx, templateID, version)
}

type service struct {
	repo      domainreporttemplate.Repository
	manifests domainreporttemplate.ManifestCatalog
	now       func() time.Time
	newID     func() meta.ID
}

func NewService(repo domainreporttemplate.Repository, manifests domainreporttemplate.ManifestCatalog) Service {
	return &service{repo: repo, manifests: manifests, now: time.Now, newID: meta.New}
}

func (s *service) CreateDraft(ctx context.Context, command CreateDraftCommand) (*domainreporttemplate.ReportTemplate, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("report template service is not configured")
	}
	if command.Actor.OperatorUserID == 0 {
		return nil, fmt.Errorf("operator identity is required")
	}
	if s.manifests == nil {
		return nil, fmt.Errorf("report template manifest catalog is not configured")
	}
	manifest, ok := s.manifests.ResolveManifest(command.TemplateID, command.TemplateVersion)
	if !ok {
		return nil, fmt.Errorf("report template release is not registered in the current binary: %s@%s", command.TemplateID, command.TemplateVersion)
	}
	now := s.now()
	tmpl, err := domainreporttemplate.NewDraft(domainreporttemplate.CreateInput{
		ID: s.newID(), Manifest: manifest, CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.FindByKey(ctx, tmpl.TemplateID(), tmpl.TemplateVersion()); err == nil {
		return nil, domainreporttemplate.ErrAlreadyExists
	} else if err != domainreporttemplate.ErrNotFound {
		return nil, err
	}
	if err := s.repo.Save(ctx, tmpl); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func (s *service) Publish(ctx context.Context, command PublishCommand) (*domainreporttemplate.ReportTemplate, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("report template service is not configured")
	}
	actor := actorLabel(command.Actor)
	if actor == "" {
		return nil, fmt.Errorf("operator identity is required")
	}
	tmpl, err := s.repo.FindByKey(ctx, command.TemplateID, command.TemplateVersion)
	if err != nil {
		return nil, err
	}
	if err := validateReleaseMetadata(tmpl, s.manifests); err != nil {
		return nil, err
	}
	if err := tmpl.Publish(actor, s.now()); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, tmpl); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func validateReleaseMetadata(tmpl *domainreporttemplate.ReportTemplate, manifests domainreporttemplate.ManifestCatalog) error {
	if tmpl == nil {
		return fmt.Errorf("report template is required")
	}
	if manifests == nil {
		return fmt.Errorf("report template manifest catalog is not configured")
	}
	expected, ok := manifests.ResolveManifest(tmpl.TemplateID(), tmpl.TemplateVersion())
	if !ok {
		return fmt.Errorf("report template release is not registered in the current binary: %s@%s", tmpl.TemplateID(), tmpl.TemplateVersion())
	}
	expectedFingerprint, err := expected.Fingerprint()
	if err != nil {
		return err
	}
	if tmpl.ManifestFingerprint() != expectedFingerprint {
		return fmt.Errorf("report template release manifest does not match the current binary: %s@%s", tmpl.TemplateID(), tmpl.TemplateVersion())
	}
	return nil
}

func (s *service) Disable(ctx context.Context, command DisableCommand) (*domainreporttemplate.ReportTemplate, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("report template service is not configured")
	}
	actor := actorLabel(command.Actor)
	if actor == "" {
		return nil, fmt.Errorf("operator identity is required")
	}
	tmpl, err := s.repo.FindByKey(ctx, command.TemplateID, command.TemplateVersion)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Disable(actor, s.now()); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, tmpl); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func actorLabel(actor Actor) string {
	if actor.OperatorUserID == 0 {
		return ""
	}
	return fmt.Sprintf("user:%d", actor.OperatorUserID)
}

// BootstrapDrafts are canonical report-template releases seeded on repository init.
var BootstrapDrafts = []CreateDraftCommand{
	{TemplateID: "standard", TemplateVersion: policy.TemplateVersionV1},
	{TemplateID: "mbti", TemplateVersion: policy.TemplateVersionV1},
	{TemplateID: "sbti", TemplateVersion: policy.TemplateVersionV1},
	{TemplateID: "bigfive", TemplateVersion: policy.TemplateVersionV1},
	{TemplateID: "enneagram", TemplateVersion: policy.TemplateVersionV1},
}
