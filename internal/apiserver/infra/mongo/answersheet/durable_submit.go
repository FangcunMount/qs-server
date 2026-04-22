package answersheet

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	appAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	idempotencyLookupTimeout = 2 * time.Second
	idempotencyLookupPoll    = 100 * time.Millisecond
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

		events := append([]event.DomainEvent{}, sheet.Events()...)
		orgID, err := safeconv.Uint64ToInt64(metaInfo.OrgID)
		if err != nil {
			return err
		}
		events = append(events, domainStatistics.NewFootprintAnswerSheetSubmittedEvent(
			orgID,
			metaInfo.TesteeID,
			sheet.ID().Uint64(),
			sheet.FilledAt(),
		))

		if err := r.outboxStore.StageEventsTx(txCtx, events); err != nil {
			return err
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

func (r *Repository) ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	return r.outboxStore.ClaimDueEvents(ctx, limit, now)
}

func (r *Repository) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	return r.outboxStore.MarkEventPublished(ctx, eventID, publishedAt)
}

func (r *Repository) MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	return r.outboxStore.MarkEventFailed(ctx, eventID, lastError, nextAttemptAt)
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
