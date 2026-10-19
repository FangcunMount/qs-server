package answersheet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	submit "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrSubmissionUnresolved = errors.New("submission outcome unresolved; reuse the original idempotency key")

type submissionSender interface {
	Reserve(context.Context, transaction.Request, transaction.FrozenPlan) (transaction.Reservation, error)
	transaction.Sender
}

// RocketMQSubmissionStore uses the repository's original Mongo database for
// both the business document and protocol witnesses. It never stages Outbox
// rows. The host owns runner admission and the returned runtime lifecycle.
type RocketMQSubmissionStore struct {
	repo      *Repository
	authority *txmongo.Store
	runner    apptransaction.Runner
	sender    submissionSender
	runtime   *messagingruntime.TransactionRuntime
	topic     string
}

// NewRocketMQSubmissionStore is inert: no schema, network, goroutines or Start.
// The authority and sender are constructed together to prevent a second store
// with the same textual binding from replacing the original host database.
func NewRocketMQSubmissionStore(repo *Repository, runner apptransaction.Runner, o *options.MessagingOptions, resolver eventcatalog.TopicResolver) (*RocketMQSubmissionStore, error) {
	if repo == nil || repo.DB() == nil || runner == nil || resolver == nil || o == nil || o.RocketMQ == nil {
		return nil, fmt.Errorf("original Mongo repository, transaction runner and explicit RocketMQ configuration required")
	}
	topic, found := resolver.GetTopicForEvent(domain.EventTypeSubmitted)
	if !found || topic == "" {
		return nil, fmt.Errorf("submission route absent")
	}
	config, found := o.RocketMQ.Transactions[submissionOwner]
	if !found {
		return nil, transaction.ErrBinding
	}
	store, err := txmongo.New(repo.DB(), transaction.Binding{Owner: submissionOwner, StoreID: config.StoreID, WriterEpoch: config.WriterEpoch})
	if err != nil {
		return nil, err
	}
	runtime, err := messagingruntime.NewTransactionalSender(o, submissionOwner, store, []string{topic})
	if err != nil {
		return nil, err
	}
	return &RocketMQSubmissionStore{repo: repo, authority: store, runner: runner, sender: runtime, runtime: runtime, topic: topic}, nil
}

func (s *RocketMQSubmissionStore) Runtime() *messagingruntime.TransactionRuntime { return s.runtime }

func (s *RocketMQSubmissionStore) FindCompleted(ctx context.Context, meta submit.DurableSubmitMeta) (*submit.CompletedSubmission, error) {
	if meta.IdempotencyKey == "" {
		return nil, nil
	}
	return s.findCompleted(ctx, bson.M{"submit_meta.writer_id": meta.WriterID, "submit_meta.idempotency_key": meta.IdempotencyKey, "deleted_at": nil}, meta.Fingerprint)
}

