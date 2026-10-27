package aibridge

import (
	"context"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	owner "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrEventUnresolved = errors.New("AI event transaction unresolved; retain the original delivery")
var ErrLegacyEventResponsibility = errors.New("historical AI event retains its original acknowledgement responsibility")
var errEventOutcomeChanged = errors.New("AI event failure disposition changed before original root")

// TransactionEvents borrows the SAME qs.mysql runtime/pool as commands. It owns
// no network client, maintenance loop or lifecycle. Settle is a host consumer
// entrypoint only after actual topic/signature/body authentication.
type TransactionEvents struct {
	archive *TransactionArchive
	store   *MessagingStore
	sender  commandSender
	ready   func() bool
	seal    MessagingSeal
	reserve transaction.ReserveOptions
}

func NewTransactionEvents(db *gorm.DB, shared *owner.Committer, store *MessagingStore, seal MessagingSeal, config options.TransactionOptions) (*TransactionEvents, error) {
	if shared == nil || !shared.UsesPrimary(db) || !shared.HasTopic(app.AcksTopic) || shared.Runtime() == nil || store == nil || seal == nil {
		return nil, transaction.ErrBinding
	}
	limits, err := config.ProtocolLimits("qs.mysql")
	if err != nil || shared.Binding() != (transaction.Binding{Owner: "qs.mysql", StoreID: config.StoreID, WriterEpoch: config.WriterEpoch}) {
		return nil, transaction.ErrBinding
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	archive, err := NewTransactionArchive(pool, shared.Binding())
	if err != nil {
		return nil, err
	}
	archive.routes = shared.Runtime().Routes()
	// Bind the receipt reader to this same primary without mutating the caller's
	// legacy store. Direct users get the same proof boundary as module assembly.
	borrowed := *store
	borrowed.Archive = archive
	return &TransactionEvents{archive: archive, store: &borrowed, sender: shared.Runtime(), ready: func() bool { return shared.Runtime().Stats().Ready }, seal: seal, reserve: transaction.ReserveOptions{Lease: limits.Lease}}, nil
}

func (s *TransactionEvents) Settle(ctx context.Context, header *pb.MessagingEnvelope, raw, wire []byte, forceHeld bool) error {
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return transaction.ErrTransactionRequired
	}
	if s == nil || s.archive == nil || s.store == nil || s.sender == nil || s.seal == nil || s.ready == nil || !s.reserve.Valid() {
		return app.ErrManagementUnavailable
	}
	body, err := app.ParseMessagingBody(header, raw)
	if err != nil {
		return err
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	organization, err := readEventOrganization(readCtx, s.archive.db, header, body, false)
	if err != nil {
		cancel()
		return err
	}
	attempts, err := readTechnicalAttempts(readCtx, s.archive.db, header, false)
	cancel()
	if err != nil {
		return err
	}
	incoming, err := app.FreezeIncomingEvent(header, raw, wire, organization)
	if err != nil {
		return err
	}
	held := forceHeld || attempts >= 8
	if forceHeld && attempts < 8 {
		return app.ErrInvalid
	}
	reservation, err := s.reserveEvent(ctx, incoming, held)
	if err != nil {
		return err
	}
	ack, err := incoming.ReadACK(reservation.Execution.Plan, s.archive.routes)
	if err != nil {
		return err
	}
	if !reservation.Acquired {
		if s.confirmEvent(ctx, incoming, reservation.Execution) == transaction.LocalCommitted {
			return s.verifyCommittedEvent(ctx, incoming, reservation.Execution, true)
		}
		return ErrEventUnresolved
	}
	result, sendErr := executeCommand(ctx, s.sender, reservation.Execution, func(local context.Context, actual transaction.Execution) (transaction.LocalOutcome, error) {
		return s.eventRoot(local, incoming, actual, ack)
	})
	if result.Local == transaction.LocalCommitted {
		return nil
	}
	proofCtx, proofCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer proofCancel()
	if s.confirmEvent(proofCtx, incoming, reservation.Execution) == transaction.LocalCommitted {
		return s.verifyCommittedEvent(proofCtx, incoming, reservation.Execution, false)
	}
	return errors.Join(ErrEventUnresolved, sendErr)
}

func (s *TransactionEvents) reserveEvent(ctx context.Context, incoming app.FrozenIncomingEvent, held bool) (transaction.Reservation, error) {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	previous, err := s.archive.store.Lookup(readCtx, incoming.Request())
	cancel()
	if err == nil {
		oldACK, validateErr := incoming.ReadACK(previous.Execution.Plan, s.archive.routes)
		if validateErr != nil {
			return transaction.Reservation{}, validateErr
		}
		state := s.confirmEvent(ctx, incoming, previous.Execution)
		if state != transaction.LocalAborted {
			return previous, nil
		}
		if !s.ready() {
			return transaction.Reservation{}, messagingruntime.ErrTransactionAdmissionClosed
		}
		// A CURRENT authenticated consumer delivery is the host authorization to
		// retry only an exactly Aborted SQL projection/Inbox. SDK Checker and
		// maintenance never enter this path, and no model/provider is invoked.
		desired := pb.MessagingEventAcknowledgement_STORED
		if held {
			desired = pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
		}
		oldBody, decodeErr := app.ParseMessagingBody(oldACK.Envelope, oldACK.Body)
		if decodeErr != nil {
			return transaction.Reservation{}, decodeErr
		}
		if oldBody.GetEventAcknowledgement().Outcome != desired {
			candidate, err := s.freezeACK(incoming, held)
			if err != nil {
				return transaction.Reservation{}, err
			}
			readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return s.archive.store.Replan(readCtx, transaction.ReplanAuthorization{Previous: previous.Execution, Candidate: candidate}, s.reserve)
		}
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return s.archive.store.Retry(readCtx, transaction.RetryAuthorization{Previous: previous.Execution}, s.reserve)
	}
	if !errors.Is(err, transaction.ErrNotFound) {
		return transaction.Reservation{}, err
	}
	if !s.ready() {
		return transaction.Reservation{}, messagingruntime.ErrTransactionAdmissionClosed
	}
	if err := s.refuseHistoricalInbox(ctx, incoming); err != nil {
		return transaction.Reservation{}, err
	}
	candidate, err := s.freezeACK(incoming, held)
	if err != nil {
		return transaction.Reservation{}, err
	}
	reservation, err := s.sender.Reserve(ctx, incoming.Request(), candidate)
	if err != nil {
		return transaction.Reservation{}, err
	}
	if _, err := incoming.ReadACK(reservation.Execution.Plan, s.archive.routes); err != nil {
		return transaction.Reservation{}, err
	}
	return reservation, nil
}

func (s *TransactionEvents) freezeACK(incoming app.FrozenIncomingEvent, held bool) (transaction.FrozenPlan, error) {
	header, _, _, org := incoming.Input()
	outcome := pb.MessagingEventAcknowledgement_STORED
	if held {
		outcome = pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
	}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: header.MessageId, EventBodySha256: header.BodySha256, EventKind: header.Kind, Outcome: outcome}}}
	// Both identities, the time and protected wire are generated outside SQL.
	ack, err := s.seal(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, uuid.NewString(), header.AggregateKey, org, time.Now().In(time.FixedZone("UTC+8", 8*60*60)).Format(time.RFC3339Nano), body)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	return incoming.FreezeACK(ack, uuid.NewString(), s.archive.routes)
}

func (s *TransactionEvents) confirmEvent(ctx context.Context, incoming app.FrozenIncomingEvent, execution transaction.Execution) transaction.LocalOutcome {
	if _, err := incoming.ReadACK(execution.Plan, s.archive.routes); err != nil {
		return transaction.LocalUnknown
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var evidence []*transaction.Evidence
	for _, ref := range execution.References() {
		fact, err := s.archive.store.Evidence(ctx, ref)
		if err != nil {
			return transaction.LocalUnknown
		}
		evidence = append(evidence, fact)
	}
	return transaction.DecideLocal(execution.References(), evidence)
}
