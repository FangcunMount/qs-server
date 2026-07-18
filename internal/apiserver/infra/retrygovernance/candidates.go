package retrygovernance

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const maxCandidateOffset = 10000

func (r *Reader) ListRetryCandidates(ctx context.Context, orgID int64, cursor string, limit int) (app.RetryCandidatePage, error) {
	if r == nil || r.mysql == nil || r.mongo == nil || orgID <= 0 || limit < 1 || limit > 100 {
		return app.RetryCandidatePage{}, fmt.Errorf("invalid retry candidate query")
	}
	offset, err := decodeCandidateCursor(cursor)
	if err != nil {
		return app.RetryCandidatePage{}, err
	}
	fetch := offset + limit + 1
	if fetch > maxCandidateOffset+101 {
		return app.RetryCandidatePage{}, fmt.Errorf("retry candidate cursor exceeds bounded window")
	}

	items := make([]app.RetryCandidate, 0, fetch*4)
	if err := r.appendEvaluationCandidates(ctx, orgID, fetch, &items); err != nil {
		return app.RetryCandidatePage{}, err
	}
	if err := r.appendInterpretationCandidates(ctx, orgID, fetch, &items); err != nil {
		return app.RetryCandidatePage{}, err
	}
	if err := r.appendMySQLOutboxCandidates(ctx, orgID, fetch, &items); err != nil {
		return app.RetryCandidatePage{}, err
	}
	if err := r.appendMongoOutboxCandidates(ctx, orgID, fetch, &items); err != nil {
		return app.RetryCandidatePage{}, err
	}
	if err := r.appendDeliveryCandidates(ctx, orgID, fetch, &items); err != nil {
		return app.RetryCandidatePage{}, err
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].ResourceID < items[j].ResourceID
	})
	if offset >= len(items) {
		return app.RetryCandidatePage{Items: []app.RetryCandidate{}}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	page := app.RetryCandidatePage{Items: append([]app.RetryCandidate(nil), items[offset:end]...)}
	if end < len(items) {
		page.NextCursor = encodeCandidateCursor(end)
	}
	return page, nil
}

type businessCandidateRow struct {
	ResourceID      string
	Attempt         int
	Disposition     string
	NextAttemptAt   *time.Time
	RetryEventID    *string
	ActionRequestID *string
	UpdatedAt       time.Time
}

func (r *Reader) appendEvaluationCandidates(ctx context.Context, orgID int64, limit int, dst *[]app.RetryCandidate) error {
	var rows []businessCandidateRow
	err := r.mysql.WithContext(ctx).Raw(`
SELECT CAST(rc.assessment_id AS CHAR) resource_id, rc.attempt_no attempt,
       rc.retry_disposition disposition, rc.next_attempt_at, rc.retry_event_id,
       rc.action_request_id, rc.updated_at
FROM runtime_checkpoint rc
JOIN (SELECT assessment_id, MAX(attempt_no) attempt_no FROM runtime_checkpoint
      WHERE scope='evaluation_run' AND deleted_at IS NULL GROUP BY assessment_id) latest
  ON latest.assessment_id=rc.assessment_id AND latest.attempt_no=rc.attempt_no
JOIN assessment a ON a.id=rc.assessment_id AND a.deleted_at IS NULL
WHERE rc.scope='evaluation_run' AND rc.status='failed' AND rc.deleted_at IS NULL
  AND a.org_id=? AND rc.retry_disposition IN ('automatic','manual_required','terminal')
ORDER BY rc.updated_at DESC LIMIT ?`, orgID, limit).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		*dst = append(*dst, fromBusinessRow("evaluation", "mysql", row))
	}
	return nil
}

type interpretationCandidate struct {
	ResourceID      uint64     `bson:"resource_id"`
	OutcomeID       uint64     `bson:"outcome_id"`
	Attempt         int        `bson:"attempt"`
	Disposition     string     `bson:"disposition"`
	NextAttemptAt   *time.Time `bson:"next_attempt_at"`
	RetryEventID    string     `bson:"retry_event_id"`
	ActionRequestID string     `bson:"action_request_id"`
	UpdatedAt       time.Time  `bson:"updated_at"`
}

func (r *Reader) appendInterpretationCandidates(ctx context.Context, orgID int64, limit int, dst *[]app.RetryCandidate) error {
	var allowedOutcomeIDs []uint64
	if err := r.mysql.WithContext(ctx).Raw("SELECT id FROM evaluation_outcome WHERE org_id=? ORDER BY id DESC LIMIT ?", orgID, maxCandidateOffset+101).Scan(&allowedOutcomeIDs).Error; err != nil {
		return err
	}
	if len(allowedOutcomeIDs) == 0 {
		return nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "status", Value: "failed"}, {Key: "deleted_at", Value: nil}, {Key: "outcome_id", Value: bson.D{{Key: "$in", Value: allowedOutcomeIDs}}}}}},
		{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "interpretation_runs"}, {Key: "localField", Value: "latest_run_id"}, {Key: "foreignField", Value: "domain_id"}, {Key: "as", Value: "run"}}}},
		{{Key: "$unwind", Value: "$run"}},
		{{Key: "$match", Value: bson.D{{Key: "run.status", Value: "failed"}, {Key: "run.retry_disposition", Value: bson.D{{Key: "$in", Value: bson.A{"automatic", "manual_required", "terminal"}}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "run.updated_at", Value: -1}}}},
		{{Key: "$limit", Value: int64(limit)}},
		{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "resource_id", Value: "$domain_id"}, {Key: "outcome_id", Value: 1}, {Key: "attempt", Value: "$run.attempt"}, {Key: "disposition", Value: "$run.retry_disposition"}, {Key: "next_attempt_at", Value: "$run.next_attempt_at"}, {Key: "retry_event_id", Value: "$run.retry_event_id"}, {Key: "action_request_id", Value: "$run.action_request_id"}, {Key: "updated_at", Value: "$run.updated_at"}}}},
	}
	cur, err := r.mongo.Collection("report_generations").Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	var rows []interpretationCandidate
	if err := cur.All(ctx, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		*dst = append(*dst, app.RetryCandidate{Kind: "interpretation", Store: "mongo", ResourceID: strconv.FormatUint(row.ResourceID, 10), Attempt: row.Attempt, Disposition: row.Disposition, NextAttemptAt: row.NextAttemptAt, RetryEventID: row.RetryEventID, ActionRequestID: row.ActionRequestID, UpdatedAt: row.UpdatedAt})
	}
	return nil
}

