package answersheet

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ListSubmittedAnswerSheetFacts scans submitted answer sheets for behavior journey projection.
func (r *Repository) ListSubmittedAnswerSheetFacts(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
) ([]domainStatistics.AnswerSheetSubmittedFact, error) {
	if r == nil || limit <= 0 {
		return nil, nil
	}
	filter := bson.M{
		"org_id":     uint64(orgID),
		"deleted_at": nil,
	}
	if !sinceTime.IsZero() {
		filter["filled_at"] = bson.M{"$gte": sinceTime}
	}
	if sinceID > 0 {
		filter["domain_id"] = bson.M{"$gt": meta.FromUint64(sinceID)}
	}
	cursor, err := r.Collection().Find(ctx, filter, options.Find().
		SetProjection(bson.M{
			"domain_id": 1,
			"org_id":    1,
			"testee_id": 1,
			"filled_at": 1,
		}).
		SetSort(bson.D{{Key: "domain_id", Value: 1}}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	facts := make([]domainStatistics.AnswerSheetSubmittedFact, 0, limit)
	for cursor.Next(ctx) {
		var row struct {
			DomainID meta.ID   `bson:"domain_id"`
			OrgID    uint64    `bson:"org_id"`
			TesteeID uint64    `bson:"testee_id"`
			FilledAt time.Time `bson:"filled_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		facts = append(facts, domainStatistics.AnswerSheetSubmittedFact{
			OrgID:         orgID,
			TesteeID:      row.TesteeID,
			AnswerSheetID: row.DomainID.Uint64(),
			OccurredAt:    row.FilledAt,
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return facts, nil
}
