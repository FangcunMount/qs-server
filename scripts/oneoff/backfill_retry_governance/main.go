// Command backfill_retry_governance classifies only the latest failed run for
// each Evaluation/Interpretation resource and stages deterministic wake-up
// events for automatic decisions. It is resumable and dry-run by default.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const retryTopic = "qs.evaluation.lifecycle"

type config struct {
	mysqlDSN, mongoURI, mongoDB string
	limit                       int
	apply                       bool
	timeout                     time.Duration
}

type evaluationCandidate struct {
	CheckpointID                            uint64
	AssessmentID                            uint64
	Attempt                                 int
	Retryable                               bool
	OrgID                                   int64
	TesteeID                                uint64
	QuestionnaireCode, QuestionnaireVersion string
	AnswerSheetID                           uint64
	ModelKind, ModelSubKind, ModelAlgorithm sql.NullString
	ModelCode, ModelVersion                 sql.NullString
}

type generationCandidate struct {
	DomainID        uint64 `bson:"domain_id"`
	OutcomeID       uint64 `bson:"outcome_id"`
	ReportType      string `bson:"report_type"`
	TemplateVersion string `bson:"template_version"`
	LatestRunID     uint64 `bson:"latest_run_id"`
}

type runCandidate struct {
	DomainID         uint64 `bson:"domain_id"`
	GenerationID     uint64 `bson:"generation_id"`
	Attempt          int    `bson:"attempt"`
	RetryDisposition string `bson:"retry_disposition"`
	RetryEventID     string `bson:"retry_event_id"`
	Failure          *struct {
		Retryable bool `bson:"retryable"`
	} `bson:"failure"`
}

type outcomeCorrelation struct {
	OrgID, AssessmentID int64
	TesteeID            uint64
}

func main() {
	cfg := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	mysqlDB, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer mysqlDB.Close()
	if err := mysqlDB.PingContext(ctx); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer mongoClient.Disconnect(context.Background())
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("ping mongo: %v", err)
	}

	evaluationCount, err := backfillEvaluation(ctx, mysqlDB, cfg)
	if err != nil {
		log.Fatalf("backfill evaluation: %v", err)
	}
	interpretationCount, err := backfillInterpretation(ctx, mysqlDB, mongoClient.Database(cfg.mongoDB), cfg)
	if err != nil {
		log.Fatalf("backfill interpretation: %v", err)
	}
	log.Printf("retry governance backfill complete apply=%v evaluation=%d interpretation=%d", cfg.apply, evaluationCount, interpretationCount)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", os.Getenv("MONGO_DB"), "MongoDB database")
	flag.IntVar(&cfg.limit, "limit", 1000, "maximum latest failures per resource type")
	flag.BoolVar(&cfg.apply, "apply", false, "persist decisions and retry events")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Minute, "overall timeout")
	flag.Parse()
	if cfg.mysqlDSN == "" || cfg.mongoURI == "" || cfg.mongoDB == "" {
		log.Fatal("--mysql-dsn, --mongo-uri and --mongo-db are required")
	}
	if cfg.limit < 1 || cfg.limit > 100000 {
		log.Fatal("--limit must be between 1 and 100000")
	}
	return cfg
}

