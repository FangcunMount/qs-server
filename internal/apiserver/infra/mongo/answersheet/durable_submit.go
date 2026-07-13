package answersheet

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	idempotencyLookupTimeout = 2 * time.Second
	idempotencyLookupPoll    = 100 * time.Millisecond
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

func (r *Repository) FindCompletedSubmission(ctx context.Context, idempotencyKey string) (*domainAnswerSheet.AnswerSheet, error) {
	return r.findByIdempotencyKey(ctx, idempotencyKey)
}

func (r *Repository) WaitForCompletedSubmission(ctx context.Context, idempotencyKey string) (*domainAnswerSheet.AnswerSheet, error) {
	return r.waitForCompletedIdempotentResult(ctx, idempotencyKey)
}

func (r *Repository) SaveSubmittedAnswerSheet(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, metaInfo submitport.DurableSubmitMeta) ([]event.DomainEvent, error) {
	if sheet == nil {
		return nil, nil
	}
	if sheet.ID().IsZero() {
		return nil, fmt.Errorf("answersheet durable save requires preassigned answer sheet id")
	}
	submissionContext := sheet.SubmissionContext()
	if err := submissionContext.Validate(); err != nil {
		return nil, fmt.Errorf("answersheet durable save requires complete submission context: %w", err)
	}

	po := r.mapper.ToPO(sheet)
	if po == nil {
		return nil, nil
	}

	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()

	answerSheetDoc, err := po.ToBsonM()
	if err != nil {
		return nil, err
	}
	writerID, err := safeconv.Int64ToUint64(submissionContext.Filler().UserID())
	if err != nil {
		return nil, err
	}
	testeeID, err := safeconv.MetaIDToUint64(submissionContext.TesteeID())
	if err != nil {
		return nil, err
	}
	var idempotencyDoc *AnswerSheetSubmitIdempotencyPO
	if metaInfo.IdempotencyKey != "" {
		code, version, _ := sheet.QuestionnaireInfo()
		now := time.Now()
		idempotencyDoc = &AnswerSheetSubmitIdempotencyPO{
			IdempotencyKey:       metaInfo.IdempotencyKey,
			WriterID:             writerID,
			TesteeID:             testeeID,
			QuestionnaireCode:    code,
			QuestionnaireVersion: version,
			AnswerSheetID:        sheet.ID().Uint64(),
			Status:               idempotencyStatusCompleted,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
	}

	if idempotencyDoc != nil {
		if _, err := r.idempotencyColl.InsertOne(ctx, idempotencyDoc); err != nil {
			return nil, err
		}
	}

	if _, err := r.Collection().InsertOne(ctx, answerSheetDoc); err != nil {
		return nil, err
	}

	events := append([]event.DomainEvent{}, sheet.Events()...)
	return events, nil
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
