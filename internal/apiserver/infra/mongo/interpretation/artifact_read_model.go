package interpretation

import (
	"context"
	"fmt"
	"time"

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

func (r *reportReadModel) GetCurrentReportMetadataByAssessmentIDs(ctx context.Context, assessmentIDs []uint64) (map[uint64]readmodel.CurrentReportMetadata, error) {
	result := make(map[uint64]readmodel.CurrentReportMetadata, len(assessmentIDs))
	uniqueIDs := make([]uint64, 0, len(assessmentIDs))
	for _, assessmentID := range assessmentIDs {
		if assessmentID == 0 {
			continue
		}
		if _, exists := result[assessmentID]; exists {
			continue
		}
		result[assessmentID] = readmodel.CurrentReportMetadata{AssessmentID: assessmentID, Status: readmodel.CurrentReportMetadataMissing}
		uniqueIDs = append(uniqueIDs, assessmentID)
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	cursor, err := r.catalog.Find(
		ctx,
		bson.M{"assessment_id": bson.M{"$in": uniqueIDs}},
		options.Find().SetProjection(bson.M{
			"assessment_id": 1,
			"org_id":        1,
			"testee_id":     1,
			"source_kind":   1,
			"source_id":     1,
		}),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	entries := make([]ReportCatalogPO, 0, len(uniqueIDs))
	for cursor.Next(ctx) {
		var entry ReportCatalogPO
		if err := cursor.Decode(&entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}

	sources, err := r.loadCatalogSourceMetadata(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		item := readmodel.CurrentReportMetadata{
			AssessmentID: entry.AssessmentID,
			Status:       readmodel.CurrentReportMetadataDangling,
			SourceKind:   entry.SourceKind,
			SourceID:     entry.SourceID,
		}
		source, found := sources[fmt.Sprintf("%s:%d", entry.SourceKind, entry.SourceID)]
		if found {
			item.CreatedAt = source.CreatedAt
			if fields := MismatchedAssociationFields(entry, source.Association); len(fields) > 0 {
				item.Status = readmodel.CurrentReportMetadataMismatch
				item.MismatchedFields = fields
			} else {
				item.Status = readmodel.CurrentReportMetadataFound
			}
		}
		result[entry.AssessmentID] = item
	}
	return result, nil
}

type catalogSourceMetadata struct {
	Association CatalogSourceAssociation
	CreatedAt   time.Time
}

func (r *reportReadModel) loadCatalogSourceMetadata(ctx context.Context, entries []ReportCatalogPO) (map[string]catalogSourceMetadata, error) {
	artifactIDs, archiveIDs := make([]uint64, 0, len(entries)), make([]uint64, 0, len(entries))
	for _, entry := range entries {
		switch entry.SourceKind {
		case ReportCatalogSourceArtifact:
			artifactIDs = append(artifactIDs, entry.SourceID)
		case ReportCatalogSourceArchive:
			archiveIDs = append(archiveIDs, entry.SourceID)
		}
	}
	result := make(map[string]catalogSourceMetadata, len(entries))
	if len(artifactIDs) > 0 {
		cursor, err := r.reports.Find(
			ctx,
			bson.M{"domain_id": bson.M{"$in": artifactIDs}, "deleted_at": nil},
			options.Find().SetProjection(bson.M{
				"domain_id":     1,
				"assessment_id": 1,
				"org_id":        1,
				"testee_id":     1,
				"outcome_id":    1,
				"generation_id": 1,
				"generated_at":  1,
			}),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = cursor.Close(ctx) }()
		for cursor.Next(ctx) {
			var po InterpretReportPO
			if err := cursor.Decode(&po); err != nil {
				return nil, err
			}
			result[fmt.Sprintf("%s:%d", ReportCatalogSourceArtifact, po.DomainID.Uint64())] = catalogSourceMetadata{
				Association: CatalogSourceAssociation{
					AssessmentID: po.AssessmentID, OrgID: po.OrgID, HasOrgID: true, TesteeID: po.TesteeID,
					OutcomeID: po.OutcomeID, HasOutcomeID: po.OutcomeID != 0,
					GenerationID: po.GenerationID, HasGenerationID: po.GenerationID != 0,
				},
				CreatedAt: po.GeneratedAt,
			}
		}
		if err := cursor.Err(); err != nil {
			return nil, err
		}
	}
	if len(archiveIDs) > 0 {
		cursor, err := r.archives.Find(
			ctx,
			bson.M{"domain_id": bson.M{"$in": archiveIDs}, "deleted_at": nil},
			options.Find().SetProjection(bson.M{
				"domain_id":  1,
				"org_id":     1,
				"testee_id":  1,
				"outcome_id": 1,
				"created_at": 1,
			}),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = cursor.Close(ctx) }()
		for cursor.Next(ctx) {
			var po ArchivedReportPO
			if err := cursor.Decode(&po); err != nil {
				return nil, err
			}
			association := CatalogSourceAssociation{
				AssessmentID: po.DomainID.Uint64(), TesteeID: po.TesteeID,
				OutcomeID: po.OutcomeID, HasOutcomeID: po.OutcomeID != 0,
			}
			if po.OrgID != nil {
				association.OrgID = *po.OrgID
				association.HasOrgID = true
			}
			result[fmt.Sprintf("%s:%d", ReportCatalogSourceArchive, po.DomainID.Uint64())] = catalogSourceMetadata{
				Association: association,
				CreatedAt:   po.CreatedAt,
			}
		}
		if err := cursor.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
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

// catalogSourceEnvelope carries the loaded report body together with the
// association identity used for IR-R002 catalog↔source fail-closed checks.
// Association fields are never taken from ReportRow (which omits org/testee).
type catalogSourceEnvelope struct {
	AssessmentID    uint64
	OrgID           int64
	HasOrgID        bool
	TesteeID        uint64
	OutcomeID       uint64
	HasOutcomeID    bool
	GenerationID    uint64
	HasGenerationID bool
	Row             readmodel.ReportRow
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
	byKey := map[string]catalogSourceEnvelope{}
	if err := r.loadArtifacts(ctx, artifactIDs, byKey); err != nil {
		return nil, err
	}
	if err := r.loadArchives(ctx, archiveIDs, byKey); err != nil {
		return nil, err
	}
	rows := make([]readmodel.ReportRow, 0, len(entries))
	for _, e := range entries {
		key := fmt.Sprintf("%s:%d", e.SourceKind, e.SourceID)
		env, ok := byKey[key]
		if !ok {
			logger.L(ctx).Errorw("report catalog points to a missing source",
				"assessment_id", e.AssessmentID,
				"source_kind", e.SourceKind,
				"source_id", e.SourceID,
			)
			return nil, &readmodel.CatalogDanglingSourceError{AssessmentID: e.AssessmentID, SourceKind: e.SourceKind, SourceID: e.SourceID}
		}
		if fields := mismatchedAssociationFields(e, env); len(fields) > 0 {
			observeCatalogAssociationMismatch(e.SourceKind)
			logger.L(ctx).Errorw("report catalog source association mismatch",
				"assessment_id", e.AssessmentID,
				"source_kind", e.SourceKind,
				"source_id", e.SourceID,
				"mismatched_fields", fields,
				"catalog_assessment_id", e.AssessmentID,
				"source_assessment_id", env.AssessmentID,
				"catalog_org_id", e.OrgID,
				"source_org_id", env.OrgID,
				"source_has_org_id", env.HasOrgID,
				"catalog_testee_id", e.TesteeID,
				"source_testee_id", env.TesteeID,
			)
			return nil, &readmodel.CatalogSourceAssociationMismatchError{
				AssessmentID:     e.AssessmentID,
				SourceKind:       e.SourceKind,
				SourceID:         e.SourceID,
				MismatchedFields: fields,
			}
		}
		rows = append(rows, env.Row)
	}
	return rows, nil
}

func (r *reportReadModel) loadArtifacts(ctx context.Context, ids []uint64, dst map[string]catalogSourceEnvelope) error {
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
		dst[fmt.Sprintf("%s:%d", ReportCatalogSourceArtifact, po.DomainID.Uint64())] = catalogSourceEnvelope{
			AssessmentID:    po.AssessmentID,
			OrgID:           po.OrgID,
			HasOrgID:        true,
			TesteeID:        po.TesteeID,
			OutcomeID:       po.OutcomeID,
			HasOutcomeID:    po.OutcomeID != 0,
			GenerationID:    po.GenerationID,
			HasGenerationID: po.GenerationID != 0,
			Row:             interpretReportPOToReadRow(&po),
		}
	}
	return cur.Err()
}

func (r *reportReadModel) loadArchives(ctx context.Context, ids []uint64, dst map[string]catalogSourceEnvelope) error {
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
		env := catalogSourceEnvelope{
			AssessmentID: po.DomainID.Uint64(),
			TesteeID:     po.TesteeID,
			OutcomeID:    po.OutcomeID,
			HasOutcomeID: po.OutcomeID != 0,
			Row:          projectArchivedReportRow(&po),
		}
		if po.OrgID != nil {
			env.OrgID = *po.OrgID
			env.HasOrgID = true
		}
		dst[fmt.Sprintf("%s:%d", ReportCatalogSourceArchive, po.DomainID.Uint64())] = env
	}
	return cur.Err()
}

func interpretReportPOToReadRow(po *InterpretReportPO) readmodel.ReportRow {
	if po == nil {
		return readmodel.ReportRow{}
	}
	archived := &ArchivedReportPO{BaseDocument: base.BaseDocument{DomainID: meta.FromUint64(po.AssessmentID), CreatedAt: po.GeneratedAt}, ScaleName: po.ScaleName, ScaleCode: po.ScaleCode, Model: po.Model, PrimaryScore: po.PrimaryScore, Level: po.Level, TotalScore: po.TotalScore, RiskLevel: po.RiskLevel, Conclusion: po.Conclusion, Dimensions: po.Dimensions, Suggestions: po.Suggestions, ModelExtra: po.ModelExtra, PresentationProfile: po.PresentationProfile}
	return projectArchivedReportRow(archived)
}
