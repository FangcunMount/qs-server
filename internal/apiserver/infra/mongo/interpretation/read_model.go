package interpretation

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func buildReportReadModelQuery(filter evaluationreadmodel.ReportFilter) bson.M {
	query := bson.M{"deleted_at": nil, "$or": generatedReportConditions()}
	if filter.TesteeID != nil {
		query["testee_id"] = *filter.TesteeID
	}
	if len(filter.TesteeIDs) > 0 {
		query["testee_id"] = bson.M{"$in": filter.TesteeIDs}
	}
	if filter.HighRiskOnly {
		query["risk_level"] = bson.M{"$in": []string{"high", "severe"}}
	}
	if filter.ModelCode != "" {
		query["scale_code"] = filter.ModelCode
	}
	if filter.RiskLevel != nil {
		query["risk_level"] = *filter.RiskLevel
	}
	return query
}

func generatedReportConditions() bson.A {
	return bson.A{
		bson.M{"status": string(domainreport.ReportStatusGenerated)},
		bson.M{"status": bson.M{"$exists": false}},
		bson.M{"status": ""},
	}
}

func buildReportReadModelFindOptions(page evaluationreadmodel.PageRequest) *options.FindOptions {
	return options.Find().
		SetSkip(int64(page.Offset())).
		SetLimit(int64(page.Limit())).
		SetSort(bson.M{"created_at": -1})
}

func reportPOToReadRow(po *InterpretReportPO) evaluationreadmodel.ReportRow {
	if po == nil {
		return evaluationreadmodel.ReportRow{}
	}
	dimensions := make([]evaluationreadmodel.ReportDimensionRow, 0, len(po.Dimensions))
	for _, d := range po.Dimensions {
		dimensions = append(dimensions, evaluationreadmodel.ReportDimensionRow{
			FactorCode:     d.FactorCode,
			FactorName:     d.FactorName,
			RawScore:       d.RawScore,
			MaxScore:       d.MaxScore,
			RiskLevel:      d.RiskLevel,
			Role:           d.Role,
			ParentCode:     d.ParentCode,
			HierarchyLevel: d.HierarchyLevel,
			SortOrder:      d.SortOrder,
			Description:    d.Description,
			Suggestion:     d.Suggestion,
		})
	}
	suggestions := make([]evaluationreadmodel.ReportSuggestionRow, 0, len(po.Suggestions))
	for _, s := range po.Suggestions {
		suggestions = append(suggestions, evaluationreadmodel.ReportSuggestionRow{
			Category:   s.Category,
			Content:    s.Content,
			FactorCode: s.FactorCode,
		})
	}
	modelName := po.ScaleName
	modelCode := po.ScaleCode
	if po.Model != nil {
		if po.Model.Title != "" {
			modelName = po.Model.Title
		}
		if po.Model.Code != "" {
			modelCode = po.Model.Code
		}
	}
	row := evaluationreadmodel.ReportRow{
		AssessmentID: po.DomainID.Uint64(),
		ModelName:    modelName,
		ModelCode:    modelCode,
		TotalScore:   po.TotalScore,
		RiskLevel:    po.RiskLevel,
		Conclusion:   po.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		ModelExtra:   reportModelExtraPOToRow(po.ModelExtra),
		CreatedAt:    po.CreatedAt,
	}
	if po.Model != nil {
		row.Model = evaluationreadmodel.ModelIdentityRow{
			Kind:            po.Model.Kind,
			SubKind:         po.Model.SubKind,
			Algorithm:       po.Model.Algorithm,
			Code:            po.Model.Code,
			Version:         po.Model.Version,
			Title:           po.Model.Title,
			ProductChannel:  po.Model.ProductChannel,
			AlgorithmFamily: po.Model.AlgorithmFamily,
		}
	}
	if po.PrimaryScore != nil {
		row.PrimaryScore = &evaluationreadmodel.ScoreValueRow{
			Kind:  po.PrimaryScore.Kind,
			Value: po.PrimaryScore.Value,
			Label: po.PrimaryScore.Label,
			Max:   po.PrimaryScore.Max,
		}
	}
	if po.Level != nil {
		row.Level = &evaluationreadmodel.ResultLevelRow{
			Code:     po.Level.Code,
			Label:    po.Level.Label,
			Severity: po.Level.Severity,
		}
	}
	return row
}

func reportModelExtraPOToRow(po *ModelExtraPO) *evaluationreadmodel.ReportModelExtraRow {
	if po == nil {
		return nil
	}
	row := &evaluationreadmodel.ReportModelExtraRow{
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
		row.Rarity = &evaluationreadmodel.ReportModelRarityRow{
			Percent: po.Rarity.Percent,
			Label:   po.Rarity.Label,
			OneInX:  po.Rarity.OneInX,
		}
	}
	return row
}
