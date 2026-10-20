package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	owner "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

// ErrCommandUnresolved always refers to the original command ID. It grants no
// permission to replace that ID, re-Seal, or repeat an AI/provider invocation.
var ErrCommandUnresolved = errors.New("AI command transaction unresolved; query the original operation")

type commandSender interface {
	transaction.Sender
	Reserve(context.Context, transaction.Request, transaction.FrozenPlan) (transaction.Reservation, error)
}

// TransactionCommands borrows the already configured qs.mysql owner. It owns no
// producer, pool, maintenance loop, DDL or lifecycle. Start/Close remain host work.
type TransactionCommands struct {
	archive *TransactionArchive
	sender  commandSender
	seal    MessagingSeal
}

func NewTransactionCommands(db *gorm.DB, shared *owner.Committer, seal MessagingSeal) (*TransactionCommands, error) {
	if shared == nil || !shared.UsesPrimary(db) || !shared.HasTopic(app.CommandsTopic) || shared.Runtime() == nil || seal == nil {
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
	return &TransactionCommands{archive: archive, sender: shared.Runtime(), seal: seal}, nil
}

// Archive provides a read/guard adapter over the same borrowed primary.
func (s *TransactionCommands) Archive() *TransactionArchive { return s.archive }

// Validate is an explicit startup check, never an installer. The hosting process
// owns the shared owner's Start/Close and its complete transaction Topic list.
func (s *TransactionCommands) Validate(ctx context.Context) error {
	if s == nil || s.archive == nil || s.sender == nil || s.seal == nil {
		return transaction.ErrBinding
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.archive.store.Validate(ctx); err != nil {
		return err
	}
	if err := requireCommandOrderIndex(ctx, s.archive.db); err != nil {
		return err
	}
	rows, err := s.archive.db.QueryContext(ctx, "SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive WHERE FALSE")
	if err != nil {
		return err
	}
	return rows.Close()
}

// Submit first reads the exact stable request before clock/Seal/order planning.
// Only the reservation winner may open a local root, after all half ACKs.
func (s *TransactionCommands) Submit(ctx context.Context, c app.TransactionCommand) error {
	if s == nil || s.archive == nil || s.sender == nil || s.seal == nil || c.ID() == "" {
		return app.ErrManagementUnavailable
	}
	request := transaction.Request{Owner: "qs.mysql", Scope: "org:" + c.Scope().OrganizationID, Key: c.Key(), InputHash: c.InputHash()}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	winner, err := s.archive.store.Lookup(readCtx, request)
	cancel()
	if err == nil {
		return s.readWinner(ctx, request, winner)
	}
	if !errors.Is(err, transaction.ErrNotFound) {
		return err
	}
	// Do not reinterpret an old operation as a new transaction. The old path
	// retains its exact payload and its outstanding receipt/Relay responsibility.
	exists, err := s.legacyOperation(ctx, c)
	if err != nil || exists {
		return err
	}
	orderCtx, orderCancel := context.WithTimeout(ctx, 2*time.Second)
	sequence, err := readCommandSequence(orderCtx, s.archive.db, c, false)
	orderCancel()
	if err != nil {
		return err
	}
	body, err := c.Body()
	if err != nil {
		return err
	}
	header, _, err := app.PrepareMessaging(c.Kind(), c.ID(), c.Aggregate(), "", body)
	if err != nil {
		return err
	}
	original := time.Now().In(time.FixedZone("UTC+8", 8*60*60)).Format(time.RFC3339Nano)
	prepared, err := s.seal(header.Kind, c.ID(), c.Aggregate(), c.Scope().OrganizationID, original, body)
	if err != nil {
		return err
	}
	plan, err := c.Freeze(prepared, sequence)
	if err != nil {
		return err
	}
	winner, err = s.sender.Reserve(ctx, request, plan)
	if err != nil {
		return err
	}
	if err := request.Match(winner.Execution.Plan); err != nil {
		return err
	}
	if _, _, err := app.ReadTransactionCommand(winner.Execution.Plan); err != nil {
		return err
	}
	if !winner.Acquired {
		return s.readWinner(ctx, request, winner)
	}
	result, err := executeCommand(ctx, s.sender, winner.Execution, s.archive.commandRoot)
	if result.Local == transaction.LocalCommitted {
		return nil // FinalUnknown does not erase a proven local commit.
	}
	confirmCtx, confirmCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer confirmCancel()
	if s.archive.confirmCommand(confirmCtx, winner.Execution) == transaction.LocalCommitted {
		return nil
	}
	return errors.Join(ErrCommandUnresolved, err)
}

func (s *TransactionCommands) readWinner(ctx context.Context, q transaction.Request, winner transaction.Reservation) error {
	if err := q.Match(winner.Execution.Plan); err != nil {
		return err
	}
	if _, _, err := app.ReadTransactionCommand(winner.Execution.Plan); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.archive.confirmCommand(ctx, winner.Execution) == transaction.LocalCommitted {
		return nil
	}
	return ErrCommandUnresolved // Prepared/Aborted never silently authorize Execute.
}

func (s *TransactionCommands) legacyOperation(ctx context.Context, c app.TransactionCommand) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	body, err := c.Body()
	if err != nil {
		return false, err
	}
	header, _, err := app.PrepareMessaging(c.Kind(), c.ID(), c.Aggregate(), "", body)
	if err != nil {
		return false, err
	}
	tx, err := s.archive.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := NewMessagingStore().existingOperation(ctx, tx, header, c.Scope())
	if err != nil {
		return false, err
	}
	if exists {
		var archives int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_archive WHERE producer=? AND destination=? AND message_id=?", []byte("qs-server"), []byte("qs-ai"), []byte(c.ID())).Scan(&archives); err != nil {
			return false, err
		}
		if archives != 0 {
			return false, app.ErrMessagingContract // damaged new request is never legacy success
		}
		return true, nil
	}
	gate, err := (MessagingAdmission{}).Inspect(ctx, tx)
	if err != nil {
		return false, err
	}
	if gate.Closed {
		return false, app.ErrRuntimeAdmissionClosed
	}
	return false, nil
}

// Join a genuinely running root even if the upstream Execute returns early on
// cancellation. A late/second callback is refused without opening another root.
func executeCommand(ctx context.Context, sender transaction.Sender, e transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
	var mu sync.Mutex
	started, closed, committed := false, false, false
	done := make(chan struct{})
	result, err := sender.Execute(ctx, e, func(ctx context.Context, actual transaction.Execution) (transaction.LocalOutcome, error) {
		mu.Lock()
		if closed || started {
			mu.Unlock()
			return transaction.LocalUnknown, transaction.ErrStaleExecution
		}
		started = true
		mu.Unlock()
		defer close(done)
		if actual.Plan.Digest() != e.Plan.Digest() || actual.Fence != e.Fence || actual.WriterEpoch != e.WriterEpoch || actual.Token != e.Token {
			return transaction.LocalUnknown, transaction.ErrStaleExecution
		}
		state, err := local(ctx, actual)
		mu.Lock()
		committed = state == transaction.LocalCommitted
		mu.Unlock()
		return state, err
	})
	mu.Lock()
	closed = true
	join := started
	mu.Unlock()
	if join {
		<-done
	}
	mu.Lock()
	if committed {
		result.Local = transaction.LocalCommitted
	}
	mu.Unlock()
	return result, err
}
