package definition

import (
	"fmt"
	"strings"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// PublishedReportTemplateLookup is the publication-time boundary for explicit
// Interpretation template routes.
type PublishedReportTemplateLookup interface {
	IsPublished(templateID string, version string) bool
}

// ValidateReportTemplateRoutes rejects publishable models that would force
// Interpretation to guess a template identity at runtime.
func ValidateReportTemplateRoutes(model *domain.AssessmentModel, published PublishedReportTemplateLookup) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil {
		return nil
	}
	sections := model.DefinitionV2.ReportMap.Sections
	if len(sections) == 0 {
		return []domain.DomainValidationIssue{reportTemplateRouteIssue(
			"report_map.sections", "report_section.required",
			"至少需要一个显式绑定报告模板的 report section",
		)}
	}

	issues := make([]domain.DomainValidationIssue, 0)
	resolvedTemplateID := ""
	resolvedTemplateVersion := ""
	routes := make(map[string]struct{})
	for _, section := range sections {
		prefix := "report_map.sections"
		if section.Code != "" {
			prefix += "." + section.Code
		}
		if section.TemplateID == "" {
			issues = append(issues, reportTemplateRouteIssue(prefix+".template_id", "report_section.template_id.required", "report template_id 不能为空"))
		} else if resolvedTemplateID == "" {
			resolvedTemplateID = section.TemplateID
		} else if resolvedTemplateID != section.TemplateID {
			issues = append(issues, reportTemplateRouteIssue(prefix+".template_id", "report_section.template_id.conflict", fmt.Sprintf("report template_id %s 与 %s 冲突", section.TemplateID, resolvedTemplateID)))
		}
		if section.TemplateVersion == "" {
			issues = append(issues, reportTemplateRouteIssue(prefix+".template_version", "report_section.template_version.required", "report template_version 不能为空"))
		} else if resolvedTemplateVersion == "" {
			resolvedTemplateVersion = section.TemplateVersion
		} else if resolvedTemplateVersion != section.TemplateVersion {
			issues = append(issues, reportTemplateRouteIssue(prefix+".template_version", "report_section.template_version.conflict", fmt.Sprintf("report template_version %s 与 %s 冲突", section.TemplateVersion, resolvedTemplateVersion)))
		}
		if model.Kind != domain.KindTypology && section.TemplateID != "" && section.TemplateID != "standard" {
			issues = append(issues, reportTemplateRouteIssue(prefix+".template_id", "report_section.template_id.model_kind_mismatch", fmt.Sprintf("模型类型 %s 只能使用 standard 报告模板", model.Kind)))
		}
		if section.TemplateID != "" && section.TemplateVersion != "" {
			routes[section.TemplateID+"\x00"+section.TemplateVersion] = struct{}{}
		}
	}
	if len(routes) == 0 {
		return issues
	}
	if published == nil {
		return append(issues, reportTemplateRouteIssue("report_map.sections", "report_template.catalog.unavailable", "报告模板发布目录未配置"))
	}
	for route := range routes {
		templateID, version, _ := strings.Cut(route, "\x00")
		if !published.IsPublished(templateID, version) {
			issues = append(issues, reportTemplateRouteIssue("report_map.sections", "report_template.unpublished", fmt.Sprintf("报告模板 %s@%s 未发布", templateID, version)))
		}
	}
	return issues
}

func reportTemplateRouteIssue(field, code, message string) domain.DomainValidationIssue {
	return domain.DomainValidationIssue{Field: field, Code: code, Message: message, Level: domain.ValidationLevelError}
}
