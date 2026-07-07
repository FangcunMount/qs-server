package typology

import (
	"strings"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// Profile 人格类报告展示配置。
type Profile struct {
	Kind             string
	DefaultModelName string
	DefaultModelCode string
	TypeCode         string
	TypeName         string
	OneLiner         string
	ImageURL         string
	MatchPercent     float64
	IsSpecial        bool
	SpecialTrigger   string
	Rarity           *domainreport.ModelRarity
	Commentary       string
}

// Input 人格类报告组装输入。
type Input struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Profile      Profile
	Conclusion   string
	Dimensions   []domainreport.DimensionInterpret
	Suggestions  []domainreport.Suggestion
}

// ReportModelName 返回展示用模型名称。
func (p Profile) ReportModelName() string {
	if p.TypeName == "" {
		return p.DefaultModelName
	}
	return p.DefaultModelName + " - " + p.TypeName
}

// ReportModelCode 返回展示用模型编码。
func (p Profile) ReportModelCode(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if p.TypeCode != "" {
		return p.TypeCode
	}
	return p.DefaultModelCode
}

// Conclusion 返回人格类结论标题。
func (p Profile) Conclusion(suffix string) string {
	title := strings.TrimSpace(p.TypeCode + " " + p.TypeName)
	if p.OneLiner != "" {
		title += " - " + p.OneLiner
	}
	if suffix != "" {
		title += suffix
	}
	return strings.TrimSpace(title)
}

// ModelExtra 返回人格类扩展信息。
func (p Profile) ModelExtra() *domainreport.ModelExtra {
	extra := &domainreport.ModelExtra{
		Kind:           p.Kind,
		TypeCode:       p.TypeCode,
		TypeName:       p.TypeName,
		OneLiner:       p.OneLiner,
		ImageURL:       p.ImageURL,
		MatchPercent:   p.MatchPercent,
		IsSpecial:      p.IsSpecial,
		SpecialTrigger: p.SpecialTrigger,
		Rarity:         p.Rarity,
		Commentary:     p.Commentary,
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}
