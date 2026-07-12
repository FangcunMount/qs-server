package statistics

import (
	"context"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

const reportCatalogCollection = "report_query_catalog"

type reportScanSource struct {
	mysql *gorm.DB
	mongo *mongo.Database
}

// NewReportScanSource builds a report scan source from MySQL assessments and
// Interpretation's assessment-level report catalog.
func NewReportScanSource(mysqlDB *gorm.DB, mongoDB *mongo.Database) statisticsApp.ReportScanSource {
	if mysqlDB == nil {
		return nil
	}
	return &reportScanSource{mysql: mysqlDB, mongo: mongoDB}
}

func (s *reportScanSource) ListReportGeneratedFacts(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
) ([]domainStatistics.ReportGeneratedFact, error) {
	if s == nil || s.mysql == nil || limit <= 0 {
		return nil, nil
	}
	query := s.mysql.WithContext(ctx).
		Model(&evaluationInfra.AssessmentPO{}).
		Select("id, org_id, testee_id, evaluated_at, created_at").
		Where("org_id = ? AND deleted_at IS NULL AND status = ? AND evaluated_at IS NOT NULL", orgID, "evaluated")
	if !sinceTime.IsZero() {
		query = query.Where("(id > ? OR evaluated_at > ?)", sinceID, sinceTime)
	}
	var rows []evaluationInfra.AssessmentPO
	if err := query.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	assessmentIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		assessmentIDs = append(assessmentIDs, row.ID.Uint64())
	}
	reportMeta, err := s.loadReportMeta(ctx, assessmentIDs)
	if err != nil {
		return nil, err
	}

	facts := make([]domainStatistics.ReportGeneratedFact, 0, len(rows))
	for _, row := range rows {
		assessmentID := row.ID.Uint64()
		metaRow, ok := reportMeta[assessmentID]
		if !ok {
			continue
		}
		occurredAt := metaRow.createdAt
		if occurredAt.IsZero() && row.EvaluatedAt != nil {
			occurredAt = *row.EvaluatedAt
		}
		facts = append(facts, domainStatistics.ReportGeneratedFact{
			OrgID:        row.OrgID,
			TesteeID:     row.TesteeID,
			AssessmentID: assessmentID,
			ReportID:     metaRow.reportID,
			OccurredAt:   occurredAt,
		})
	}
	return facts, nil
}

type reportMeta struct {
	reportID  uint64
	createdAt time.Time
}

func (s *reportScanSource) loadReportMeta(ctx context.Context, assessmentIDs []uint64) (map[uint64]reportMeta, error) {
	result := make(map[uint64]reportMeta, len(assessmentIDs))
	if s.mongo == nil || len(assessmentIDs) == 0 {
		return result, nil
	}
	cursor, err := s.mongo.Collection(reportCatalogCollection).Find(ctx, bson.M{
		"assessment_id": bson.M{"$in": assessmentIDs},
	}, options.Find().SetProjection(bson.M{
		"assessment_id": 1,
		"source_id":     1,
		"sort_at":       1,
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	for cursor.Next(ctx) {
		var row struct {
			AssessmentID uint64    `bson:"assessment_id"`
			SourceID     uint64    `bson:"source_id"`
			SortAt       time.Time `bson:"sort_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.AssessmentID] = reportMeta{reportID: row.SourceID, createdAt: row.SortAt}
	}
	return result, cursor.Err()
}
