package answersheet

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"time"

	appAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	idempotencyLookupTimeout = 2 * time.Second
	idempotencyLookupPoll    = 100 * time.Millisecond
	outboxPublishingStaleFor = 1 * time.Minute
	outboxRetryDelay         = 10 * time.Second
)

func (r *Repository) ensureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := r.idempotencyColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "idempotency_key", Value: 1}},
			Options: options.Index().SetName("uk_idempotency_key").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "updated_at", Value: 1}},
			Options: options.Index().SetName("idx_status_updated_at"),
		},
	}); err != nil {
		return fmt.Errorf("create answersheet idempotency indexes: %w", err)
	}

	if _, err := r.outboxColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetName("uk_event_id").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}},
			Options: options.Index().SetName("idx_status_next_attempt_at"),
		},
	}); err != nil {
		return fmt.Errorf("create answersheet outbox indexes: %w", err)
	}

	return nil
}

func (r *Repository) CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, metaInfo appAnswerSheet.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	if sheet == nil {
		return nil, false, nil
	}

	if metaInfo.IdempotencyKey != "" {
		existing, err := r.findByIdempotencyKey(ctx, metaInfo.IdempotencyKey)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			return existing, true, nil
		}
	}

	po := r.mapper.ToPO(sheet)
	if po == nil {
		return nil, false, nil
	}

	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()
	sheet.AssignID(meta.ID(po.DomainID))
	sheet.RaiseSubmittedEvent(metaInfo.TesteeID, metaInfo.OrgID, metaInfo.TaskID)

	answerSheetDoc, err := po.ToBsonM()
	if err != nil {
		return nil, false, err
	}

	outboxDocs, err := r.buildOutboxDocuments(sheet.Events())
	if err != nil {
		return nil, false, err
	}

	var idempotencyDoc *AnswerSheetSubmitIdempotencyPO
	if metaInfo.IdempotencyKey != "" {
		code, version, _ := sheet.QuestionnaireInfo()
		now := time.Now()
		idempotencyDoc = &AnswerSheetSubmitIdempotencyPO{
			IdempotencyKey:       metaInfo.IdempotencyKey,
			WriterID:             metaInfo.WriterID,
			TesteeID:             metaInfo.TesteeID,
			QuestionnaireCode:    code,
			QuestionnaireVersion: version,
			AnswerSheetID:        sheet.ID().Uint64(),
			Status:               idempotencyStatusCompleted,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
	}

	if err := r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		if idempotencyDoc != nil {
			if _, err := r.idempotencyColl.InsertOne(txCtx, idempotencyDoc); err != nil {
				return err
			}
		}

		if _, err := r.Collection().InsertOne(txCtx, answerSheetDoc); err != nil {
			return err
		}

		if len(outboxDocs) > 0 {
			docs := make([]interface{}, 0, len(outboxDocs))
			for _, doc := range outboxDocs {
				docs = append(docs, doc)
			}
			if _, err := r.outboxColl.InsertMany(txCtx, docs); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		if metaInfo.IdempotencyKey != "" {
			existing, lookupErr := r.waitForCompletedIdempotentResult(ctx, metaInfo.IdempotencyKey)
			if lookupErr == nil && existing != nil {
				sheet.ClearEvents()
				return existing, true, nil
			}
		}
		return nil, false, err
	}

	sheet.ClearEvents()
	return sheet, false, nil
}

func (r *Repository) ClaimDueSubmittedEvents(ctx context.Context, limit int, now time.Time) ([]appAnswerSheet.PendingSubmittedEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	claimed := make([]appAnswerSheet.PendingSubmittedEvent, 0, limit)
	staleBefore := now.Add(-outboxPublishingStaleFor)
	for len(claimed) < limit {
		filter := bson.M{
			"$or": []bson.M{
				{"status": outboxStatusPending, "next_attempt_at": bson.M{"$lte": now}},
				{"status": outboxStatusFailed, "next_attempt_at": bson.M{"$lte": now}},
				{"status": outboxStatusPublishing, "updated_at": bson.M{"$lte": staleBefore}},
			},
		}
		update := bson.M{
			"$set": bson.M{
				"status":     outboxStatusPublishing,
				"updated_at": now,
			},
		}
		opts := options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "created_at", Value: 1}}).
			SetReturnDocument(options.After)

		var po AnswerSheetSubmittedOutboxPO
		if err := r.outboxColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&po); err != nil {
			if stderrors.Is(err, mongo.ErrNoDocuments) {
				break
			}
			return nil, err
		}

		evt, err := po.ToEvent()
		if err != nil {
			_ = r.MarkSubmittedEventFailed(ctx, po.EventID, fmt.Sprintf("decode outbox payload: %v", err), time.Now().Add(outboxRetryDelay))
			continue
		}

		claimed = append(claimed, appAnswerSheet.PendingSubmittedEvent{
			EventID: po.EventID,
			Event:   evt,
		})
	}

	return claimed, nil
}

func (r *Repository) MarkSubmittedEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	_, err := r.outboxColl.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":       outboxStatusPublished,
			"published_at": publishedAt,
			"updated_at":   publishedAt,
		},
	})
	return err
}

func (r *Repository) MarkSubmittedEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	_, err := r.outboxColl.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":          outboxStatusFailed,
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now(),
		},
		"$inc": bson.M{
			"attempt_count": 1,
		},
	})
	return err
}

func (r *Repository) buildOutboxDocuments(events []event.DomainEvent) ([]*AnswerSheetSubmittedOutboxPO, error) {
	if len(events) == 0 {
		return nil, nil
	}

	docs := make([]*AnswerSheetSubmittedOutboxPO, 0, len(events))
	now := time.Now()
	for _, evt := range events {
		topicName, ok := eventconfig.Global().GetTopicForEvent(evt.EventType())
		if !ok {
			return nil, fmt.Errorf("event %q not found in event config", evt.EventType())
		}

		payload, err := json.Marshal(evt)
		if err != nil {
			return nil, err
		}

		docs = append(docs, &AnswerSheetSubmittedOutboxPO{
			EventID:       evt.EventID(),
			EventType:     evt.EventType(),
			AggregateType: evt.AggregateType(),
			AggregateID:   evt.AggregateID(),
			TopicName:     topicName,
			PayloadJSON:   string(payload),
			Status:        outboxStatusPending,
			AttemptCount:  0,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	return docs, nil
}

func (r *Repository) withTransaction(ctx context.Context, fn func(txCtx mongo.SessionContext) error) error {
	session, err := r.DB().Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(txCtx)
	})
	return err
}

func (r *Repository) findByIdempotencyKey(ctx context.Context, key string) (*domainAnswerSheet.AnswerSheet, error) {
	if key == "" {
		return nil, nil
	}

	var po AnswerSheetSubmitIdempotencyPO
	if err := r.idempotencyColl.FindOne(ctx, bson.M{
		"idempotency_key": key,
		"status":          idempotencyStatusCompleted,
	}).Decode(&po); err != nil {
		if stderrors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return r.FindByID(ctx, meta.MustFromUint64(po.AnswerSheetID))
}

func (r *Repository) waitForCompletedIdempotentResult(ctx context.Context, key string) (*domainAnswerSheet.AnswerSheet, error) {
	deadline := time.Now().Add(idempotencyLookupTimeout)
	for {
		existing, err := r.findByIdempotencyKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
		if time.Now().After(deadline) {
			return nil, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(idempotencyLookupPoll):
		}
	}
}