func (s *RocketMQSubmissionStore) findCompleted(ctx context.Context, filter bson.M, fingerprint string) (*submit.CompletedSubmission, error) {
	var po AnswerSheetPO
	if err := s.repo.FindOne(ctx, filter, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	if a := po.DurableAcceptance; a != nil && a.SchemaVersion != 1 {
		if err := s.confirmSubmission(ctx, po); err != nil {
			return nil, err
		}
	}
	stored := ""
	if po.SubmitMeta != nil {
		stored = po.SubmitMeta.Fingerprint
	}
	return completedSubmission(s.repo.mapper.ToBO(&po), stored, fingerprint)
}

func (s *RocketMQSubmissionStore) confirmSubmission(ctx context.Context, po AnswerSheetPO) error {
	a := po.DurableAcceptance
	if a == nil || a.SchemaVersion != 2 || a.EventEvidence != nil || a.TransactionEvidence == nil {
		return ErrSubmissionUnresolved
	}
	proof := a.TransactionEvidence
	if err := proof.Validate(); err != nil {
		return ErrSubmissionUnresolved
	}
	ref, err := transaction.DecodeReference(proof.TransactionReference)
	if err != nil || ref.Owner != submissionOwner || ref.LogicalTopic != s.topic || a.EventID != ref.MessageID {
		return ErrSubmissionUnresolved
	}
	binding, err := submissionBinding(po)
	if err != nil || binding != proof.BusinessBindingSHA256 {
		return ErrSubmissionUnresolved
	}
	sheet := s.repo.mapper.ToBO(&po)
	fingerprint, err := submit.Fingerprint(sheet)
	if err != nil {
		return ErrSubmissionUnresolved
	}
	key := ""
	if v := po.SubmitMeta; v != nil {
		if v.WriterID != uint64(po.FillerID) || v.Fingerprint != fingerprint || v.RequestID != a.RequestID || !v.AcceptedAt.Equal(a.AcceptedAt) {
			return ErrSubmissionUnresolved
		}
		key = v.IdempotencyKey
	}
	var hash [32]byte
	decoded, _ := hex.DecodeString(fingerprint)
	copy(hash[:], decoded)
	winner, err := s.authority.Lookup(ctx, transaction.Request{Owner: submissionOwner, Scope: ref.Scope, Key: submissionKey(uint64(po.FillerID), key, ref.OperationID), InputHash: hash})
	if err != nil || winner.State != transaction.Committed {
		return ErrSubmissionUnresolved
	}
	refs := winner.Execution.References()
	if len(refs) != 1 || refs[0] != ref {
		return ErrSubmissionUnresolved
	}
	frozen, frozenBinding, err := readSubmissionPlan(winner.Execution.Plan)
	if err != nil || frozen.DomainID != po.DomainID || frozen.ID != po.ID || frozenBinding != binding || !frozen.DurableAcceptance.AcceptedAt.Equal(a.AcceptedAt) ||
		proof.Reference != standard.ReferenceFromMessage(winner.Execution.Plan.Input().Branches[0].Message) || !proof.RecordedAt.Equal(a.AcceptedAt) {
		return ErrSubmissionUnresolved
	}
	e, err := s.authority.Evidence(ctx, ref)
	if err != nil || transaction.Resolve(ref, e) != transaction.Commit {
		return ErrSubmissionUnresolved
	}
	return nil
}

func (s *RocketMQSubmissionStore) CreateDurably(ctx context.Context, sheet *domain.AnswerSheet, meta submit.DurableSubmitMeta) (*domain.AnswerSheet, bool, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return nil, false, fmt.Errorf("submission half preparation must precede the original Mongo root transaction")
	}
	if existing, err := s.FindCompleted(ctx, meta); err != nil || existing != nil {
		if err != nil {
			return nil, false, err
		}
		return existing.Sheet, true, nil
	}
	plan, err := freezeSubmission(ctx, sheet, meta, s.topic)
	if err != nil {
		return nil, false, err
	}
	reservation, err := s.sender.Reserve(ctx, transaction.RequestFor(plan), plan)
	if err != nil {
		if errors.Is(err, transaction.ErrInputConflict) {
			return nil, false, submit.ErrIdempotencyConflict
		}
		return nil, false, err
	}
	frozen, _, err := readSubmissionPlan(reservation.Execution.Plan)
	if err != nil {
		return nil, false, err
	}
	if !reservation.Acquired {
		return s.recover(ctx, frozen.DomainID.Uint64(), meta.Fingerprint, true, ErrSubmissionUnresolved)
	}
	// The final BSON, including the exact reference, is also frozen before the
	// retryable driver callback. No signing, encoding or event creation in it.
	raw, _, err := materializeSubmission(reservation.Execution)
	if err != nil {
		return nil, false, err
	}
	_, executeErr := s.sender.Execute(ctx, reservation.Execution, func(localCtx context.Context, execution transaction.Execution) (transaction.LocalOutcome, error) {
		err := s.runner.WithinTransaction(localCtx, func(txCtx context.Context) error {
			session := mongo.SessionFromContext(txCtx)
			if session == nil {
				return transaction.ErrTransactionRequired
			}
			guard, err := s.authority.Bind(mongo.NewSessionContext(txCtx, session), execution)
			if err != nil {
				return err
			}
			if _, err := s.repo.InsertOne(txCtx, raw); err != nil {
				if mongo.IsDuplicateKeyError(err) && strings.Contains(err.Error(), "uk_answersheet_answering_start") {
					return submit.ErrIdempotencyConflict
				}
				return err
			}
			return guard.CommitWitness(txCtx)
		})
		if err != nil {
			return transaction.LocalUnknown, err
		}
		return transaction.LocalCommitted, nil
	})
	if executeErr == nil {
		executeErr = ErrSubmissionUnresolved
	}
	// Cancellation and a missing commit response cannot prove rollback. Read
	// the exact fact AND witness from a new boundary; never rerun the executor.
	result, existed, err := s.recover(ctx, frozen.DomainID.Uint64(), meta.Fingerprint, false, executeErr)
	if err == nil {
		sheet.ClearEvents()
	}
	return result, existed, err
}

func (s *RocketMQSubmissionStore) recover(ctx context.Context, id uint64, fingerprint string, existed bool, cause error) (*domain.AnswerSheet, bool, error) {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	completed, err := s.findCompleted(readCtx, bson.M{"domain_id": id, "deleted_at": nil}, fingerprint)
	if err != nil {
		return nil, false, errors.Join(cause, err)
	}
	if completed == nil {
		return nil, false, cause
	}
	return completed.Sheet, existed, nil
}
