package reporttemplate

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type memoryRepo struct {
	items map[string]*domainreporttemplate.ReportTemplate
}

func (r *memoryRepo) key(templateID string, version policy.TemplateVersion) string {
	return templateID + "|" + version.String()
}

func (r *memoryRepo) Save(_ context.Context, template *domainreporttemplate.ReportTemplate) error {
	r.items[r.key(template.TemplateID(), template.TemplateVersion())] = template
	return nil
}

func (r *memoryRepo) FindByKey(_ context.Context, templateID string, version policy.TemplateVersion) (*domainreporttemplate.ReportTemplate, error) {
	item, ok := r.items[r.key(templateID, version)]
	if !ok {
		return nil, domainreporttemplate.ErrNotFound
	}
	return item, nil
}

func (r *memoryRepo) FindPublished(ctx context.Context, templateID string, version policy.TemplateVersion) (*domainreporttemplate.ReportTemplate, error) {
	item, err := r.FindByKey(ctx, templateID, version)
	if err != nil {
		return nil, err
	}
	if !item.IsPublished() {
		return nil, domainreporttemplate.ErrNotFound
	}
	return item, nil
}

func (r *memoryRepo) IsPublished(templateID string, version string) bool {
	item, err := r.FindPublished(context.Background(), templateID, policy.TemplateVersion(version))
	return err == nil && item != nil
}

func TestServicePublishAndDisableAreAudited(t *testing.T) {
	repo := &memoryRepo{items: map[string]*domainreporttemplate.ReportTemplate{}}
	svc := NewService(repo)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	svcImpl := svc.(*service)
	svcImpl.now = func() time.Time { return now }
	svcImpl.newID = func() meta.ID { return meta.FromUint64(9) }

	draft, err := svc.CreateDraft(context.Background(), CreateDraftCommand{
		Actor: Actor{OperatorUserID: 1}, TemplateID: "mbti", TemplateVersion: "custom-v2",
		BuilderIdentity: "typology", AdapterKey: "personality_type",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status() != domainreporttemplate.StatusDraft {
		t.Fatalf("status = %s", draft.Status())
	}

	published, err := svc.Publish(context.Background(), PublishCommand{
		Actor: Actor{OperatorUserID: 2}, TemplateID: "mbti", TemplateVersion: "custom-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if published.PublishedBy() != "user:2" || published.PublishedAt() == nil {
		t.Fatalf("publish audit = %#v", published)
	}

	disabled, err := svc.Disable(context.Background(), DisableCommand{
		Actor: Actor{OperatorUserID: 3}, TemplateID: "mbti", TemplateVersion: "custom-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Status() != domainreporttemplate.StatusDisabled || disabled.DisabledBy() != "user:3" {
		t.Fatalf("disable audit = %#v", disabled)
	}
}
