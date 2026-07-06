package evaluation

import (
	"time"

	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== Report Mapper ====================

// ReportMapper 报告映射器
type ReportMapper struct{}

// NewReportMapper 创建报告映射器
func NewReportMapper() *ReportMapper {
	return &ReportMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *ReportMapper) ToPO(domain *report.InterpretReport, testeeID uint64) *InterpretReportPO {
	if domain == nil {
		return nil
	}

	// 转换维度列表
	dimensions := make([]DimensionInterpretPO, len(domain.Dimensions()))
	for i, d := range domain.Dimensions() {
		dimensions[i] = dimensionToPO(d)
	}

	po := &InterpretReportPO{
		ScaleName:    domain.ModelName(),
		ScaleCode:    domain.ModelCode(),
		Model:        modelIdentityToPO(domain.Model()),
		PrimaryScore: scoreValueToPO(domain.PrimaryScore()),
		Level:        resultLevelToPO(domain.Level()),
		TesteeID:     testeeID,
		TotalScore:   domain.TotalScore(),
		RiskLevel:    string(domain.RiskLevel()),
		Conclusion:   domain.Conclusion(),
		Dimensions:   dimensions,
		Suggestions:  toSuggestionPOs(domain.Suggestions()),
		ModelExtra:   toModelExtraPO(domain.ModelExtra()),
	}

	// 设置 DomainID（与 AssessmentID 一致）
	if !domain.ID().IsZero() {
		po.DomainID = meta.ID(domain.ID())
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *ReportMapper) ToDomain(po *InterpretReportPO) *report.InterpretReport {
	if po == nil {
		return nil
	}

	// 转换维度列表
	dimensions := make([]report.DimensionInterpret, len(po.Dimensions))
	for i, d := range po.Dimensions {
		dimensions[i] = dimensionToDomain(d)
	}

	modelName := po.ScaleName
	modelCode := po.ScaleCode
	totalScore := po.TotalScore
	riskLevel := report.RiskLevel(po.RiskLevel)
	var model report.ModelIdentity
	var primaryScore *report.ScoreValue
	var level *report.ResultLevel
	if po.Model != nil {
		model = modelIdentityToDomain(po.Model)
		if model.Title != "" {
			modelName = model.Title
		}
		if model.Code != "" {
			modelCode = model.Code
		}
	}
	if po.PrimaryScore != nil {
		primaryScore = scoreValueToDomain(po.PrimaryScore)
		if primaryScore != nil {
			totalScore = primaryScore.Value
		}
	}
	if po.Level != nil {
		level = resultLevelToDomain(po.Level)
		if level != nil && level.Code != "" && report.IsRiskLevelCode(level.Code) {
			riskLevel = report.RiskLevel(level.Code)
		}
	}

	// 处理更新时间
	var updatedAt *time.Time
	if !po.UpdatedAt.IsZero() && po.UpdatedAt != po.CreatedAt {
		updatedAt = &po.UpdatedAt
	}

	r := report.ReconstructInterpretReport(
		report.ID(po.DomainID),
		modelName,
		modelCode,
		totalScore,
		riskLevel,
		po.Conclusion,
		dimensions,
		toDomainSuggestions(po.Suggestions),
		toDomainModelExtra(po.ModelExtra),
		po.CreatedAt,
		updatedAt,
	)
	return report.AttachOutcomeSummary(r, model, primaryScore, level)
}

func toSuggestionPOs(items []report.Suggestion) []SuggestionPO {
	if len(items) == 0 {
		return nil
	}
	result := make([]SuggestionPO, len(items))
	for i, s := range items {
		var fc *string
		if s.FactorCode != nil {
			code := s.FactorCode.String()
			fc = &code
		}
		result[i] = SuggestionPO{
			Category:   string(s.Category),
			Content:    s.Content,
			FactorCode: fc,
		}
	}
	return result
}

func toDomainSuggestions(items []SuggestionPO) []report.Suggestion {
	if len(items) == 0 {
		return nil
	}
	result := make([]report.Suggestion, len(items))
	for i, s := range items {
		var fc *report.FactorCode
		if s.FactorCode != nil {
			code := report.NewFactorCode(*s.FactorCode)
			fc = &code
		}
		result[i] = report.Suggestion{
			Category:   report.SuggestionCategory(s.Category),
			Content:    s.Content,
			FactorCode: fc,
		}
	}
	return result
}

func toModelExtraPO(extra *report.ModelExtra) *ModelExtraPO {
	if extra == nil || extra.IsEmpty() {
		return nil
	}
	po := &ModelExtraPO{
		Kind:           extra.Kind,
		TypeCode:       extra.TypeCode,
		TypeName:       extra.TypeName,
		OneLiner:       extra.OneLiner,
		ImageURL:       extra.ImageURL,
		MatchPercent:   extra.MatchPercent,
		IsSpecial:      extra.IsSpecial,
		SpecialTrigger: extra.SpecialTrigger,
		Commentary:     extra.Commentary,
	}
	if extra.Rarity != nil {
		po.Rarity = &ModelRarityPO{
			Percent: extra.Rarity.Percent,
			Label:   extra.Rarity.Label,
			OneInX:  extra.Rarity.OneInX,
		}
	}
	return po
}

func toDomainModelExtra(po *ModelExtraPO) *report.ModelExtra {
	if po == nil {
		return nil
	}
	extra := &report.ModelExtra{
		Kind:           po.Kind,
		TypeCode:       po.TypeCode,
		TypeName:       po.TypeName,
		OneLiner:       po.OneLiner,
		ImageURL:       po.ImageURL,
		MatchPercent:   po.MatchPercent,
		IsSpecial:      po.IsSpecial,
		SpecialTrigger: po.SpecialTrigger,
		Commentary:     po.Commentary,
	}
	if po.Rarity != nil {
		extra.Rarity = &report.ModelRarity{
			Percent: po.Rarity.Percent,
			Label:   po.Rarity.Label,
			OneInX:  po.Rarity.OneInX,
		}
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}

func dimensionToPO(d report.DimensionInterpret) DimensionInterpretPO {
	po := DimensionInterpretPO{
		Kind:        string(d.Kind()),
		FactorCode:  d.Code().String(),
		FactorName:  d.Name(),
		RawScore:    d.RawScore(),
		MaxScore:    d.MaxScore(),
		RiskLevel:   d.Severity(),
		Description: d.Description(),
		Suggestion:  d.Suggestion(),
	}
	po.Score = scoreValueToPO(report.NewRawTotalScore(d.RawScore(), d.MaxScore()))
	if report.IsRiskLevelCode(d.Severity()) {
		po.Level = resultLevelToPO(report.LevelFromRisk(report.RiskLevel(d.Severity())))
	}
	return po
}

func dimensionToDomain(po DimensionInterpretPO) report.DimensionInterpret {
	rawScore := po.RawScore
	maxScore := po.MaxScore
	risk := report.RiskLevel(po.RiskLevel)
	if po.Score != nil {
		if score := scoreValueToDomain(po.Score); score != nil {
			rawScore = score.Value
			maxScore = score.Max
		}
	}
	if po.Level != nil {
		if level := resultLevelToDomain(po.Level); level != nil && level.Code != "" {
			risk = report.RiskLevel(level.Code)
		}
	}
	kind := report.DimensionKind(po.Kind)
	if kind == report.DimensionKindTrait {
		return report.NewNeutralDimensionInterpret(
			report.NewDimensionCode(po.FactorCode),
			kind,
			po.FactorName,
			rawScore,
			maxScore,
			nil,
			po.Description,
			po.Suggestion,
		)
	}
	return report.NewDimensionInterpret(
		report.NewFactorCode(po.FactorCode),
		po.FactorName,
		rawScore,
		maxScore,
		risk,
		po.Description,
		po.Suggestion,
	)
}

func modelIdentityToPO(model report.ModelIdentity) *ModelIdentityPO {
	if model.IsEmpty() {
		return nil
	}
	return &ModelIdentityPO{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func modelIdentityToDomain(po *ModelIdentityPO) report.ModelIdentity {
	if po == nil {
		return report.ModelIdentity{}
	}
	return report.ModelIdentity{
		Kind:      po.Kind,
		SubKind:   po.SubKind,
		Algorithm: po.Algorithm,
		Code:      po.Code,
		Version:   po.Version,
		Title:     po.Title,
	}
}

func scoreValueToPO(score *report.ScoreValue) *ScoreValuePO {
	if score == nil {
		return nil
	}
	return &ScoreValuePO{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func scoreValueToDomain(po *ScoreValuePO) *report.ScoreValue {
	if po == nil {
		return nil
	}
	return &report.ScoreValue{
		Kind:  po.Kind,
		Value: po.Value,
		Label: po.Label,
		Max:   po.Max,
	}
}

func resultLevelToPO(level *report.ResultLevel) *ResultLevelPO {
	if level == nil {
		return nil
	}
	return &ResultLevelPO{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func resultLevelToDomain(po *ResultLevelPO) *report.ResultLevel {
	if po == nil {
		return nil
	}
	return &report.ResultLevel{
		Code:     po.Code,
		Label:    po.Label,
		Severity: po.Severity,
	}
}

// ToDomainList 批量转换持久化对象为领域对象
func (m *ReportMapper) ToDomainList(pos []*InterpretReportPO) []*report.InterpretReport {
	if len(pos) == 0 {
		return nil
	}

	result := make([]*report.InterpretReport, 0, len(pos))
	for _, po := range pos {
		if domain := m.ToDomain(po); domain != nil {
			result = append(result, domain)
		}
	}
	return result
}
