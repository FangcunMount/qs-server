package evaluation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	evalevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
)

type committedStandardRow struct {
	ID            uint64
	Producer      string
	MessageID     string
	Destination   string
	EventType     string
	SchemaVersion string
	Scope         string
	ContentType   string
	OccurredAt    string
	Payload       []byte
	Fingerprint   []byte
	State         string
}

const canonicalMissingClassificationReason = "canonical outcome lacks classified committed event evidence"

func (row committedStandardRow) input() message.Input {
	return message.Input{Producer: row.Producer, ID: row.MessageID, Destination: row.Destination, EventType: row.EventType,
		SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload}
}
func verifyCommittedStandard(row committedStandardRow, record *domainoutcome.Record) error {
	evidence := record.CommittedEventEvidence()
	if evidence == nil || evidence.Class != eventevidence.StandardReferenceClass || evidence.Reference == nil {
		return fmt.Errorf("standard message has no canonical standard reference")
	}
	if err := evidence.Validate(); err != nil {
		return err
	}
	if evidence.BusinessBindingSHA256 != record.BusinessBindingSHA256() {
		return fmt.Errorf("canonical business binding changed")
	}
	inner, err := standard.VerifyReference(row.input(), hex.EncodeToString(row.Fingerprint), *evidence.Reference)
	if err != nil {
		return err
	}
	var data eventpayload.EvaluationOutcomeCommittedData
	if err := json.Unmarshal(inner.Data, &data); err != nil {
		return err
	}
	if inner.AggregateType != evalevent.AggregateType || inner.AggregateID != record.AssessmentID().String() ||
		data.OrgID != record.OrgID() || data.AssessmentID <= 0 || uint64(data.AssessmentID) != record.AssessmentID().Uint64() ||
		data.TesteeID != record.TesteeID() || data.OutcomeID != record.ID().String() || data.EvaluationRunID != record.RunID() ||
		eventevidence.MillisecondTime(data.CommittedAt) != eventevidence.MillisecondTime(record.EvaluatedAt()) {
		return fmt.Errorf("committed message conflicts with frozen outcome identity or business time")
	}
	return nil
}