type outboxCandidateRow struct {
	EventID       string
	AttemptCount  int
	Disposition   string
	NextAttemptAt *time.Time
	LastErrorKind *string
	UpdatedAt     time.Time
}

func (r *Reader) appendMySQLOutboxCandidates(ctx context.Context, orgID int64, limit int, dst *[]app.RetryCandidate) error {
	var rows []outboxCandidateRow
	if err := r.mysql.WithContext(ctx).Raw(`SELECT event_id, attempt_count, retry_disposition disposition,
next_attempt_at, last_error_kind, updated_at FROM domain_event_outbox
WHERE org_id=? AND status='failed' AND retry_disposition='manual_required'
ORDER BY updated_at DESC LIMIT ?`, orgID, limit).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		*dst = append(*dst, fromOutboxRow("mysql", row))
	}
	return nil
}

func (r *Reader) appendMongoOutboxCandidates(ctx context.Context, orgID int64, limit int, dst *[]app.RetryCandidate) error {
	findOpts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(int64(limit)).SetProjection(bson.M{"event_id": 1, "attempt_count": 1, "retry_disposition": 1, "next_attempt_at": 1, "last_error_kind": 1, "updated_at": 1})
	cur, err := r.mongo.Collection("domain_event_outbox").Find(ctx, bson.M{"org_id": orgID, "status": "failed", "retry_disposition": "manual_required"}, findOpts)
	if err != nil {
		return err
	}
	var rows []struct {
		EventID       string    `bson:"event_id"`
		AttemptCount  int       `bson:"attempt_count"`
		Disposition   string    `bson:"retry_disposition"`
		NextAttemptAt time.Time `bson:"next_attempt_at"`
		LastErrorKind string    `bson:"last_error_kind"`
		UpdatedAt     time.Time `bson:"updated_at"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		next := row.NextAttemptAt
		lastKind := row.LastErrorKind
		*dst = append(*dst, fromOutboxRow("mongo", outboxCandidateRow{EventID: row.EventID, AttemptCount: row.AttemptCount, Disposition: row.Disposition, NextAttemptAt: &next, LastErrorKind: &lastKind, UpdatedAt: row.UpdatedAt}))
	}
	return nil
}

func (r *Reader) appendDeliveryCandidates(ctx context.Context, orgID int64, limit int, dst *[]app.RetryCandidate) error {
	var rows []struct {
		ID               uint64
		DeliveryAttempts int
		LastError        *string
		UpdatedAt        time.Time
	}
	if err := r.mysql.WithContext(ctx).Raw(`SELECT id, delivery_attempts, last_error, updated_at
FROM event_delivery_dead_letter WHERE org_id=? AND retry_disposition='manual_required'
ORDER BY updated_at DESC LIMIT ?`, orgID, limit).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		last := valueOrEmpty(row.LastError)
		*dst = append(*dst, app.RetryCandidate{Kind: "transport_delivery", Store: "mysql", ResourceID: strconv.FormatUint(row.ID, 10), Attempt: row.DeliveryAttempts, Disposition: "manual_required", LastErrorKind: last, UpdatedAt: row.UpdatedAt})
	}
	return nil
}

func fromBusinessRow(kind, store string, row businessCandidateRow) app.RetryCandidate {
	return app.RetryCandidate{Kind: kind, Store: store, ResourceID: row.ResourceID, Attempt: row.Attempt, Disposition: row.Disposition, NextAttemptAt: row.NextAttemptAt, RetryEventID: valueOrEmpty(row.RetryEventID), ActionRequestID: valueOrEmpty(row.ActionRequestID), UpdatedAt: row.UpdatedAt}
}

func fromOutboxRow(store string, row outboxCandidateRow) app.RetryCandidate {
	return app.RetryCandidate{Kind: "outbox", Store: store, ResourceID: row.EventID, Attempt: row.AttemptCount, Disposition: row.Disposition, NextAttemptAt: row.NextAttemptAt, LastErrorKind: valueOrEmpty(row.LastErrorKind), UpdatedAt: row.UpdatedAt}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func encodeCandidateCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeCandidateCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid retry candidate cursor")
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 || offset > maxCandidateOffset {
		return 0, fmt.Errorf("invalid retry candidate cursor")
	}
	return offset, nil
}

var _ app.RetryCandidateReader = (*Reader)(nil)
