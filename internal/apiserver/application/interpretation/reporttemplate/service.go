package reporttemplate

import (
	"context"
	"fmt"
	"time"

	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Actor struct {
	OperatorUserID int64
}

type CreateDraftCommand struct {
	Actor           Actor
	TemplateID      string
	TemplateVersion policy.TemplateVersion
	BuilderIdentity string
	AdapterKey      string
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
	CreateDraft(ctx context.Context, command CreateDraftCommand) (*domainreporttemplate.ReportTemplate, error)
	Publish(ctx context.Context, command PublishCommand) (*domainreporttemplate.ReportTemplate, error)
	Disable(ctx context.Context, command DisableCommand) (*domainreporttemplate.ReportTemplate, error)
}

type service struct {
	repo  domainreporttemplate.Repository
	now   func() time.Time
	newID func() meta.ID
}

func NewService(repo domainreporttemplate.Repository) Service {
	return &service{repo: repo, now: time.Now, newID: meta.New}
}

func (s *service) CreateDraft(ctx context.Context, command CreateDraftCommand) (*domainreporttemplate.ReportTemplate, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("report template service is not configured")
	}
	if command.Actor.OperatorUserID == 0 {
		return nil, fmt.Errorf("operator identity is required")
	}
	now := s.now()
	tmpl, err := domainreporttemplate.NewDraft(domainreporttemplate.CreateInput{
		ID: s.newID(), TemplateID: command.TemplateID, TemplateVersion: command.TemplateVersion,
		BuilderIdentity: command.BuilderIdentity, AdapterKey: command.AdapterKey, CreatedAt: now,
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
	if err := tmpl.Publish(actor, s.now()); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, tmpl); err != nil {
		return nil, err
	}
	return tmpl, nil
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

// LegacyBootstrapDrafts are compatibility releases seeded on repository init.
var LegacyBootstrapDrafts = []CreateDraftCommand{
	{TemplateID: "standard", TemplateVersion: policy.TemplateVersionV1, BuilderIdentity: report.BuilderIdentityFactorScoring},
	{TemplateID: "mbti", TemplateVersion: policy.TemplateVersionV1, BuilderIdentity: report.BuilderIdentityTypology, AdapterKey: "personality_type"},
	{TemplateID: "sbti", TemplateVersion: policy.TemplateVersionV1, BuilderIdentity: report.BuilderIdentityTypology, AdapterKey: "personality_type"},
	{TemplateID: "bigfive", TemplateVersion: policy.TemplateVersionV1, BuilderIdentity: report.BuilderIdentityTypology, AdapterKey: "trait_profile"},
}
