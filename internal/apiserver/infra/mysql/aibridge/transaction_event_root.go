package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

func (s *TransactionEvents) eventRoot(ctx context.Context, incoming app.FrozenIncomingEvent, execution transaction.Execution, ack app.FrozenMessagingArchive) (transaction.LocalOutcome, error) {
	if _, err := incoming.ReadACK(execution.Plan, s.archive.routes); err != nil {
		return transaction.LocalUnknown, err
	}
	header, raw, wire, organization := incoming.Input()
	body, err := app.ParseMessagingBody(header, raw)
	if err != nil {
		return transaction.LocalUnknown, err
	}
	ackBody, err := app.ParseMessagingBody(ack.Envelope, ack.Body)
	if err != nil {
		return transaction.LocalUnknown, err
	}
	held := ackBody.GetEventAcknowledgement().Outcome == pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
	outcome := "stored"
	if held {
		outcome = "held"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.archive.originalMessagingRoot(ctx, execution, func(tx *sql.Tx, guard *ArchiveGuard) error {
		attempts, err := s.store.FailureAttempts(ctx, tx, header)
		if err != nil {
			return err
		}
		if held != (attempts >= 8) {
			return errEventOutcomeChanged
		}
		// This insert serializes physical redeliveries. An existing Inbox must
		// never be adopted as proof for a newly prepared ACK transaction.
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_messaging_inbox(producer,message_id,body_sha256,body,wire_sha256,kind,aggregate_key,ack_id,outcome,received_at) VALUES(?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE message_id=message_id`, header.Producer, header.MessageId, header.BodySha256, raw, messagingHash(wire), header.Kind, header.AggregateKey, ack.Envelope.MessageId, outcome)
		if err != nil {
			return err
		}
		var oldBody []byte
		var hash, wireHash, aggregate, ackID, storedOutcome string
		var kind int32
		err = tx.QueryRowContext(ctx, `SELECT body_sha256,body,wire_sha256,kind,aggregate_key,ack_id,outcome FROM ai_messaging_inbox WHERE producer=? AND message_id=? FOR UPDATE`, header.Producer, header.MessageId).Scan(&hash, &oldBody, &wireHash, &kind, &aggregate, &ackID, &storedOutcome)
		if err != nil {
			return err
		}
		if hash != header.BodySha256 || !bytes.Equal(oldBody, raw) || wireHash != messagingHash(wire) || kind != int32(header.Kind) || aggregate != header.AggregateKey {
			return app.ErrConflict
		}
		if ackID != ack.Envelope.MessageId {
			return ErrLegacyEventResponsibility
		}
		if storedOutcome != outcome {
			return app.ErrConflict
		}
		var actualOrg string
		if held {
			actualOrg, err = s.store.heldEventOrganization(ctx, tx, header, body)
		} else {
			actualOrg, err = s.store.applyEvent(ctx, tx, header, body, raw)
		}
		if err != nil {
			return err
		}
		if actualOrg != organization {
			return app.ErrConflict
		}
		return guard.Put(ctx, ack.Envelope.MessageId)
	})
}

func (s *TransactionEvents) refuseHistoricalInbox(ctx context.Context, incoming app.FrozenIncomingEvent) error {
	header, raw, wire, _ := incoming.Input()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var oldBody []byte
	var hash, wireHash, aggregate string
	var kind int32
	err := s.archive.db.QueryRowContext(ctx, `SELECT body_sha256,body,wire_sha256,kind,aggregate_key FROM ai_messaging_inbox WHERE producer=? AND message_id=?`, header.Producer, header.MessageId).Scan(&hash, &oldBody, &wireHash, &kind, &aggregate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if hash != header.BodySha256 || !bytes.Equal(oldBody, raw) || wireHash != messagingHash(wire) || kind != int32(header.Kind) || aggregate != header.AggregateKey {
		return app.ErrConflict
	}
	return ErrLegacyEventResponsibility
}

func (s *TransactionEvents) verifyCommittedEvent(ctx context.Context, incoming app.FrozenIncomingEvent, execution transaction.Execution, duplicate bool) error {
	ack, err := incoming.ReadACK(execution.Plan, s.archive.routes)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stored, found, err := s.archive.ReadCommitted(ctx, ack.Envelope.MessageId)
	if err != nil {
		return err
	}
	if !found || !bytes.Equal(stored.Body, ack.Body) || !bytes.Equal(stored.Wire, ack.Wire) || stored.OrganizationID != ack.OrganizationID {
		return app.ErrMessagingContract
	}
	header, raw, wire, _ := incoming.Input()
	ackBody, err := app.ParseMessagingBody(ack.Envelope, ack.Body)
	if err != nil {
		return err
	}
	outcome := "stored"
	if ackBody.GetEventAcknowledgement().Outcome == pb.MessagingEventAcknowledgement_TECHNICALLY_HELD {
		outcome = "held"
	}
	var oldBody []byte
	var hash, wireHash, aggregate, ackID, oldOutcome string
	var kind int32
	err = s.archive.db.QueryRowContext(ctx, `SELECT body_sha256,body,wire_sha256,kind,aggregate_key,ack_id,outcome FROM ai_messaging_inbox WHERE producer=? AND message_id=?`, header.Producer, header.MessageId).Scan(&hash, &oldBody, &wireHash, &kind, &aggregate, &ackID, &oldOutcome)
	if err != nil {
		return err
	}
	if hash != header.BodySha256 || !bytes.Equal(oldBody, raw) || wireHash != messagingHash(wire) || kind != int32(header.Kind) || aggregate != header.AggregateKey || ackID != ack.Envelope.MessageId || oldOutcome != outcome {
		return app.ErrMessagingContract
	}
	if !duplicate {
		return nil
	}
	tx, err := s.archive.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = RecordMessagingObservation(ctx, tx, "duplicate_event"); err != nil {
		return err
	}
	return tx.Commit()
}
