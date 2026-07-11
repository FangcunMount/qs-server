package interpretation

import (
	"context"
	"sort"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// reportReadModel reads current reports and immutable historical archives.
type reportReadModel struct {
	reports  base.BaseRepository
	archives base.BaseRepository
}

func NewReportReadModel(db *mongo.Database, opts ...base.BaseRepositoryOptions) evaluationreadmodel.ReportReader {
	return &reportReadModel{
		reports:  base.NewBaseRepository(db, (InterpretReportPO{}).CollectionName(), opts...),
		archives: base.NewBaseRepository(db, "archived_reports", opts...),
	}
}

func (r *reportReadModel) GetReportByID(ctx context.Context, reportID uint64) (*evaluationreadmodel.ReportRow, error) {
	row, err := r.findReport(ctx, bson.M{"domain_id": reportID, "deleted_at": nil}, nil)
	if err != nil {
		return nil, err
	}
	if row != nil {
		return row, nil
	}
	if row, err := r.findArchive(ctx, bson.M{"domain_id": reportID, "deleted_at": nil}, nil); err != nil || row != nil {
		return row, err
	}
	return nil, mongo.ErrNoDocuments
}

func (r *reportReadModel) GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*evaluationreadmodel.ReportRow, error) {
	row, err := r.findReport(ctx, bson.M{"assessment_id": assessmentID, "deleted_at": nil}, options.Find().SetSort(bson.D{{Key: "generated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	if row != nil {
		return row, nil
	}
	if row, err := r.findArchive(ctx, bson.M{"domain_id": assessmentID, "deleted_at": nil}, nil); err != nil || row != nil {
		return row, err
	}
	return nil, mongo.ErrNoDocuments
}

func (r *reportReadModel) ListReports(ctx context.Context, filter evaluationreadmodel.ReportFilter, page evaluationreadmodel.PageRequest) ([]evaluationreadmodel.ReportRow, int64, error) {
	reports, err := r.listReportsFromStore(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	archives, err := r.listArchives(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	merged := mergeCurrentAndArchivedReportRows(reports, archives)
	total := int64(len(merged))
	offset, limit := page.Offset(), page.Limit()
	if offset >= len(merged) {
		return []evaluationreadmodel.ReportRow{}, total, nil
	}
	end := offset + limit
	if end > len(merged) {
		end = len(merged)
	}
	return merged[offset:end], total, nil
}

func (r *reportReadModel) findArchive(ctx context.Context, filter bson.M, opts *options.FindOptions) (*evaluationreadmodel.ReportRow, error) {
	if opts == nil {
		opts = options.Find()
	}
	opts.SetLimit(1)
	cursor, err := r.archives.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	if !cursor.Next(ctx) {
		return nil, cursor.Err()
	}
	var po ArchivedReportPO
	if err := cursor.Decode(&po); err != nil {
		return nil, err
	}
	row := archivedReportPOToReadRow(&po)
	return &row, nil
}

func (r *reportReadModel) listArchives(ctx context.Context, filter evaluationreadmodel.ReportFilter) ([]evaluationreadmodel.ReportRow, error) {
	cursor, err := r.archives.Find(ctx, buildReportReadModelQuery(filter), options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	rows := make([]evaluationreadmodel.ReportRow, 0)
	for cursor.Next(ctx) {
		var po ArchivedReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		rows = append(rows, archivedReportPOToReadRow(&po))
	}
	return rows, cursor.Err()
}

func (r *reportReadModel) findReport(ctx context.Context, filter bson.M, opts *options.FindOptions) (*evaluationreadmodel.ReportRow, error) {
	if opts == nil {
		opts = options.Find()
	}
	opts.SetLimit(1)
	cursor, err := r.reports.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	if !cursor.Next(ctx) {
		return nil, cursor.Err()
	}
	var po InterpretReportPO
	if err := cursor.Decode(&po); err != nil {
		return nil, err
	}
	row := interpretReportPOToReadRow(&po)
	return &row, nil
}

func (r *reportReadModel) listReportsFromStore(ctx context.Context, filter evaluationreadmodel.ReportFilter) ([]evaluationreadmodel.ReportRow, error) {
	cursor, err := r.reports.Find(ctx, buildInterpretReportReadModelQuery(filter), options.Find().SetSort(bson.D{{Key: "generated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	rows := make([]evaluationreadmodel.ReportRow, 0)
	for cursor.Next(ctx) {
		var po InterpretReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		rows = append(rows, interpretReportPOToReadRow(&po))
	}
	return rows, cursor.Err()
}

func buildInterpretReportReadModelQuery(filter evaluationreadmodel.ReportFilter) bson.M {
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
	if filter.ModelCode != "" {
		query["scale_code"] = filter.ModelCode
	}
	if filter.RiskLevel != nil {
		query["risk_level"] = *filter.RiskLevel
	}
	return query
}

func interpretReportPOToReadRow(po *InterpretReportPO) evaluationreadmodel.ReportRow {
	if po == nil {
		return evaluationreadmodel.ReportRow{}
	}
	archivedShape := &ArchivedReportPO{
		BaseDocument: base.BaseDocument{DomainID: meta.FromUint64(po.AssessmentID), CreatedAt: po.GeneratedAt},
		ScaleName:    po.ScaleName,
		ScaleCode:    po.ScaleCode,
		Model:        po.Model,
		PrimaryScore: po.PrimaryScore,
		Level:        po.Level,
		TotalScore:   po.TotalScore,
		RiskLevel:    po.RiskLevel,
		Conclusion:   po.Conclusion,
		Dimensions:   po.Dimensions,
		Suggestions:  po.Suggestions,
		ModelExtra:   po.ModelExtra,
	}
	return archivedReportPOToReadRow(archivedShape)
}

// mergeCurrentAndArchivedReportRows preserves ReportReader's assessment-level
// query semantics: the newest current report wins, and an archived report is
// used only when no current report exists for that Assessment.
func mergeCurrentAndArchivedReportRows(reports, archives []evaluationreadmodel.ReportRow) []evaluationreadmodel.ReportRow {
	byAssessment := make(map[uint64]evaluationreadmodel.ReportRow, len(reports)+len(archives))
	for _, row := range reports {
		if current, ok := byAssessment[row.AssessmentID]; !ok || row.CreatedAt.After(current.CreatedAt) {
			byAssessment[row.AssessmentID] = row
		}
	}
	for _, row := range archives {
		if _, exists := byAssessment[row.AssessmentID]; !exists {
			byAssessment[row.AssessmentID] = row
		}
	}
	merged := make([]evaluationreadmodel.ReportRow, 0, len(byAssessment))
	for _, row := range byAssessment {
		merged = append(merged, row)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].CreatedAt.After(merged[j].CreatedAt) })
	return merged
}
