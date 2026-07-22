package patterns

import (
	"fmt"
	"strings"
)

// MBTIPersonalityTypeTemplate 返回呈现 template 用于 MBTI reports。
func MBTIPersonalityTypeTemplate() PersonalityTypeReportTemplate {
	maxScore := 40.0
	return PersonalityTypeReportTemplate{
		Kind:              string(ReportAdapterPersonalityType),
		DefaultModelName:  "MBTI 人格类型测评",
		DefaultModelCode:  "MBTI_OEJTS",
		DimensionMaxScore: &maxScore,
		DimensionDescription: func(name, preference string, rawScore, strength float64, _, _ string) string {
			return fmt.Sprintf("%s：倾向 %s（原始分 %.0f，偏好强度 %.0f%%）", name, preference, rawScore, strength)
		},
	}
}

// SBTIPersonalityTypeTemplate 返回呈现 template 用于 SBTI reports。
func SBTIPersonalityTypeTemplate() PersonalityTypeReportTemplate {
	maxScore := 6.0
	return PersonalityTypeReportTemplate{
		Kind:              string(ReportAdapterPersonalityType),
		DefaultModelName:  "SBTI 趣味人格测评",
		DefaultModelCode:  "SBTI_FUN",
		DimensionMaxScore: &maxScore,
		DimensionDescription: func(name, _ string, rawScore, _ float64, level, model string) string {
			description := strings.TrimSpace(fmt.Sprintf("%s：%s 档，原始分 %.0f/6", name, level, rawScore))
			if model != "" {
				description = model + " / " + description
			}
			return description
		},
		ConclusionSuffix: func(detail PersonalityTypeReportDetail) string {
			if detail.MatchPercent > 0 {
				return fmt.Sprintf("（匹配度 %.0f%%）", detail.MatchPercent)
			}
			return ""
		},
	}
}

// BigFiveTraitProfileTemplate 返回呈现 template 用于 Big Five reports。
func BigFiveTraitProfileTemplate() TraitProfileReportTemplate {
	return TraitProfileReportTemplate{
		Kind:             string(ReportAdapterTraitProfile),
		DefaultModelName: "Big Five 五大人格特质测评",
		DefaultModelCode: "BIGFIVE_V1",
		TypeName:         "五大人格特质",
		ConclusionTitle:  "五大人格特质画像",
		OneLiner:         "基于各维度原始分展示人格特质分布",
	}
}

// EnneagramTraitProfileTemplate returns the presentation template for the
// nine-factor Enneagram trait profile. The current model reports the complete
// vector rather than selecting one OutcomeCode.
func EnneagramTraitProfileTemplate() TraitProfileReportTemplate {
	return TraitProfileReportTemplate{
		Kind:             string(ReportAdapterTraitProfile),
		DefaultModelName: "九型人格测评",
		DefaultModelCode: "ENNEAGRAM_45",
		TypeName:         "九型人格特质",
		ConclusionTitle:  "九型人格特质画像",
		OneLiner:         "基于九个人格类型因子的原始分展示特质分布",
	}
}
