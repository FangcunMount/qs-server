package retrygovernance

import (
	"context"
	"fmt"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type Reader struct {
	mysql            *gorm.DB
	mongo            *mongo.Database
	standardOutboxes map[string]app.OutboxGovernanceReader
}

type countRow struct {
	Disposition string
	Count       int64
}

func NewReader(mysql *gorm.DB, mongoDB *mongo.Database) *Reader {
	return &Reader{mysql: mysql, mongo: mongoDB}
}

// WithStandardOutboxes replaces only the selected profiles' historical
// Outbox views. Business retry, delivery dead letters and holds remain owned
// by their existing readers.
func (r *Reader) WithStandardOutboxes(readers map[string]app.OutboxGovernanceReader) *Reader {
	if r != nil {
		r.standardOutboxes = make(map[string]app.OutboxGovernanceReader, len(readers))
		for name, reader := range readers {
			r.standardOutboxes[name] = reader
		}
	}
	return r
}

func (r *Reader) ReadRetryGovernance(ctx context.Context, orgID int64) (app.RetryGovernanceSummary, error) {
	var summary app.RetryGovernanceSummary
	if r == nil || r.mysql == nil || r.mongo == nil || orgID <= 0 {
		return summary, fmt.Errorf("retry governance stores are not configured")
	}
	var evaluation []countRow
	if err := r.mysql.WithContext(ctx).Raw(`
SELECT rc.retry_disposition disposition, COUNT(*) count
FROM runtime_checkpoint rc
JOIN (SELECT assessment_id, MAX(attempt_no) attempt_no FROM runtime_checkpoint
      WHERE scope='evaluation_run' AND deleted_at IS NULL GROUP BY assessment_id) latest
 ON latest.assessment_id=rc.assessment_id AND latest.attempt_no=rc.attempt_no
JOIN assessment a ON a.id=rc.assessment_id AND a.deleted_at IS NULL
WHERE rc.scope='evaluation_run' AND rc.status='failed' AND rc.deleted_at IS NULL AND a.org_id=?
GROUP BY rc.retry_disposition`, orgID).Scan(&evaluation).Error; err != nil {
		return summary, err
	}
	addDispositionCounts(&summary, evaluation)

	var outcomeIDs []uint64
	if err := r.mysql.WithContext(ctx).Raw("SELECT id FROM evaluation_outcome WHERE org_id=?", orgID).Scan(&outcomeIDs).Error; err != nil {
		return summary, err
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "status", Value: "failed"}, {Key: "deleted_at", Value: nil}, {Key: "outcome_id", Value: bson.D{{Key: "$in", Value: outcomeIDs}}}}}},
		{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "interpretation_runs"}, {Key: "localField", Value: "latest_run_id"}, {Key: "foreignField", Value: "domain_id"}, {Key: "as", Value: "run"}}}},
		{{Key: "$unwind", Value: "$run"}},
		{{Key: "$match", Value: bson.D{{Key: "run.status", Value: "failed"}, {Key: "run.retry_disposition", Value: bson.D{{Key: "$in", Value: bson.A{"automatic", "manual_required", "terminal"}}}}}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$run.retry_disposition"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	}
	cursor, err := r.mongo.Collection("report_generations").Aggregate(ctx, pipeline)
	if err != nil {
		return summary, err
	}
	var interpretation []struct {
		Disposition string `bson:"_id"`
		Count       int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &interpretation); err != nil {
		return summary, err
	}
	for _, row := range interpretation {
		addDisposition(&summary, row.Disposition, row.Count)
	}

	if err := r.addOutboxGovernance(ctx, orgID, "assessment-mysql-outbox", &summary); err != nil {
		return summary, err
	}
	if err := r.addOutboxGovernance(ctx, orgID, "mongo-domain-events", &summary); err != nil {
		return summary, err
	}
	if err := r.mysql.WithContext(ctx).Raw("SELECT COUNT(*) FROM event_delivery_dead_letter WHERE org_id=? AND retry_disposition='manual_required'", orgID).Scan(&summary.TransportDeadLetters).Error; err != nil {
		return summary, err
	}
	if err := r.mysql.WithContext(ctx).Raw("SELECT COUNT(*) FROM event_delivery_dead_letter WHERE org_id=? AND retry_disposition='automatic' AND replay_request_id IS NOT NULL", orgID).Scan(&summary.TransportReplayUnresolved).Error; err != nil {
		return summary, err
	}
	if err := r.mysql.WithContext(ctx).Raw("SELECT COUNT(*) FROM retry_event_hold WHERE org_id=? AND retry_disposition='automatic' AND status IN ('blocked','failed','replaying')", orgID).Scan(&summary.HeldAutomatic).Error; err != nil {
		return summary, err
	}
	if err := r.mysql.WithContext(ctx).Raw("SELECT COUNT(*) FROM retry_event_hold WHERE org_id=? AND retry_disposition='manual_required' AND status='failed'", orgID).Scan(&summary.HeldManualRequired).Error; err != nil {
		return summary, err
	}
	return summary, nil
}

func (r *Reader) addOutboxGovernance(ctx context.Context, orgID int64, name string, summary *app.RetryGovernanceSummary) error {
	if standard := r.standardOutboxes[name]; standard != nil {
		counts, err := standard.ReadOutboxGovernance(ctx, orgID)
		if err != nil {
			return err
		}
		summary.OutboxAutomatic += counts.Automatic
		summary.OutboxManual += counts.ManualRequired
		summary.OutboxAuthorized += counts.Authorized
		summary.OutboxTerminal += counts.Terminal
		summary.BlockedRetryEvents += counts.BlockedRetryEvents
		return nil
	}
	if name == "assessment-mysql-outbox" {
		var rows []countRow
		if err := r.mysql.WithContext(ctx).Raw("SELECT retry_disposition disposition, COUNT(*) count FROM domain_event_outbox WHERE org_id=? AND status='failed' GROUP BY retry_disposition", orgID).Scan(&rows).Error; err != nil {
			return err
		}
		addOutboxCounts(summary, rows)
		var blocked int64
		if err := r.mysql.WithContext(ctx).Raw("SELECT COUNT(*) FROM domain_event_outbox WHERE org_id=? AND status='failed' AND retry_disposition='manual_required' AND event_type IN ('evaluation.retry.requested','interpretation.retry.requested')", orgID).Scan(&blocked).Error; err != nil {
			return err
		}
		summary.BlockedRetryEvents += blocked
		return nil
	}
	for _, disposition := range []string{"automatic", "manual_required"} {
		count, err := r.mongo.Collection("domain_event_outbox").CountDocuments(ctx, bson.M{"org_id": orgID, "status": "failed", "retry_disposition": disposition})
		if err != nil {
			return err
		}
		addOutbox(summary, disposition, count)
	}
	blocked, err := r.mongo.Collection("domain_event_outbox").CountDocuments(ctx, bson.M{"org_id": orgID, "status": "failed", "retry_disposition": "manual_required", "event_type": bson.M{"$in": []string{"evaluation.retry.requested", "interpretation.retry.requested"}}})
	if err != nil {
		return err
	}
	summary.BlockedRetryEvents += blocked
	return nil
}

func addDispositionCounts(summary *app.RetryGovernanceSummary, rows []countRow) {
	for _, row := range rows {
		addDisposition(summary, row.Disposition, row.Count)
	}
}
func addDisposition(summary *app.RetryGovernanceSummary, disposition string, count int64) {
	switch disposition {
	case "automatic":
		summary.Automatic += count
	case "manual_required":
		summary.ManualRequired += count
	case "terminal":
		summary.Terminal += count
	}
}
func addOutboxCounts(summary *app.RetryGovernanceSummary, rows []countRow) {
	for _, row := range rows {
		addOutbox(summary, row.Disposition, row.Count)
	}
}
func addOutbox(summary *app.RetryGovernanceSummary, disposition string, count int64) {
	switch disposition {
	case "automatic":
		summary.OutboxAutomatic += count
	case "manual_required":
		summary.OutboxManual += count
	}
}

var _ app.RetryGovernanceReader = (*Reader)(nil)
