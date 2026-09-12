package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
	"strconv"
	"time"
)

type activityStart struct {
	ID               uint64    `json:"id" bson:"id"`
	StartedAt        time.Time `json:"started_at" bson:"started_at"`
	StoreID          *uint64   `json:"conducting_store_id" bson:"conducting_store_id"`
	OwnershipVersion uint32    `json:"ownership_version" bson:"ownership_version"`
	Version          uint32    `json:"version" bson:"version"`
}

func activityFact(org int64, kind string, id uint64, at time.Time, start *activityStart) (factCandidate, error) {
	if org <= 0 || id == 0 || at.IsZero() {
		return factCandidate{}, fmt.Errorf("incomplete store activity source")
	}
	reason := string(sheet.StartLegacy)
	var storeID *uint64
	if start != nil {
		value, err := sheet.RestoreStartContext(start.ID, start.StartedAt, start.StoreID, start.OwnershipVersion, start.Version)
		if err != nil {
			return factCandidate{}, err
		}
		storeID = value.StoreID()
		reason = ""
		if storeID == nil {
			reason = string(sheet.StartUnassigned)
		}
	}
	fact := baseFact(org, fmt.Sprintf("store_activity:%d:%s:%d", org, kind, id), kind, at, kind, strconv.FormatUint(id, 10))
	fact["business_object_id"] = id
	fact["conducting_store_id"] = storeID
	fact["unknown_reason"] = reason
	return factCandidate{SourceID: id, FactType: kind, Values: fact}, nil
}

type StoreActivityCollector struct {
	db     *gorm.DB
	mongo  *mongo.Database
	writer factWriter
}

func NewStoreActivityCollector(db *gorm.DB, mongoDB *mongo.Database) *StoreActivityCollector {
	return &StoreActivityCollector{db: db, mongo: mongoDB, writer: factWriter{db}}
}
func (*StoreActivityCollector) Name() string { return "store_activity" }
func (c *StoreActivityCollector) Collect(ctx context.Context, req domain.CollectRequest) (domain.CollectResult, error) {
	result := domain.CollectResult{Collector: c.Name(), FactTypeCounts: map[string]int64{}}
	if err := req.Window.Validate(); err != nil {
		return result, err
	}
	if req.OrgID <= 0 || c.db == nil || c.mongo == nil {
		return result, fmt.Errorf("store activity dependencies unavailable")
	}
	cursor, err := c.mongo.Collection("answersheets").Find(ctx, bson.M{"org_id": uint64(req.OrgID), "deleted_at": nil, "filled_at": bson.M{"$gte": req.Window.From, "$lt": req.Window.To}}, options.Find().SetProjection(bson.M{"domain_id": 1, "filled_at": 1, "start_context": 1}).SetSort(bson.D{{Key: "filled_at", Value: 1}, {Key: "domain_id", Value: 1}}).SetBatchSize(collectorBatchSize))
	if err != nil {
		return result, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	batch := make([]factCandidate, 0, collectorBatchSize)
	flush := func() error {
		err := writeFactCandidates(ctx, c.writer, "statistics_store_activity_fact", batch, req.Mode == domain.CollectModeValidate, &result)
		batch = batch[:0]
		return err
	}
	for cursor.Next(ctx) {
		var row struct {
			ID       uint64         `bson:"domain_id"`
			FilledAt time.Time      `bson:"filled_at"`
			Start    *activityStart `bson:"start_context"`
		}
		if err := cursor.Decode(&row); err != nil {
			return result, err
		}
		fact, err := activityFact(req.OrgID, "answersheet_submitted", row.ID, row.FilledAt, row.Start)
		if err != nil {
			return result, fmt.Errorf("answer sheet %d: %w", row.ID, err)
		}
		batch = append(batch, fact)
		if len(batch) == collectorBatchSize {
			if err := flush(); err != nil {
				return result, err
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return result, err
	}
	if err := flush(); err != nil {
		return result, err
	}
	type outcomeRow struct {
		ID, AssessmentID  uint64
		EvaluatedAt       time.Time
		ConductingContext *string
	}
	var rows []outcomeRow
	err = scanStableBatches(req.Window.From, &rows, func(at time.Time, id uint64) error {
		return c.db.WithContext(ctx).Table("evaluation_outcome o").Select("o.id,o.assessment_id,o.evaluated_at,a.conducting_context").Joins("JOIN assessment a ON a.id=o.assessment_id AND a.org_id=o.org_id").Where("o.org_id=? AND o.evaluated_at>=? AND o.evaluated_at<?", req.OrgID, req.Window.From, req.Window.To).Where("(o.evaluated_at>? OR (o.evaluated_at=? AND o.id>?))", at, at, id).Order("o.evaluated_at,o.id").Limit(collectorBatchSize).Scan(&rows).Error
	}, func(row outcomeRow) (time.Time, uint64) { return row.EvaluatedAt, row.ID }, func(rows []outcomeRow) error {
		for _, row := range rows {
			var start *activityStart
			if row.ConductingContext != nil {
				start = &activityStart{}
				if err := json.Unmarshal([]byte(*row.ConductingContext), start); err != nil {
					return err
				}
			}
			fact, err := activityFact(req.OrgID, "assessment_completed", row.AssessmentID, row.EvaluatedAt, start)
			if err != nil {
				return fmt.Errorf("assessment %d: %w", row.AssessmentID, err)
			}
			batch = append(batch, fact)
		}
		return flush()
	})
	return result, err
}

type StoreActivityDailyProjection struct{ db *gorm.DB }

func (*StoreActivityDailyProjection) Name() string { return "store_activity_daily" }
func (p *StoreActivityDailyProjection) Project(ctx context.Context, r domain.ProjectionRequest) (domain.ProjectionResult, error) {
	rows, err := replaceWindow(ctx, p.db, "statistics_store_activity_daily", "stat_date", `
 INSERT INTO statistics_store_activity_daily(org_id,stat_date,conducting_store_id,unknown_reason,answersheet_submitted_count,assessment_completed_count)
 SELECT org_id,stat_date,COALESCE(conducting_store_id,0),unknown_reason,SUM(fact_type='answersheet_submitted'),SUM(fact_type='assessment_completed')
 FROM statistics_store_activity_fact WHERE org_id=? AND stat_date>=? AND stat_date<?
 GROUP BY org_id,stat_date,COALESCE(conducting_store_id,0),unknown_reason`, r, r.OrgID, r.Window.From, r.Window.To)
	return domain.ProjectionResult{Name: p.Name(), Rows: rows}, err
}
