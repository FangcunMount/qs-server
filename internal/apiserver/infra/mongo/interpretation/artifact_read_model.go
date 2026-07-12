package interpretation

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	readmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// reportReadModel resolves assessment-level report queries through the compact
// report catalog and loads report bodies only for the requested page.
type reportReadModel struct {
	reports, archives, catalog base.BaseRepository
}

func NewReportReadModel(db *mongo.Database, opts ...base.BaseRepositoryOptions) readmodel.ReportReader {
	return &reportReadModel{
		reports:  base.NewBaseRepository(db, (InterpretReportPO{}).CollectionName(), opts...),
		archives: base.NewBaseRepository(db, (ArchivedReportPO{}).CollectionName(), opts...),
		catalog:  base.NewBaseRepository(db, (ReportCatalogPO{}).CollectionName(), opts...),
	}
}

func (r *reportReadModel) GetReportByID(ctx context.Context, reportID uint64) (*readmodel.ReportRow, error) {
	var po InterpretReportPO
	if err := r.reports.FindOne(ctx, bson.M{"domain_id": reportID, "deleted_at": nil}, &po); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, readmodel.ErrReportNotFound
		}
		return nil, err
	}
	row := interpretReportPOToReadRow(&po)
	return &row, nil
}

func (r *reportReadModel) GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*readmodel.ReportRow, error) {
	var entry ReportCatalogPO
	if err := r.catalog.FindOne(ctx, bson.M{"assessment_id": assessmentID}, &entry); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, readmodel.ErrReportNotFound
		}
		return nil, err
	}
	rows, err := r.loadCatalogRows(ctx, []ReportCatalogPO{entry})
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (r *reportReadModel) ListReports(ctx context.Context, filter readmodel.ReportFilter, page readmodel.PageRequest) ([]readmodel.ReportRow, int64, error) {
	query := buildCatalogQuery(filter)
	total, err := r.catalog.CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	cur, err := r.catalog.Find(ctx, query, options.Find().SetSort(bson.D{{Key: "sort_at", Value: -1}, {Key: "sort_report_id", Value: -1}, {Key: "assessment_id", Value: -1}}).SetSkip(int64(page.Offset())).SetLimit(int64(page.Limit())))
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = cur.Close(ctx) }()
	entries := make([]ReportCatalogPO, 0, page.Limit())
	for cur.Next(ctx) {
		var entry ReportCatalogPO
		if err := cur.Decode(&entry); err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}
	rows, err := r.loadCatalogRows(ctx, entries)
	return rows, total, err
}

func buildCatalogQuery(filter readmodel.ReportFilter) bson.M {
	q := bson.M{}
	if filter.OrgID != nil {
		q["org_id"] = *filter.OrgID
	}
	if filter.TesteeID != nil {
		q["testee_id"] = *filter.TesteeID
	}
	if len(filter.TesteeIDs) > 0 {
		q["testee_id"] = bson.M{"$in": filter.TesteeIDs}
	}
	if filter.ModelCode != "" {
		q["model_code"] = filter.ModelCode
	}
	if filter.RiskLevel != nil {
		q["risk_level"] = *filter.RiskLevel
	} else if filter.HighRiskOnly {
		q["risk_level"] = bson.M{"$in": []string{"high", "severe"}}
	}
	return q
}

func (r *reportReadModel) loadCatalogRows(ctx context.Context, entries []ReportCatalogPO) ([]readmodel.ReportRow, error) {
	if len(entries) == 0 {
		return []readmodel.ReportRow{}, nil
	}
	artifactIDs, archiveIDs := make([]uint64, 0, len(entries)), make([]uint64, 0, len(entries))
	for _, e := range entries {
		switch e.SourceKind {
		case ReportCatalogSourceArtifact:
			artifactIDs = append(artifactIDs, e.SourceID)
		case ReportCatalogSourceArchive:
			archiveIDs = append(archiveIDs, e.SourceID)
		default:
			return nil, fmt.Errorf("unknown report catalog source %q", e.SourceKind)
		}
	}
	byKey := map[string]readmodel.ReportRow{}
	if err := r.loadArtifacts(ctx, artifactIDs, byKey); err != nil {
		return nil, err
	}
	if err := r.loadArchives(ctx, archiveIDs, byKey); err != nil {
		return nil, err
	}
	rows := make([]readmodel.ReportRow, 0, len(entries))
	for _, e := range entries {
		key := fmt.Sprintf("%s:%d", e.SourceKind, e.SourceID)
		row, ok := byKey[key]
		if !ok {
			logger.L(ctx).Errorw("report catalog points to a missing source",
				"assessment_id", e.AssessmentID,
				"source_kind", e.SourceKind,
				"source_id", e.SourceID,
			)
			return nil, &readmodel.CatalogDanglingSourceError{AssessmentID: e.AssessmentID, SourceKind: e.SourceKind, SourceID: e.SourceID}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *reportReadModel) loadArtifacts(ctx context.Context, ids []uint64, dst map[string]readmodel.ReportRow) error {
	if len(ids) == 0 {
		return nil
	}
	cur, err := r.reports.Find(ctx, bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		var po InterpretReportPO
		if err := cur.Decode(&po); err != nil {
			return err
		}
		dst[fmt.Sprintf("%s:%d", ReportCatalogSourceArtifact, po.DomainID.Uint64())] = interpretReportPOToReadRow(&po)
	}
	return cur.Err()
}
func (r *reportReadModel) loadArchives(ctx context.Context, ids []uint64, dst map[string]readmodel.ReportRow) error {
	if len(ids) == 0 {
		return nil
	}
	cur, err := r.archives.Find(ctx, bson.M{"domain_id": bson.M{"$in": ids}, "deleted_at": nil})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		var po ArchivedReportPO
		if err := cur.Decode(&po); err != nil {
			return err
		}
		dst[fmt.Sprintf("%s:%d", ReportCatalogSourceArchive, po.DomainID.Uint64())] = projectArchivedReportRow(&po)
	}
	return cur.Err()
}

func interpretReportPOToReadRow(po *InterpretReportPO) readmodel.ReportRow {
	if po == nil {
		return readmodel.ReportRow{}
	}
	archived := &ArchivedReportPO{BaseDocument: base.BaseDocument{DomainID: meta.FromUint64(po.AssessmentID), CreatedAt: po.GeneratedAt}, ScaleName: po.ScaleName, ScaleCode: po.ScaleCode, Model: po.Model, PrimaryScore: po.PrimaryScore, Level: po.Level, TotalScore: po.TotalScore, RiskLevel: po.RiskLevel, Conclusion: po.Conclusion, Dimensions: po.Dimensions, Suggestions: po.Suggestions, ModelExtra: po.ModelExtra}
	return projectArchivedReportRow(archived)
}
