package retirement

import (
	"bytes"
	"context"
	"reflect"
	"strconv"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/mongo"
)

const ErrCoordinatorCASQualification SourceError = "historical_cas_opaque_joint_qualification_rejected"

// The old two-argument PrepareSQLCAS/PrepareMongoCAS gates remain unchanged.
// This bounded factory accepts only this coordinator's actual consumed joint
// page and the actual independently rescanned origin/AI proofs. No Candidate,
// JSON report, attachment, terminal flag, clock or imported approval is input.
//
// Returned plans prepare business evidence only: they neither Apply/Commit nor
// authorize a production invocation, writer-fence bypass, qs-ai remote mutation,
// old-table DROP or archive purge. Those remain the host's separate gates.
func (c *HistoricalCoordinator) PrepareQualifiedHistoricalCAS(ctx context.Context, joint *WholeSourceJointPage, origin *SourceOriginRecheckProof, ai *AIReverseFreshProof) (*sqlevaluation.SQLHistoricalBatchCASPlan, *MongoHistoricalBatchCASPlan, *HistoricalCASPersistencePage, error) {
	rows, err := c.qualifiedCASRows(ctx, joint, origin, ai)
	if err != nil {
		return nil, nil, nil, err
	}
	var sqlAttachments []sqlevaluation.SQLHistoricalBatchAttachment
	var mongoAttachments []MongoHistoricalBatchAttachment
	var sqlBindings []sqlevaluation.SQLHistoricalBatchAttachment
	var sqlPositions []int
	for position, row := range rows {
		if row.facts.Source.Database == "mysql" {
			assessment, outcome, err := qualifiedCASOwnerIDs(row.facts, row.candidate)
			if err != nil {
				return nil, nil, nil, err
			}
			// Only source-derived selector heads are submitted; this existing
			// Attachment DTO is not treated as qualification or write evidence.
			sqlBindings = append(sqlBindings, sqlevaluation.SQLHistoricalBatchAttachment{AssessmentID: assessment, OutcomeID: outcome, Entry: evidence.HistoricalReferenceEntryV1{EventType: row.facts.EventType, Run: row.candidate.ActualOriginalRun}})
			sqlPositions = append(sqlPositions, position)
		}
	}
	if len(sqlBindings) != 0 {
		// One bounded batch reconstructs bindings from real complete owner rows,
		// never one identity/permission point read for every historical event.
		bindings, err := sqlevaluation.SQLHistoricalBatchBindings(ctx, joint.sql.facts, sqlBindings)
		if err != nil || len(bindings) != len(sqlPositions) {
			return nil, nil, nil, ErrCoordinatorCASQualification
		}
		for i, position := range sqlPositions {
			rows[position].bindingSHA = bindings[i]
		}
	}
	for _, row := range rows {
		entry, err := qualifiedCASEntry(c.binding, row, ai.verifiedAt)
		if err != nil {
			return nil, nil, nil, err
		}
		if row.facts.Source.Database == "mysql" {
			assessment, outcome, err := qualifiedCASOwnerIDs(row.facts, row.candidate)
			if err != nil {
				return nil, nil, nil, err
			}
			sqlAttachments = append(sqlAttachments, sqlevaluation.SQLHistoricalBatchAttachment{AssessmentID: assessment, OutcomeID: outcome, Entry: entry, ContentDigest: row.facts.ContentDigest})
		} else {
			mongoAttachments = append(mongoAttachments, MongoHistoricalBatchAttachment{Source: row.handle, Entry: entry})
		}
	}
	// Revalidate actual borrowed scopes after binding construction and before
	// either plan is exposed. A partially constructed plan never escapes.
	if _, err = c.qualifiedCASRows(ctx, joint, origin, ai); err != nil {
		return nil, nil, nil, err
	}
	var sqlPlan *sqlevaluation.SQLHistoricalBatchCASPlan
	var mongoPlan *MongoHistoricalBatchCASPlan
	if len(sqlAttachments) != 0 {
		sqlPlan, err = sqlevaluation.PrepareSQLHistoricalBatchCAS(ctx, joint.sql.facts, sqlAttachments)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if len(mongoAttachments) != 0 {
		mongoPlan, err = PrepareMongoHistoricalBatchCAS(ctx, joint.mongo, c.authenticated, mongoAttachments)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if _, err = c.qualifiedCASRows(ctx, joint, origin, ai); err != nil {
		return nil, nil, nil, err
	}
	sealed, err := c.SealHistoricalCASPersistencePage(ctx, joint, origin, ai, sqlPlan, mongoPlan)
	if err != nil {
		return nil, nil, nil, err
	}
	return sqlPlan, mongoPlan, sealed, nil
}

// This type is private, never serialized and has no public constructor. Only
// qualifiedCASRows supplies whole-source rows after its original proof checks.
// The fresh business-only constructor marks its native observation separately;
// those rows are rejected by the old whole-source evidence helper.
type qualifiedCASRow struct {
	// A scoped business result cannot enter the legacy whole-source evidence path.
	sourceObservation *HistoricalComponentSourceObservation
	handle            *VerifiedSourceEvent
	facts             *DecodedSourceEvent
	candidate         HistoricalCandidate
	bindingSHA        string
}

func (c *HistoricalCoordinator) qualifiedCASRows(ctx context.Context, joint *WholeSourceJointPage, origin *SourceOriginRecheckProof, ai *AIReverseFreshProof) ([]qualifiedCASRow, error) {
	if c == nil || ctx == nil || joint == nil || origin == nil || origin.second == nil || origin.first != nil || origin.firstAnchor == nil || joint.sql == nil || joint.sql.responsibility == nil || ai == nil || ai.self != ai || ai.anchor == nil || ai.old != nil || ai.fresh != nil || ai.verifiedAt.IsZero() {
		return nil, ErrCoordinatorCASQualification
	}
	// validateCurrent takes c.mu itself. Run its actual snapshot checks before
	// entering this metadata critical section, then bind every sealed field
	// again below; never recursively lock the coordinator.
	if ai.anchor.validateCurrent(ctx, joint.sql.responsibility, c) != nil {
		return nil, ErrCoordinatorCASQualification
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if !c.coverage || c.failed || c.authenticated == nil || !c.authenticated.complete || c.authenticated.entries != uint64(len(c.authenticated.rows)) || c.authenticated.entries != uint64(len(c.candidates)) || len(c.remaining) != 0 || c.receipts != c.authenticated.receipts {
		return nil, ErrCoordinatorIncomplete
	}
	if joint.owner != c || !joint.consumed || joint.page == nil || !joint.page.consumed || joint.page.owner != c || joint.index == nil || joint.index.owner != c || joint.index.auth != c.authenticated || !joint.index.complete || joint.catalog == nil || joint.sql == nil || joint.mongo == nil || joint.candidateStart < 0 || joint.candidateCount <= 0 || joint.candidateCount > 512 || joint.candidateStart+joint.candidateCount > len(c.candidates) || len(joint.current) != joint.candidateCount || (len(joint.page.rows) != 0 && len(joint.page.rows) != joint.candidateCount) || !coordinatorStoredCandidateMatches(c.candidates[joint.candidateStart:joint.candidateStart+joint.candidateCount], joint.candidateSHA) {
		return nil, ErrCoordinatorCASQualification
	}
	bindingSHA, err := wholeSourceJointBindingsHash(joint.bindings)
	if err != nil || bindingSHA != joint.bindingSHA || joint.ValidateBorrowedSnapshot(ctx) != nil || origin.second.validate(ctx) != nil || origin.firstAnchor.binding == nil || !origin.firstAnchor.intact() || origin.firstAnchor.binding.alive(ctx) != nil || ai.anchor.alive(ctx) != nil || ai.anchor.oldTransactionEnded(ctx) != nil {
		return nil, ErrCoordinatorCASQualification
	}
	// Link both real second-epoch proofs to the SAME original graphless seal,
	// exact four files/expectations and actual current SQL/Mongo snapshots.
	a, first, second := ai.anchor, origin.firstAnchor, origin.second
	if ai.coordinator != c || ai.current != joint.sql.responsibility || ai.currentCycle != joint.sql.responsibility.Report().CycleID || ai.currentCycle == a.sqlCycle || ai.typedFactsSHA != a.typedFactsSHA || ai.classificationSHA != a.classificationSHA || a.binding != c.binding || a.entries != c.authenticated.entries || a.receipts != c.receipts || second.binding.copies != c.authenticated || second.sql != joint.sql.responsibility.cycle || second.mongo != joint.mongo.global || second.binding.hash != a.origin.BindingSHA || second.binding.expected != a.origin.Expected || second.binding.fileHashes != a.origin.Files || second.binding.fileBytes != a.origin.FileBytes || second.receipts != c.receipts || second.boundaries != a.origin.Boundaries || first.sqlConnection != a.pool || first.sqlCycleID != a.origin.SQLCycle || first.originalEpochHash != a.origin.OriginalEpochSHA || first.seal != a.origin.OriginalSeal || !bytes.Equal(first.transaction.session, a.origin.Transaction.session) || first.transaction.number != a.origin.Transaction.number {
		return nil, ErrCoordinatorCASQualification
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Statement.ConnPool != ai.currentPool || tx.Statement.ConnPool != second.sqlConnection || tx.Statement.ConnPool == a.pool {
		return nil, ErrCoordinatorCASQualification
	}
	session := mongo.SessionFromContext(ctx)
	actual, err := mongoCycleTransaction(ctx, joint.mongo.global.db)
	if err != nil || session == nil || !bytes.Equal(actual.session, second.transaction.session) || actual.number != second.transaction.number || !bytes.Equal(actual.session, joint.mongo.global.txn.session) || actual.number != joint.mongo.global.txn.number || bytes.Equal(actual.session, first.transaction.session) && actual.number == first.transaction.number {
		return nil, ErrCoordinatorCASQualification
	}
	if a.ai.Unknown != 0 || a.ai.Blocking != 0 || len(a.ai.BlockingReasons) != 0 || !a.ai.WholeLedgerEOF || len(a.ai.Ledgers) != 14 || a.ai.SourceAuthenticationRequired || joint.sql.responsibility.Report().Unknown != 0 || joint.sql.responsibility.Report().Blocking != 0 || !qualifiedCASMongoGlobalKnown(joint.mongo.global.Report()) {
		return nil, ErrCoordinatorCASQualification
	}
	rows := make([]qualifiedCASRow, 0, joint.candidateCount)
	seen := map[verifiedSourceKey]bool{}
	for i, handle := range joint.current {
		// Ordinary consume releases page.rows; the retained genuine handles,
		// original index/auth facts and consumed candidate block stay exact.
		// A replay rebuilds actual rows, which must also match when present.
		if handle == nil || (len(joint.page.rows) != 0 && (joint.page.rows[i].event != handle || len(joint.page.rows[i].keys) != 1)) {
			return nil, ErrCoordinatorCASQualification
		}
		facts, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		hash, hashErr := privateFactsSHA(facts)
		observed, present := c.authenticated.rows[key]
		indexed, indexedPresent := joint.index.entries[facts.EventID]
		if err != nil || hashErr != nil || !present || !indexedPresent || seen[key] || (len(joint.page.rows) != 0 && key != joint.page.rows[i].keys[0]) || hash != observed.facts || hash != indexed.FactsSHA256 {
			return nil, ErrSourceAuthentication
		}
		seen[key] = true
		candidate, present := joint.values[key]
		stored := c.candidates[joint.candidateStart+i]
		if !present || stored == nil || coordinatorCandidateHash([]HistoricalCandidate{candidate}) != coordinatorCandidateHash([]HistoricalCandidate{*stored}) || qualifiedCASSourceMatches(facts, candidate) != nil {
			return nil, ErrCoordinatorCASQualification
		}
		row := qualifiedCASRow{handle: handle, facts: facts, candidate: coordinatorCloneCandidate(candidate)}
		if facts.Source.Database == "mysql" {
			local, err := joint.resolveSQL(ctx, facts)
			if err != nil || !local.OwnerLocalTerminal || len(local.BlockingReasons) != 0 || !reflect.DeepEqual(local.OriginalRun, candidate.ActualOriginalRun) || local.OrgID != facts.OrgID {
				return nil, ErrCoordinatorCASQualification
			}
			view, err := joint.mongo.ResponsibilitiesForSource(ctx, handle)
			if err != nil || len(view.BlockingReasons) != 0 {
				return nil, ErrCoordinatorCASQualification
			}
		} else {
			q, err := joint.originalMongoGraph(ctx, facts, key, joint.resolved)
			if err != nil || q == nil {
				return nil, ErrCoordinatorCASQualification
			}
			local := q.Local()
			view, err := joint.jointMongoSQLView(ctx, facts, local)
			if err != nil || !local.OwnerLocalTerminal || len(local.BlockingReasons) != 0 || len(view.BlockingReasons) != 0 || local.OrgID != facts.OrgID || !reflect.DeepEqual(local.OriginalRun, candidate.ActualOriginalRun) || local.BusinessBindingSHA256 != candidate.BusinessBindingSHA256 {
				return nil, ErrCoordinatorCASQualification
			}
			row.bindingSHA = local.BusinessBindingSHA256
		}
		// Numeric owner IDs are checked against authenticated original fields;
		// a nearest/current/latest Run or owner is never substituted here.
		if _, _, err := qualifiedCASOwnerIDs(facts, candidate); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if joint.ValidateBorrowedSnapshot(ctx) != nil || c.alive(ctx) != nil || ai.anchor.alive(ctx) != nil {
		return nil, ErrCoordinatorCASQualification
	}
	return rows, nil
}

func qualifiedCASSourceMatches(facts *DecodedSourceEvent, candidate HistoricalCandidate) error {
	if facts == nil || facts.Source.Object != "domain_event_outbox" || facts.OrgID == 0 || !candidate.LocalQualified || len(candidate.BlockingReasons) != 0 || candidate.Source != facts.Source || candidate.OriginalID != facts.EventID || candidate.EventType != facts.EventType || candidate.OrganizationID != strconv.FormatUint(facts.OrgID, 10) || candidate.ContentDigest != facts.ContentDigest {
		return ErrCoordinatorCASQualification
	}
	expected := coordinatorEventCandidate(facts)
	if candidate.OwnerDatabase != expected.OwnerDatabase || candidate.OwnerObject != expected.OwnerObject || candidate.OwnerID != expected.OwnerID || candidate.OwnerID == "" {
		return ErrCoordinatorCASQualification
	}
	return nil
}
