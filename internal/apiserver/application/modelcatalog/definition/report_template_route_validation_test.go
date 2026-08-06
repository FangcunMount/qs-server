package definition

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type publishedReportTemplateStub map[string]struct{}

func (s publishedReportTemplateStub) IsPublished(templateID string, version string) bool {
	_, ok := s[templateID+"@"+version]
	return ok
}

func TestValidateReportTemplateRoutesRequiresPublishedConsistentRoute(t *testing.T) {
	model := &domain.AssessmentModel{Kind: domain.KindScale, DefinitionV2: &domain.Definition{
		ReportMap: domain.ReportMap{Sections: []domain.ReportSection{
			{Code: "summary", TemplateID: "standard", TemplateVersion: "2026-08-v1"},
			{Code: "factors", TemplateID: "standard", TemplateVersion: "2026-08-v1"},
		}},
	}}
	published := publishedReportTemplateStub{"standard@2026-08-v1": {}}
	if issues := ValidateReportTemplateRoutes(model, published); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}

	model.DefinitionV2.ReportMap.Sections[1].TemplateVersion = "legacy-v1"
	if issues := ValidateReportTemplateRoutes(model, published); !hasDomainIssue(issues, "report_section.template_version.conflict") {
		t.Fatalf("issues = %#v, want version conflict", issues)
	}
	model.DefinitionV2.ReportMap.Sections[1].TemplateVersion = "2026-08-v1"
	model.DefinitionV2.ReportMap.Sections[0].TemplateID = ""
	if issues := ValidateReportTemplateRoutes(model, published); !hasDomainIssue(issues, "report_section.template_id.required") {
		t.Fatalf("issues = %#v, want template id required", issues)
	}
}

func TestValidateReportTemplateRoutesRejectsUnpublishedAndModelMismatch(t *testing.T) {
	model := &domain.AssessmentModel{Kind: domain.KindScale, DefinitionV2: &domain.Definition{
		ReportMap: domain.ReportMap{Sections: []domain.ReportSection{{
			Code: "report", TemplateID: "mbti", TemplateVersion: "2026-08-v1",
		}}},
	}}
	issues := ValidateReportTemplateRoutes(model, publishedReportTemplateStub{})
	if !hasDomainIssue(issues, "report_section.template_id.model_kind_mismatch") || !hasDomainIssue(issues, "report_template.unpublished") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateReportTemplateRoutesRejectsMissingSectionsAndCatalog(t *testing.T) {
	model := &domain.AssessmentModel{Kind: domain.KindTypology, DefinitionV2: &domain.Definition{}}
	if issues := ValidateReportTemplateRoutes(model, nil); !hasDomainIssue(issues, "report_section.required") {
		t.Fatalf("issues = %#v, want sections required", issues)
	}
	model.DefinitionV2.ReportMap.Sections = []domain.ReportSection{{Code: "report", TemplateID: "mbti", TemplateVersion: "2026-08-v1"}}
	if issues := ValidateReportTemplateRoutes(model, nil); !hasDomainIssue(issues, "report_template.catalog.unavailable") {
		t.Fatalf("issues = %#v, want catalog unavailable", issues)
	}
}

func hasDomainIssue(issues []domain.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