func (r *consistencyReadModel) listCommittedOutboxEvidence(ctx context.Context, assessmentIDs []uint64) (map[uint64]*evaluationconsistency.CommittedOutboxEvidence, error) {
	var outcomes []struct {
		EvaluationOutcomePO `gorm:"embedded"`
		CanonicalPairNull   bool `gorm:"column:canonical_pair_null"`
	}
	if err := r.db.WithContext(ctx).Table("evaluation_outcome").Select("evaluation_outcome.*, committed_event_id IS NULL AND committed_event_evidence IS NULL AS canonical_pair_null").Where("assessment_id IN ?", assessmentIDs).Find(&outcomes).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(outcomes))
	for _, po := range outcomes {
		if po.CommittedEventID != nil {
			ids = append(ids, *po.CommittedEventID)
		}
	}
	rowsByID := make(map[string][]committedStandardRow)
	if len(ids) > 0 {
		var rows []committedStandardRow
		if err := r.db.WithContext(ctx).Table("rm_outbox").Where("message_id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			rowsByID[row.MessageID] = append(rowsByID[row.MessageID], row)
		}
	}
	result := make(map[uint64]*evaluationconsistency.CommittedOutboxEvidence, len(outcomes))
	for i := range outcomes {
		po := &outcomes[i].EvaluationOutcomePO
		e := &evaluationconsistency.CommittedOutboxEvidence{OutcomeID: strconv.FormatUint(po.ID, 10), RunID: po.EvaluationRunID}
		result[po.AssessmentID] = e
		record, err := outcomeFromPO(po)
		if err != nil {
			e.InvalidReason = err.Error()
			continue
		}
		proof := record.CommittedEventEvidence()
		if proof == nil {
			e.InvalidReason = canonicalMissingClassificationReason
			e.LegacyCanonicalAbsent = outcomes[i].CanonicalPairNull
			continue
		}
		e.Class = proof.Class
		if proof.Class != eventevidence.StandardReferenceClass {
			e.HistoricalReason = proof.Verification.Reason
			// Historical verification remains historical even after source deletion.
			continue
		}
		rows := rowsByID[proof.EventID]
		e.RowCount = int64(len(rows))
		if len(rows) != 1 {
			e.InvalidReason = "canonical standard reference does not resolve to exactly one message"
			continue
		}
		e.Status = rows[0].State
		if err := verifyCommittedStandard(rows[0], record); err != nil {
			e.InvalidReason = err.Error()
		}
	}
	return result, nil
}
func (r *consistencyReadModel) BusinessUpperBound(ctx context.Context) (uint64, error) {
	var upper uint64
	err := r.db.WithContext(ctx).Table("assessment").Select("COALESCE(MAX(id),0)").Scan(&upper).Error
	return upper, err
}
func (r *consistencyReadModel) OutboxUpperBound(ctx context.Context) (uint64, error) {
	var upper uint64
	err := r.db.WithContext(ctx).Table("rm_outbox").Select("COALESCE(MAX(id),0)").Scan(&upper).Error
	return upper, err
}
func (r *consistencyReadModel) ReadOutboxBatch(ctx context.Context, afterID, upperID uint64, limit int) (evaluationconsistency.ReverseBatch, error) {
	if limit <= 0 {
		return evaluationconsistency.ReverseBatch{}, fmt.Errorf("positive standard reverse audit batch limit is required")
	}
	var rows []committedStandardRow
	if err := r.db.WithContext(ctx).Table("rm_outbox").Where("id > ? AND id <= ?", afterID, upperID).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return evaluationconsistency.ReverseBatch{}, err
	}
	batch := evaluationconsistency.ReverseBatch{Scanned: len(rows), CycleComplete: len(rows) < limit}
	if len(rows) == 0 {
		return batch, nil
	}
	batch.NextCursor = rows[len(rows)-1].ID
	outcomeIDs := make([]uint64, 0, len(rows))
	parsed := make(map[uint64]uint64)
	for _, row := range rows {
		// Validate every row before filtering supported business types. A damaged
		// outer type cannot hide an orphan committed event from reverse inspection.
		msg, err := message.New(row.input())
		if err != nil {
			batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, Reason: "invalid standard message fields"})
			continue
		}
		ref := standard.ReferenceFromMessage(msg)
		inner, err := standard.VerifyReference(row.input(), hex.EncodeToString(row.Fingerprint), ref)
		if err != nil {
			batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, Reason: err.Error()})
			continue
		}
		switch inner.EventType {
		case eventcatalog.AnswerSheetSubmitted, eventcatalog.InterpretationReportGenerated, eventcatalog.InterpretationRetryRequested:
			batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, Reason: "Mongo-owned domain event stored in MySQL standard outbox"})
		case eventcatalog.EvaluationOutcomeCommitted:
			var data eventpayload.EvaluationOutcomeCommittedData
			if err = json.Unmarshal(inner.Data, &data); err != nil {
				batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, Reason: "invalid committed payload"})
				continue
			}
			id, err := strconv.ParseUint(data.OutcomeID, 10, 64)
			if err != nil || id == 0 {
				batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, Reason: "invalid committed outcome identity"})
				continue
			}
			parsed[row.ID] = id
			outcomeIDs = append(outcomeIDs, id)
		}
	}
	if len(outcomeIDs) == 0 {
		return batch, nil
	}
	var outcomes []EvaluationOutcomePO
	if err := r.db.WithContext(ctx).Where("id IN ?", outcomeIDs).Find(&outcomes).Error; err != nil {
		return evaluationconsistency.ReverseBatch{}, err
	}
	byID := make(map[uint64]*EvaluationOutcomePO, len(outcomes))
	for i := range outcomes {
		byID[outcomes[i].ID] = &outcomes[i]
	}
	for _, row := range rows {
		id, ok := parsed[row.ID]
		if !ok {
			continue
		}
		po := byID[id]
		if po == nil {
			batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, Reason: "standard committed message is orphaned"})
			continue
		}
		record, err := outcomeFromPO(po)
		if err == nil {
			err = verifyCommittedStandard(row, record)
		}
		if err != nil {
			batch.Conflicts = append(batch.Conflicts, evaluationconsistency.ReverseConflict{MessageID: row.MessageID, AssessmentID: po.AssessmentID, Reason: err.Error()})
		}
	}
	return batch, nil
}

var _ evaluationconsistency.CycleReader = (*consistencyReadModel)(nil)
