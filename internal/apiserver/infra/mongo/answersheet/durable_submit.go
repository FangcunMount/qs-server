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

	if _, err := r.Collection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "submit_meta.writer_id", Value: 1}, {Key: "submit_meta.idempotency_key", Value: 1}},
		Options: options.Index().SetName("uk_answersheet_submit_intent").SetUnique(true).
			SetPartialFilterExpression(bson.M{"submit_meta.idempotency_key": bson.M{"$exists": true}}),
	}); err != nil {
		return fmt.Errorf("create embedded answersheet idempotency index: %w", err)
	}

	return nil
}

func isIndexNotFound(err error) bool {
	var commandErr mongo.CommandError
	return stderrors.As(err, &commandErr) && commandErr.HasErrorCode(27)
}

func (r *Repository) FindCompletedSubmission(ctx context.Context, metaInfo submitport.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	return r.findByIdempotencyKey(ctx, metaInfo)
}

func (r *Repository) WaitForCompletedSubmission(ctx context.Context, metaInfo submitport.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	return r.waitForCompletedIdempotentResult(ctx, metaInfo)
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

	writerID, err := safeconv.Int64ToUint64(submissionContext.Filler().UserID())
	if err != nil {
		return nil, err
	}
	if metaInfo.IdempotencyKey != "" {
		po.SubmitMeta = &SubmitMetaPO{IdempotencyKey: metaInfo.IdempotencyKey, WriterID: writerID, Fingerprint: metaInfo.Fingerprint, RequestID: metaInfo.RequestID, AcceptedAt: time.Now()}
	}
	answerSheetDoc, err := po.ToBsonM()
	if err != nil {
		return nil, err
	}

	if _, err := r.Collection().InsertOne(ctx, answerSheetDoc); err != nil {
		return nil, err
	}

	events := append([]event.DomainEvent{}, sheet.Events()...)
	return events, nil
}

func (r *Repository) findByIdempotencyKey(ctx context.Context, metaInfo submitport.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	if metaInfo.IdempotencyKey == "" {
		return nil, nil
	}

	var sheetPO AnswerSheetPO
	if err := r.Collection().FindOne(ctx, bson.M{
		"submit_meta.writer_id":       metaInfo.WriterID,
		"submit_meta.idempotency_key": metaInfo.IdempotencyKey,
		"deleted_at":                  nil,
	}).Decode(&sheetPO); err != nil {
		if stderrors.Is(err, mongo.ErrNoDocuments) {
			return r.findLegacyIdempotentSubmission(ctx, metaInfo)
		}
		return nil, err
	}
	sheet := r.mapper.ToBO(&sheetPO)
	if sheet == nil || metaInfo.Fingerprint == "" {
		return sheet, nil
	}
	storedFingerprint := ""
	if sheetPO.SubmitMeta != nil {
		storedFingerprint = sheetPO.SubmitMeta.Fingerprint
	}
	if storedFingerprint == "" {
		storedFingerprint, err := submitport.Fingerprint(sheet)
		if err != nil {
			return nil, err
		}
		if storedFingerprint != metaInfo.Fingerprint {
			return nil, submitport.ErrIdempotencyConflict
		}
		return sheet, nil
	}
	if storedFingerprint != metaInfo.Fingerprint {
		return nil, submitport.ErrIdempotencyConflict
	}
	return sheet, nil
}

// findLegacyIdempotentSubmission keeps pre-migration retry keys readable. New
// submissions never write this collection; it can be archived after the
// compatibility window and an explicit data migration.
func (r *Repository) findLegacyIdempotentSubmission(ctx context.Context, metaInfo submitport.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	if r.idempotencyColl == nil {
		return nil, nil
	}
	var po AnswerSheetSubmitIdempotencyPO
	if err := r.idempotencyColl.FindOne(ctx, bson.M{"writer_id": metaInfo.WriterID, "idempotency_key": metaInfo.IdempotencyKey, "status": idempotencyStatusCompleted}).Decode(&po); err != nil {
		if stderrors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	sheet, err := r.FindByID(ctx, meta.MustFromUint64(po.AnswerSheetID))
	if err != nil || sheet == nil || metaInfo.Fingerprint == "" {
		return sheet, err
	}
	stored := po.Fingerprint
	if stored == "" {
		stored, err = submitport.Fingerprint(sheet)
		if err != nil {
			return nil, err
		}
	}
	if stored != metaInfo.Fingerprint {
		return nil, submitport.ErrIdempotencyConflict
	}
	return sheet, nil
}

func (r *Repository) waitForCompletedIdempotentResult(ctx context.Context, metaInfo submitport.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	deadline := time.Now().Add(idempotencyLookupTimeout)
	for {
		existing, err := r.findByIdempotencyKey(ctx, metaInfo)
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