func backfillEvaluation(ctx context.Context, db *sql.DB, cfg config) (int, error) {
	rows, err := db.QueryContext(ctx, `
SELECT rc.id, rc.assessment_id, rc.attempt_no, rc.retryable,
       a.org_id, a.testee_id, a.questionnaire_code, a.questionnaire_version, a.answer_sheet_id,
       a.evaluation_model_kind, a.evaluation_model_sub_kind, a.evaluation_model_algorithm,
       a.evaluation_model_code, a.evaluation_model_version
FROM runtime_checkpoint rc
JOIN assessment a ON a.id = rc.assessment_id AND a.deleted_at IS NULL
JOIN (SELECT assessment_id, MAX(attempt_no) attempt_no FROM runtime_checkpoint
      WHERE scope='evaluation_run' AND deleted_at IS NULL GROUP BY assessment_id) latest
  ON latest.assessment_id=rc.assessment_id AND latest.attempt_no=rc.attempt_no
WHERE rc.scope='evaluation_run' AND rc.status='failed' AND rc.deleted_at IS NULL
  AND (rc.retry_disposition IS NULL OR (rc.retry_disposition='automatic' AND (rc.retry_event_id IS NULL OR rc.retry_event_id='')))
ORDER BY rc.id LIMIT ?`, cfg.limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	candidates := []evaluationCandidate{}
	for rows.Next() {
		var item evaluationCandidate
		if err := rows.Scan(&item.CheckpointID, &item.AssessmentID, &item.Attempt, &item.Retryable, &item.OrgID, &item.TesteeID,
			&item.QuestionnaireCode, &item.QuestionnaireVersion, &item.AnswerSheetID, &item.ModelKind, &item.ModelSubKind,
			&item.ModelAlgorithm, &item.ModelCode, &item.ModelVersion); err != nil {
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if !cfg.apply {
		return len(candidates), rows.Err()
	}
	for _, item := range candidates {
		if err := applyEvaluation(ctx, db, item); err != nil {
			return 0, err
		}
	}
	return len(candidates), nil
}

func applyEvaluation(ctx context.Context, db *sql.DB, item evaluationCandidate) error {
	disposition := retrygovernance.BusinessPolicy().DecideFailure(item.Retryable, item.Attempt, time.Now()).Disposition
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if disposition != retrygovernance.DispositionAutomatic {
		_, err = tx.ExecContext(ctx, `UPDATE runtime_checkpoint SET attempt_origin=COALESCE(attempt_origin,'initial'), retry_disposition=?,
policy_max_attempts=3, retry_policy_version='business-retry/v1', next_attempt_at=NULL, updated_at=NOW(3)
WHERE id=? AND status='failed'`, disposition, item.CheckpointID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	now := time.Now()
	eventID := fmt.Sprintf("eval-retry:%d:%d:automatic", item.AssessmentID, item.Attempt)
	evt := event.NewRetryRequestedEvent(event.RequestedInput{
		EventID: eventID, OrgID: item.OrgID, AssessmentID: int64(item.AssessmentID), TesteeID: item.TesteeID,
		QuestionnaireCode: item.QuestionnaireCode, QuestionnaireVer: item.QuestionnaireVersion,
		AnswerSheetID: strconv.FormatUint(item.AnswerSheetID, 10), ModelKind: item.ModelKind.String,
		ModelSubKind: item.ModelSubKind.String, ModelAlgorithm: item.ModelAlgorithm.String, ModelCode: item.ModelCode.String,
		ModelVersion: item.ModelVersion.String, RequestedAt: now, ExpectedAttempt: item.Attempt,
		AttemptOrigin: "automatic", Mode: "next_attempt",
	})
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_checkpoint SET attempt_origin=COALESCE(attempt_origin,'initial'),
retry_disposition='automatic', next_attempt_at=?, policy_max_attempts=3, retry_policy_version='business-retry/v1',
retry_event_id=?, updated_at=? WHERE id=? AND status='failed' AND attempt_no=?`, now, eventID, now, item.CheckpointID, item.Attempt)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("evaluation checkpoint %d changed during backfill", item.CheckpointID)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO domain_event_outbox
(event_id,event_type,aggregate_type,aggregate_id,org_id,topic_name,payload_json,status,attempt_count,retry_disposition,next_attempt_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,'pending',0,NULL,?,?,?) ON DUPLICATE KEY UPDATE event_id=VALUES(event_id)`,
		eventID, evt.EventType(), evt.AggregateType(), evt.AggregateID(), item.OrgID, retryTopic, string(payload), now, now, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func backfillInterpretation(ctx context.Context, mysqlDB *sql.DB, db *mongo.Database, cfg config) (int, error) {
	cur, err := db.Collection("report_generations").Find(ctx, bson.M{"status": "failed", "deleted_at": nil}, options.Find().SetLimit(int64(cfg.limit)))
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	count := 0
	for cur.Next(ctx) {
		var generation generationCandidate
		if err := cur.Decode(&generation); err != nil {
			return count, err
		}
		var run runCandidate
		err := db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": generation.LatestRunID, "generation_id": generation.DomainID, "status": "failed"}).Decode(&run)
		if err == mongo.ErrNoDocuments {
			continue
		}
		if err != nil {
			return count, err
		}
		if run.RetryDisposition != "" && (run.RetryDisposition != string(retrygovernance.DispositionAutomatic) || run.RetryEventID != "") {
			continue
		}
		disposition := retrygovernance.BusinessPolicy().DecideFailure(run.Failure != nil && run.Failure.Retryable, run.Attempt, time.Now()).Disposition
		count++
		if !cfg.apply {
			continue
		}
		if err := applyInterpretation(ctx, mysqlDB, db, generation, run, disposition); err != nil {
			return count - 1, err
		}
	}
	return count, cur.Err()
}

func applyInterpretation(ctx context.Context, mysqlDB *sql.DB, db *mongo.Database, generation generationCandidate, run runCandidate, disposition retrygovernance.Disposition) error {
	now := time.Now()
	set := bson.M{"attempt_origin": "initial", "retry_disposition": disposition, "policy_max_attempts": 3,
		"retry_policy_version": "business-retry/v1", "updated_at": now}
	if disposition != retrygovernance.DispositionAutomatic {
		_, err := db.Collection("interpretation_runs").UpdateOne(ctx, bson.M{"domain_id": run.DomainID, "generation_id": generation.DomainID, "status": "failed"}, bson.M{"$set": set, "$unset": bson.M{"next_attempt_at": "", "retry_event_id": ""}})
		return err
	}
	correlation, err := loadOutcomeCorrelation(ctx, mysqlDB, generation.OutcomeID)
	if err != nil {
		return err
	}
	evt := domaininterpretation.NewInterpretationRetryRequestedEvent(domaininterpretation.RetryRequestedEventInput{
		OrgID: correlation.OrgID, GenerationID: strconv.FormatUint(generation.DomainID, 10), RunID: strconv.FormatUint(run.DomainID, 10),
		AssessmentID: strconv.FormatInt(correlation.AssessmentID, 10), OutcomeID: strconv.FormatUint(generation.OutcomeID, 10), TesteeID: correlation.TesteeID,
		ExpectedAttempt: run.Attempt, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: now,
	})
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	set["next_attempt_at"], set["retry_event_id"] = now, evt.EventID()
	session, err := db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		matched, err := db.Collection("report_generations").CountDocuments(tx, bson.M{"domain_id": generation.DomainID, "latest_run_id": run.DomainID, "status": "failed"})
		if err != nil || matched != 1 {
			return nil, fmt.Errorf("interpretation generation %d changed during backfill", generation.DomainID)
		}
		result, err := db.Collection("interpretation_runs").UpdateOne(tx, bson.M{"domain_id": run.DomainID, "generation_id": generation.DomainID, "status": "failed"}, bson.M{"$set": set})
		if err != nil || result.MatchedCount != 1 {
			return nil, fmt.Errorf("interpretation run %d changed during backfill", run.DomainID)
		}
		_, err = db.Collection("domain_event_outbox").UpdateOne(tx, bson.M{"event_id": evt.EventID()}, bson.M{"$setOnInsert": bson.M{
			"event_id": evt.EventID(), "event_type": evt.EventType(), "aggregate_type": evt.AggregateType(), "aggregate_id": evt.AggregateID(),
			"org_id": correlation.OrgID, "topic_name": retryTopic, "payload_json": string(payload), "status": "pending", "attempt_count": 0,
			"next_attempt_at": now, "created_at": now, "updated_at": now,
		}}, options.Update().SetUpsert(true))
		return nil, err
	})
	return err
}

func loadOutcomeCorrelation(ctx context.Context, db *sql.DB, outcomeID uint64) (outcomeCorrelation, error) {
	var item outcomeCorrelation
	err := db.QueryRowContext(ctx, "SELECT org_id, assessment_id, testee_id FROM evaluation_outcome WHERE id=?", outcomeID).
		Scan(&item.OrgID, &item.AssessmentID, &item.TesteeID)
	return item, err
}
