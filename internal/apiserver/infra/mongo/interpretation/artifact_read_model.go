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

// reportReadModel is a migration facade. New immutable artifacts win; legacy
// lifecycle-bearing documents supply only the missing compatibility rows.
type reportReadModel struct {
	legacy    *legacyReportReadModel
	artifacts base.BaseRepository
}

func NewReportReadModel(db *mongo.Database, opts ...base.BaseRepositoryOptions) evaluationreadmodel.ReportReader {
	return NewReportReadModelWithLegacyFallback(db, true, opts...)
}

// NewReportReadModelWithLegacyFallback keeps cutover explicit: production
// starts new-first with legacy fallback, then reconciliation can switch the
// same read model to artifact-only without changing query call sites.
func NewReportReadModelWithLegacyFallback(db *mongo.Database, legacyFallback bool, opts ...base.BaseRepositoryOptions) evaluationreadmodel.ReportReader {
	var legacy *legacyReportReadModel
	if legacyFallback {
		legacy = newLegacyReportReadModel(db, opts...)
	}
	return &reportReadModel{
		legacy:    legacy,
		artifacts: base.NewBaseRepository(db, (InterpretReportArtifactPO{}).CollectionName(), opts...),
	}
}

func (r *reportReadModel) GetReportByID(ctx context.Context, reportID uint64) (*evaluationreadmodel.ReportRow, error) {
	row, err := r.findArtifact(ctx, bson.M{"domain_id": reportID, "deleted_at": nil}, nil)
	if err != nil {
		return nil, err
	}
	if row != nil {
		return row, nil
	}
	if r.legacy != nil {
		return r.legacy.GetReportByID(ctx, reportID)
	}
	return nil, mongo.ErrNoDocuments
}

func (r *reportReadModel) GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*evaluationreadmodel.ReportRow, error) {
	row, err := r.findArtifact(ctx, bson.M{"assessment_id": assessmentID, "deleted_at": nil}, options.Find().SetSort(bson.D{{Key: "generated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	if row != nil {
		return row, nil
	}
	if r.legacy != nil {
		return r.legacy.GetReportByAssessmentID(ctx, assessmentID)
	}
	return nil, mongo.ErrNoDocuments
}

func (r *reportReadModel) ListReports(ctx context.Context, filter evaluationreadmodel.ReportFilter, page evaluationreadmodel.PageRequest) ([]evaluationreadmodel.ReportRow, int64, error) {
	artifacts, err := r.listArtifacts(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	legacy := []evaluationreadmodel.ReportRow(nil)
	if r.legacy != nil {
		var err error
		legacy, err = r.listLegacy(ctx, filter)
		if err != nil {
			return nil, 0, err
		}
	}
	merged := mergeNewFirstReportRows(artifacts, legacy)
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

func (r *reportReadModel) findArtifact(ctx context.Context, filter bson.M, opts *options.FindOptions) (*evaluationreadmodel.ReportRow, error) {
	if opts == nil {
		opts = options.Find()
	}
	opts.SetLimit(1)
	cursor, err := r.artifacts.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	if !cursor.Next(ctx) {
		return nil, cursor.Err()
	}
	var po InterpretReportArtifactPO
	if err := cursor.Decode(&po); err != nil {
		return nil, err
	}
	row := artifactPOToReadRow(&po)
	return &row, nil
}

func (r *reportReadModel) listArtifacts(ctx context.Context, filter evaluationreadmodel.ReportFilter) ([]evaluationreadmodel.ReportRow, error) {
	cursor, err := r.artifacts.Find(ctx, buildArtifactReadModelQuery(filter), options.Find().SetSort(bson.D{{Key: "generated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	rows := make([]evaluationreadmodel.ReportRow, 0)
	for cursor.Next(ctx) {
		var po InterpretReportArtifactPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		rows = append(rows, artifactPOToReadRow(&po))
	}
	return rows, cursor.Err()
}

func (r *reportReadModel) listLegacy(ctx context.Context, filter evaluationreadmodel.ReportFilter) ([]evaluationreadmodel.ReportRow, error) {
	cursor, err := r.legacy.Find(ctx, buildReportReadModelQuery(filter), options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
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
		rows = append(rows, reportPOToReadRow(&po))
	}
	return rows, cursor.Err()
}

func buildArtifactReadModelQuery(filter evaluationreadmodel.ReportFilter) bson.M {
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

func artifactPOToReadRow(po *InterpretReportArtifactPO) evaluationreadmodel.ReportRow {
	if po == nil {
		return evaluationreadmodel.ReportRow{}
	}
	legacyShape := &InterpretReportPO{
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
	return reportPOToReadRow(legacyShape)
}

// mergeNewFirstReportRows maintains legacy ReportReader semantics while v2
// can hold several report variants per Assessment: the newest artifact wins,
// then the legacy row is used only when no v2 artifact exists.
func mergeNewFirstReportRows(artifacts, legacy []evaluationreadmodel.ReportRow) []evaluationreadmodel.ReportRow {
	byAssessment := make(map[uint64]evaluationreadmodel.ReportRow, len(artifacts)+len(legacy))
	for _, row := range artifacts {
		if current, ok := byAssessment[row.AssessmentID]; !ok || row.CreatedAt.After(current.CreatedAt) {
			byAssessment[row.AssessmentID] = row
		}
	}
	for _, row := range legacy {
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
