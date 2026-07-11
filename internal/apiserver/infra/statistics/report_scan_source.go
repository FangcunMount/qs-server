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

const (
	currentReportsCollection  = "interpret_report_artifacts"
	archivedReportsCollection = "archived_reports"
)

type reportScanSource struct {
	mysql *gorm.DB
	mongo *mongo.Database
}

// NewReportScanSource builds a report scan source from MySQL assessments and
// Interpretation's current reports plus the immutable historical archive.
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
	if err := s.loadCurrentReportMeta(ctx, assessmentIDs, result); err != nil {
		return nil, err
	}
	if len(result) == len(assessmentIDs) {
		return result, nil
	}
	return result, s.loadArchivedReportMeta(ctx, assessmentIDs, result)
}

func (s *reportScanSource) loadCurrentReportMeta(ctx context.Context, assessmentIDs []uint64, result map[uint64]reportMeta) error {
	cursor, err := s.mongo.Collection(currentReportsCollection).Find(ctx, bson.M{
		"assessment_id": bson.M{"$in": assessmentIDs},
	}, options.Find().SetProjection(bson.M{
		"domain_id":     1,
		"assessment_id": 1,
		"generated_at":  1,
		"created_at":    1,
	}).SetSort(bson.D{{Key: "generated_at", Value: -1}}))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()

	for cursor.Next(ctx) {
		var row struct {
			DomainID     meta.ID   `bson:"domain_id"`
			AssessmentID uint64    `bson:"assessment_id"`
			GeneratedAt  time.Time `bson:"generated_at"`
			CreatedAt    time.Time `bson:"created_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		if _, exists := result[row.AssessmentID]; exists {
			continue
		}
		occurredAt := row.GeneratedAt
		if occurredAt.IsZero() {
			occurredAt = row.CreatedAt
		}
		result[row.AssessmentID] = reportMeta{reportID: row.DomainID.Uint64(), createdAt: occurredAt}
	}
	return cursor.Err()
}

func (s *reportScanSource) loadArchivedReportMeta(ctx context.Context, assessmentIDs []uint64, result map[uint64]reportMeta) error {
	domainIDs := make([]meta.ID, 0, len(assessmentIDs))
	for _, id := range assessmentIDs {
		if _, exists := result[id]; exists {
			continue
		}
		domainIDs = append(domainIDs, meta.FromUint64(id))
	}
	if len(domainIDs) == 0 {
		return nil
	}
	cursor, err := s.mongo.Collection(archivedReportsCollection).Find(ctx, bson.M{
		"deleted_at": nil,
		"domain_id":  bson.M{"$in": domainIDs},
	}, options.Find().SetProjection(bson.M{
		"domain_id":  1,
		"created_at": 1,
	}))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()

	for cursor.Next(ctx) {
		var row struct {
			DomainID  meta.ID   `bson:"domain_id"`
			CreatedAt time.Time `bson:"created_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		assessmentID := row.DomainID.Uint64()
		result[assessmentID] = reportMeta{
			reportID:  assessmentID,
			createdAt: row.CreatedAt,
		}
	}
	return cursor.Err()
}
