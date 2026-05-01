package response

import (
	"strings"
	"time"

	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

const (
	dateLayout     = "2006-01-02"
	dateTimeLayout = "2006-01-02 15:04:05"
)

var (
	periodicTaskStatusLabelMap = map[string]string{
		"completed": "已完成",
		"pending":   "待开放",
		"overdue":   "已逾期",
		"canceled":  "已取消",
	}
	riskLevelLabelMap = map[string]string{
		"normal": "正常",
		"none":   "正常",
		"low":    "低风险",
		"medium": "中风险",
		"high":   "高风险",
		"severe": "严重风险",
	}
)

func normalizeLookupValue(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func formatMappedLabel(value string, labelMap map[string]string, fallback string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return fallback
	}
	if label, ok := labelMap[normalized]; ok {
		return label
	}
	return fallbackValue(value, fallback)
}

func fallbackValue(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func FormatDateValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateLayout)
}

func FormatDateTimeValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateTimeLayout)
}

func FormatDatePtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	value := t.Format(dateLayout)
	return &value
}

func FormatDateTimePtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	value := t.Format(dateTimeLayout)
	return &value
}

func GenderCodeFromValue(gender int8) string {
	switch gender {
	case 1:
		return "male"
	case 2:
		return "female"
	default:
		return "unknown"
	}
}

func LabelForGender(value string) string {
	if normalized := normalizeLookupValue(value); normalized != "" {
		return domainTestee.Gender(mapGenderCode(normalized)).DisplayName()
	}
	return "未知"
}

func LabelForClinicianType(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "其他"
	}
	return domainClinician.Type(normalized).DisplayName()
}

func LabelForRelationType(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "未知关系"
	}
	return domainRelation.RelationType(normalized).DisplayName()
}

func LabelForRelationSource(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "未知来源"
	}
	return domainRelation.SourceType(normalized).DisplayName()
}

func LabelForTesteeSource(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "未知来源"
	}
	return domainTestee.Source(normalized).DisplayName()
}

func LabelForTargetType(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "未知类型"
	}
	return domainAssessmentEntry.TargetType(normalized).DisplayName()
}

func LabelForAssessmentOriginType(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "未知来源"
	}
	return domainAssessment.OriginType(normalized).DisplayName()
}

func LabelForRiskLevel(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "-"
	}
	return formatMappedLabel(value, riskLevelLabelMap, "-")
}

func LabelForAssessmentStatus(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "-"
	}
	switch normalized {
	case "interpreting":
		return "解读中"
	case "completed":
		return "已完成"
	case "processing":
		return "处理中"
	case "in_progress":
		return "进行中"
	case "canceled":
		return "已取消"
	default:
		return domainAssessment.Status(normalized).DisplayName()
	}
}

func LabelForPeriodicTaskStatus(value string) string {
	normalized := normalizeLookupValue(value)
	if normalized == "" {
		return "-"
	}
	if normalized == "overdue" {
		return "已逾期"
	}
	label := domainPlan.TaskStatus(normalized).DisplayName()
	if label != normalized {
		return label
	}
	return formatMappedLabel(value, periodicTaskStatusLabelMap, "-")
}

func LabelForKeyFocus(isKeyFocus bool) string {
	if isKeyFocus {
		return "重点关注"
	}
	return "普通关注"
}

func LabelTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		result = append(result, domainTestee.Tag(tag).DisplayName())
	}
	return result
}

func mapGenderCode(value string) domainTestee.Gender {
	switch value {
	case "male":
		return domainTestee.GenderMale
	case "female":
		return domainTestee.GenderFemale
	default:
		return domainTestee.GenderUnknown
	}
}
