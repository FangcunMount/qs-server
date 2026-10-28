package aibridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
)

// TransactionArchive borrows the primary pool. Construction performs no I/O.
// Only an original-root guard can write; this is not a publish queue.
type TransactionArchive struct {
	db     *sql.DB
	store  *txmysql.Store
	routes *transport.RouteResolver
}

func NewTransactionArchive(db *sql.DB, binding transaction.Binding) (*TransactionArchive, error) {
	if binding.Owner != "qs.mysql" {
		return nil, transaction.ErrBinding
	}
	s, err := txmysql.New(db, binding)
	if err != nil {
		return nil, err
	}
	return &TransactionArchive{db: db, store: s}, nil
}

type ArchiveGuard struct {
	tx        *sql.Tx
	guard     *txmysql.Guard
	execution transaction.Execution
	sealed    bool
}

func (s *TransactionArchive) Bind(ctx context.Context, tx *sql.Tx, e transaction.Execution) (*ArchiveGuard, error) {
	g, err := s.store.Bind(ctx, tx, e)
	if err != nil {
		return nil, err
	}
	return &ArchiveGuard{tx: tx, guard: g, execution: e}, nil
}

// Put uses the reservation winner, never a newly prepared candidate. A duplicate
// identity must retain every original byte and exact execution reference.
func (g *ArchiveGuard) Put(ctx context.Context, branchID string) error {
	if g == nil || g.tx == nil || g.sealed {
		return transaction.ErrTransactionRequired
	}
	for _, b := range g.execution.Plan.Input().Branches {
		if b.ID == branchID {
			a, err := app.DecodeMessagingBranch(b)
			if err != nil {
				return err
			}
			if a.Envelope.Producer != "qs-server" || a.Envelope.Destination != "qs-ai" {
				return app.ErrMessagingContract
			}
			ref, err := g.execution.Plan.Reference(b.ID, g.execution.Fence, g.execution.WriterEpoch)
			if err != nil {
				return err
			}
			raw, err := transaction.EncodeReference(ref)
			if err != nil {
				return err
			}
			ek, bk := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence), transaction.BranchKey(ref.BranchID)
			in := b.Message.Input()
			_, err = g.tx.ExecContext(ctx, `INSERT INTO ai_messaging_archive (producer,destination,message_id,organization_id,frozen_payload,first_wire,execution_key,branch_key,reference) VALUES(?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE message_id=message_id`, []byte(in.Producer), []byte(a.Envelope.Destination), []byte(in.ID), a.OrganizationID, in.Payload, b.Wire, ek[:], bk[:], raw)
			if err != nil {
				return err
			}
			var org uint64
			var payload, wire, execution, branch, reference []byte
			err = g.tx.QueryRowContext(ctx, `SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive WHERE producer=? AND destination=? AND message_id=?`, []byte(in.Producer), []byte(a.Envelope.Destination), []byte(in.ID)).Scan(&org, &payload, &wire, &execution, &branch, &reference)
			if err != nil {
				return err
			}
			if org != a.OrganizationID || !bytes.Equal(payload, in.Payload) || !bytes.Equal(wire, b.Wire) || !bytes.Equal(execution, ek[:]) || !bytes.Equal(branch, bk[:]) || !bytes.Equal(reference, raw) {
				return app.ErrMessagingContract
			}
			return nil
		}
	}
	return transaction.ErrInvalidPlan
}

// CommitWitness only stages protocol evidence. The host commits the root.
func (g *ArchiveGuard) CommitWitness(ctx context.Context) error {
	if g == nil || g.sealed {
		return transaction.ErrTransactionRequired
	}
	if err := g.guard.CommitWitness(ctx); err != nil {
		return err
	}
	g.sealed = true
	return nil
}

// ReadCommitted returns found even for an unpublished/damaged archive. Callers
// never fall back to legacy Outbox. Evidence is freshly read
// from the same configured primary; neither a wire ACK nor a business row proves commit.
func (s *TransactionArchive) ReadCommitted(ctx context.Context, id string) (a app.FrozenMessagingArchive, found bool, err error) {
	return s.readCommitted(ctx, s.db, id, s.store.Evidence)
}

