package evaluation

import (
	"context"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type reportReadModel struct {
	base.BaseRepository
}

func NewReportReadModel(db *mongo.Database, opts ...base.BaseRepositoryOptions) evaluationreadmodel.ReportReader {
	return &reportReadModel{
		BaseRepository: base.NewBaseRepository(db, (&InterpretReportPO{}).CollectionName(), opts...),
	}
}

func (r *reportReadModel) GetReportByID(ctx context.Context, reportID uint64) (*evaluationreadmodel.ReportRow, error) {
	return r.getReport(ctx, bson.M{
		"domain_id":  reportID,
		"deleted_at": nil,
	})
}

func (r *reportReadModel) GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*evaluationreadmodel.ReportRow, error) {
	return r.GetReportByID(ctx, assessmentID)
}

func (r *reportReadModel) ListReports(
	ctx context.Context,
	filter evaluationreadmodel.ReportFilter,
	page evaluationreadmodel.PageRequest,
) ([]evaluationreadmodel.ReportRow, int64, error) {
	query := buildReportReadModelQuery(filter)
	total, err := r.CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	findOptions := buildReportReadModelFindOptions(page)
	cursor, err := r.Find(ctx, query, findOptions)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	rows := make([]evaluationreadmodel.ReportRow, 0)
	for cursor.Next(ctx) {
		var po InterpretReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, 0, err
		}
		rows = append(rows, reportPOToReadRow(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func buildReportReadModelQuery(filter evaluationreadmodel.ReportFilter) bson.M {
	query := bson.M{"deleted_at": nil}
	if filter.TesteeID != nil {
		query["testee_id"] = *filter.TesteeID
	}
	if len(filter.TesteeIDs) > 0 {
		query["testee_id"] = bson.M{"$in": filter.TesteeIDs}
	}
	if filter.HighRiskOnly {
		query["risk_level"] = bson.M{"$in": []string{"high", "severe"}}
	}
	if filter.ScaleCode != "" {
		query["scale_code"] = filter.ScaleCode
	}
	if filter.RiskLevel != nil {
		query["risk_level"] = *filter.RiskLevel
	}
	return query
}

func buildReportReadModelFindOptions(page evaluationreadmodel.PageRequest) *options.FindOptions {
	return options.Find().
		SetSkip(int64(page.Offset())).
		SetLimit(int64(page.Limit())).
		SetSort(bson.M{"created_at": -1})
}

func (r *reportReadModel) getReport(ctx context.Context, filter bson.M) (*evaluationreadmodel.ReportRow, error) {
	var po InterpretReportPO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, cberrors.WithCode(code.ErrInterpretReportNotFound, "report not found")
		}
		return nil, err
	}
	row := reportPOToReadRow(&po)
	return &row, nil
}

func reportPOToReadRow(po *InterpretReportPO) evaluationreadmodel.ReportRow {
	if po == nil {
		return evaluationreadmodel.ReportRow{}
	}
	dimensions := make([]evaluationreadmodel.ReportDimensionRow, 0, len(po.Dimensions))
	for _, d := range po.Dimensions {
		dimensions = append(dimensions, evaluationreadmodel.ReportDimensionRow{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    d.MaxScore,
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
			Suggestion:  d.Suggestion,
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
	return evaluationreadmodel.ReportRow{
		AssessmentID: po.DomainID.Uint64(),
		ScaleName:    po.ScaleName,
		ScaleCode:    po.ScaleCode,
		TotalScore:   po.TotalScore,
		RiskLevel:    po.RiskLevel,
		Conclusion:   po.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		CreatedAt:    po.CreatedAt,
	}
}
