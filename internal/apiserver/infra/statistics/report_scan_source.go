package statistics

import (
	"context"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

const interpretReportsCollection = "interpret_reports"

type reportScanSource struct {
	mysql *gorm.DB
	mongo *mongo.Database
}

// NewReportScanSource builds a report scan source from MySQL assessments and Mongo interpret reports.
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
	domainIDs := make([]meta.ID, 0, len(assessmentIDs))
	for _, id := range assessmentIDs {
		domainIDs = append(domainIDs, meta.FromUint64(id))
	}
	cursor, err := s.mongo.Collection(interpretReportsCollection).Find(ctx, bson.M{
		"deleted_at": nil,
		"domain_id":  bson.M{"$in": domainIDs},
	}, options.Find().SetProjection(bson.M{
		"domain_id":  1,
		"created_at": 1,
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	for cursor.Next(ctx) {
		var row struct {
			DomainID  meta.ID   `bson:"domain_id"`
			CreatedAt time.Time `bson:"created_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		reportID := row.DomainID.Uint64()
		result[reportID] = reportMeta{
			reportID:  reportID,
			createdAt: row.CreatedAt,
		}
	}
	return result, cursor.Err()
}