type archiveReadConnection interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Receipt settlement already owns its original root. Read the immutable command
// proof on that same connection; never borrow a second pool slot while holding
// business locks. This is NOT the Checker/Payload fresh read boundary above.
func (s *TransactionArchive) readCommittedInRoot(ctx context.Context, tx *sql.Tx, id string) (app.FrozenMessagingArchive, bool, error) {
	if tx == nil {
		return app.FrozenMessagingArchive{}, true, transaction.ErrTransactionRequired
	}
	return s.readCommitted(ctx, tx, id, func(ctx context.Context, ref transaction.Reference) (*transaction.Evidence, error) {
		if !ref.Valid() || ref.Owner != s.store.Binding().Owner {
			return nil, transaction.ErrBinding
		}
		ek, bk := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence), transaction.BranchKey(ref.BranchID)
		var raw []byte
		var fact transaction.Evidence
		err := tx.QueryRowContext(ctx, `SELECT w.reference,e.state,w.state FROM rm_tx_execution e JOIN rm_tx_witness w ON w.execution_key=e.execution_key JOIN rm_tx_binding b ON b.owner=e.owner WHERE e.execution_key=? AND w.branch_key=? AND b.store_id=? AND b.schema_version=?`, ek[:], bk[:], []byte(s.store.Binding().StoreID), txmysql.SchemaVersion).Scan(&raw, &fact.ExecutionState, &fact.BranchState)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		fact.Reference, err = transaction.DecodeReference(raw)
		return &fact, err
	})
}

func (s *TransactionArchive) readCommitted(ctx context.Context, q archiveReadConnection, id string, readEvidence func(context.Context, transaction.Reference) (*transaction.Evidence, error)) (a app.FrozenMessagingArchive, found bool, err error) {
	var org uint64
	var payload, wire, execution, branch, raw []byte
	err = q.QueryRowContext(ctx, `SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive WHERE producer=? AND destination=? AND message_id=?`, []byte("qs-server"), []byte("qs-ai"), []byte(id)).Scan(&org, &payload, &wire, &execution, &branch, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	ref, err := transaction.DecodeReference(raw)
	if err != nil {
		return a, true, err
	}
	a, err = app.DecodeMessagingArchive(payload, wire)
	if err != nil {
		return a, true, err
	}
	m, err := a.Message()
	if err != nil {
		return a, true, err
	}
	ek, bk := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence), transaction.BranchKey(ref.BranchID)
	if ref.Owner != s.store.Binding().Owner || ref.RouteRevision != "m3-1" || ref.Scope != m.Input().Scope || ref.LogicalTopic != m.Input().Destination || ref.MessageID != id || ref.MessageFingerprint != m.Fingerprint() || ref.WireHash != sha256.Sum256(wire) || a.Envelope.MessageId != id || a.Envelope.Producer != "qs-server" || a.Envelope.Destination != "qs-ai" || org != a.OrganizationID || !bytes.Equal(execution, ek[:]) || !bytes.Equal(branch, bk[:]) {
		return a, true, app.ErrMessagingContract
	}
	if s.routes != nil {
		_, err = s.routes.RecordedRoute(transport.RouteKey{Topic: ref.LogicalTopic, MessageType: transport.TransactionMessageType, Owner: ref.Owner, RouteRevision: ref.RouteRevision}, ref.PhysicalTopic)
		if err != nil {
			return a, true, app.ErrMessagingContract
		}
	}
	evidence, err := readEvidence(ctx, ref)
	if err != nil {
		return a, true, err
	}
	if transaction.Resolve(ref, evidence) != transaction.Commit {
		return a, true, app.ErrMessagingContract
	}
	return a, true, nil
}

// Payload retains the original authenticated workload, tenant and exact-body
// contract. Missing current archives expose no bytes; callers return NotFound.
func (s *TransactionArchive) Payload(ctx context.Context, ref *pb.MessagePayloadReference, workload string) ([]byte, bool, error) {
	if ref == nil || workload != "qs-ai" || ref.Producer != "qs-server" || ref.Destination != "qs-ai" {
		return nil, true, app.ErrMessagingContract
	}
	a, found, err := s.ReadCommitted(ctx, ref.MessageId)
	if err != nil {
		return nil, found, err
	}
	if !found {
		return nil, true, app.ErrNotFound
	}

	if ref.OrganizationId != strconv.FormatUint(a.OrganizationID, 10) || ref.BodySha256 != a.Envelope.BodySha256 || ref.BodyLength != uint64(len(a.Body)) {
		// A caller reference mismatch keeps the established sanitized NotFound
		// RPC result. Internal proof/binding errors above remain unavailable.
		return nil, true, app.ErrConflict
	}
	return bytes.Clone(a.Body), true, nil
}
