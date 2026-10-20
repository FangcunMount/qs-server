package mongoconsistency

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type transactionAuditOwner interface {
	UsesDatabase(*mongo.Database) bool
	PlanForReference(context.Context, transaction.Reference) (transaction.FrozenPlan, error)
	Evidence(context.Context, transaction.Reference) (*transaction.Evidence, error)
}

// WithTransactionOwner borrows the host's fixed original-database authority.
// No constructor, producer or checker is started by this read-only scanner.
func (s *Scanner) WithTransactionOwner(owner *guarded.Owner) *Scanner {
	s.transactionOwner = owner
	if owner.UsesDatabase(s.db) {
		s.db = s.db.Client().Database(s.db.Name(), options.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()))
	}
	return s
}

func countTransactionEvidence(result *appaudit.BatchResult) {
	if result.EvidenceClasses == nil {
		result.EvidenceClasses = map[string]int64{}
	}
	result.EvidenceClasses[string(evidence.TransactionReferenceClass)]++
}

func (s *Scanner) checkTransactionEvidence(ctx context.Context, proof *evidence.EventEvidenceV2, eventID, eventType, aggregateType, aggregateID, scope, binding string, typed func([]byte) (string, error)) error {
	if err := proof.Validate(); err != nil {
		return conflict(err)
	}
	if proof.Reference.EventID != eventID || proof.Reference.EventType != eventType || proof.Reference.Scope != scope || proof.BusinessBindingSHA256 != binding {
		return driftf("transaction business event binding conflict")
	}
	if s == nil || s.transactionOwner == nil || !s.transactionOwner.UsesDatabase(s.db) {
		return fmt.Errorf("V2 consistency audit requires the bound original Mongo transaction owner")
	}
	ref, err := transaction.DecodeReference(proof.TransactionReference)
	if err != nil || ref.Owner != "qs.mongo" {
		return driftf("transaction audit owner differs")
	}
	plan, err := s.transactionOwner.PlanForReference(ctx, ref)
	if err != nil {
		return transactionAuditError(err)
	}
	var found bool
	for _, branch := range plan.Input().Branches {
		actual, err := plan.Reference(branch.ID, ref.Fence, ref.WriterEpoch)
		if err != nil || actual != ref {
			continue
		}
		if standard.ReferenceFromMessage(branch.Message) != proof.Reference {
			return driftf("transaction original metadata differs")
		}
		consumer := branch.Message.Input()
		consumer.Payload = bytes.Clone(branch.Wire)
		m, err := message.New(consumer)
		if err != nil {
			return conflict(err)
		}
		wireRef := standard.ReferenceFromMessage(m)
		envelope, err := standard.VerifyReference(consumer, wireRef.Fingerprint, wireRef)
		if err != nil {
			return conflict(err)
		}
		if envelope.AggregateType != aggregateType || envelope.AggregateID != aggregateID {
			return driftf("transaction aggregate differs")
		}
		actualBinding, err := typed(envelope.Data)
		if err != nil {
			return conflict(err)
		}
		if actualBinding != binding {
			return driftf("transaction typed original payload conflict")
		}
		found = true
		break
	}
	if !found {
		return driftf("exact transaction branch missing")
	}
	fact, err := s.transactionOwner.Evidence(ctx, ref)
	if err != nil {
		return transactionAuditError(err)
	}
	if transaction.Resolve(ref, fact) != transaction.Commit {
		return driftf("business fact lacks exact committed transaction witness")
	}
	return nil
}

// Missing/contradictory protocol facts are drift. Transport/database errors
// remain infrastructure errors and therefore cannot advance a checkpoint.
func transactionAuditError(err error) error {
	if errors.Is(err, transaction.ErrNotFound) || errors.Is(err, transaction.ErrInvalidPlan) {
		return conflict(err)
	}
	return err
}
