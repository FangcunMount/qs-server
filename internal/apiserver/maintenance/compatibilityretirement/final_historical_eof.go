package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/mongo"
)

const ErrFinalHistoricalEOF SourceError = "final_historical_persistence_or_eof_rejected"

// The host supplies approved read constraints, original files and an actual
// borrowed paired RR-RO/snapshot context. No result, count, complete bit, CAS
// receipt or external qualification can be imported through this input.
type FinalHistoricalInput struct {
	Binding       HistoricalCoordinatorBinding
	SQLIdentity   string
	SQLHead       uint64
	MongoDatabase *mongo.Database
	MongoConfig   MongoOwnerConfig
	Copies        []SourceCopyInput
	External      *AIExternalExecutionInput
}

type FinalHistoricalSummary struct {
	SourceSHA, OperationID, SourcesSHA256, PersistedReferencesSHA256 string
	EventReferences, AICommands, SourceReferences                    uint64
	UnverifiableReferences                                           uint64
	WholeWriterFence, CASAuthority, DropReady                        bool
}

// This process-local observation is produced only by the real native scans.
// It is a readback; it never certifies the caller's fence or enables DROP.
type FinalHistoricalObservation struct {
	self    *FinalHistoricalObservation
	sql     *SQLResponsibilitySnapshot
	mongo   *MongoResponsibilitySnapshot
	reverse *AIReverseSnapshot
	report  FinalHistoricalSummary
	seal    string
}

func (*FinalHistoricalObservation) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*FinalHistoricalObservation) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*FinalHistoricalObservation) String() string {
	return "opaque native final historical readback; no DROP authority"
}
func (o *FinalHistoricalObservation) Summary() FinalHistoricalSummary {
	if o == nil || o.self != o || o.seal != aiJSONHash(o.report) {
		return FinalHistoricalSummary{}
	}
	return o.report
}
func (o *FinalHistoricalObservation) ValidateBorrowedSnapshot(ctx context.Context) error {
	if o == nil || o.self != o || o.sql == nil || o.mongo == nil || o.reverse == nil || o.seal != aiJSONHash(o.report) || o.sql.ValidateBorrowedSnapshot(ctx) != nil || o.mongo.ValidateBorrowedSnapshot(ctx) != nil || o.reverse.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrFinalHistoricalEOF
	}
	return nil
}

func finalSourceCopies(copies []SourceCopyInput) ([]SourceCopyInput, error) {
	if len(copies) != 4 {
		return nil, ErrFinalHistoricalEOF
	}
	out := make([]SourceCopyInput, 4)
	for i, copy := range copies {
		at, ok := copy.Input.(io.ReaderAt)
		if !ok {
			return nil, ErrFinalHistoricalEOF
		}
		out[i] = SourceCopyInput{Input: io.NewSectionReader(at, 0, math.MaxInt64), Expected: copy.Expected}
	}
	return out, nil
}

// PrepareFinalHistoricalEOF performs no database writes, creates no new event
// or command ID and never calls a CAS Apply/Record. The existing CAS builders
// are used only for their native read constraints; their plans are discarded.
func PrepareFinalHistoricalEOF(ctx context.Context, in FinalHistoricalInput) (*FinalHistoricalObservation, error) {
	copies, e := finalSourceCopies(in.Copies)
	if e != nil || ctx == nil || ctx.Err() != nil || !evidenceHash(in.SQLIdentity) || in.SQLHead == 0 || in.MongoDatabase == nil {
		return nil, ErrFinalHistoricalEOF
	}
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 16
	c, e := PrepareHistoricalCoordinator(ctx, in.Binding, copies, limits)
	if e != nil {
		return nil, e
	}
	copies, _ = finalSourceCopies(in.Copies)
	originBinding, e := c.BindOriginCopies(ctx, copies, DefaultSourceOriginLimits())
	if e != nil {
		return nil, e
	}
	copies, _ = finalSourceCopies(in.Copies)
	jointCopies := make([]WholeSourceJointCopy, 4)
	for i, v := range copies {
		at, ok := v.Input.(io.ReaderAt)
		if !ok {
			return nil, ErrFinalHistoricalEOF
		}
		jointCopies[i] = WholeSourceJointCopy{Input: at, Expected: v.Expected}
	}
	jointLimits := DefaultWholeSourceJointLimits()
	jointLimits.MaxRelatedSources = 512
	index, e := c.PrepareWholeSourceJointIndex(ctx, jointCopies, jointLimits)
	if e != nil {
		return nil, e
	}
	current, e := PrepareSQLResponsibilitySnapshot(ctx, in.SQLIdentity, sqlevaluation.DefaultSQLResponsibilityLimits())
	if e != nil {
		return nil, e
	}
	global, e := PrepareMongoResponsibilitySnapshot(ctx, in.MongoDatabase, in.MongoConfig, MongoResponsibilityLimits{PageRows: 512, MaxRows: 2_000_000, MaxBytes: 8 << 30, MaxPages: 2_000_000, MaxGraphEntries: 4_000_000, MaxGraphBytes: 1 << 30, MaxDuration: 5 * time.Minute})
	if e != nil {
		return nil, e
	}
	if len(current.Report().Ledgers) != 8 || current.Report().Unknown != 0 || current.Report().Blocking != 0 || !current.Report().ActualTransactionReadOnlyRR || !qualifiedCASMongoGlobalKnown(global.Report()) || len(global.Report().Collections) != 11 {
		return nil, ErrFinalHistoricalEOF
	}
	copies, _ = finalSourceCopies(in.Copies)
	readers := make([]io.Reader, 4)
	for i, v := range copies {
		readers[i] = v.Input
	}
	if _, e = PrepareSourceOriginSnapshotEpoch(ctx, originBinding, current, global, readers); e != nil {
		return nil, e
	}
	catalog, e := PrepareSQLCrossStoreResponsibilityCatalog(ctx, current, sqlevaluation.DefaultSQLCrossStoreLimits())
	if e != nil {
		return nil, e
	}
	copies, _ = finalSourceCopies(in.Copies)
	local, e := c.PrepareAIReadOnlyResolver(ctx, current, in.SQLHead, copies[1:3])
	if e != nil {
		return nil, e
	}
	reverse, e := PrepareAIReverseSnapshot(ctx, current, in.SQLHead, DefaultAIReverseLimits())
	if e != nil {
		return nil, e
	}
	copies, _ = finalSourceCopies(in.Copies)
	if e = c.BindAIReverseSourceScope(ctx, reverse, copies); e != nil {
		return nil, e
	}
	r := reverse.Summary()
	if !r.WholeLedgerEOF || len(r.Ledgers) != 14 || r.Unknown != 0 || r.Blocking != 0 || len(r.BlockingReasons) != 0 {
		return nil, ErrFinalHistoricalEOF
	}
	var external *AIExternalExecutionQualification
	if in.Copies[1].Expected.Records+in.Copies[2].Expected.Records != 0 {
		if in.External == nil {
			return nil, ErrAIExternalInput
		}
		input := *in.External
		if e = RequireAIExternalExecQuiescence(ctx, AIExternalExecQuiescenceInput{OperationDirectory: input.OperationDirectory, SourceSHA: in.Binding.SourceSHA, OperationID: in.Binding.OperationID, RuntimeSourceSHA: input.RuntimeSourceSHA, ImageID: input.ImageID, ContainerID: input.ContainerID, SudoDocker: input.SudoDocker}); e != nil {
			return nil, e
		}
		external, e = c.PrepareAIFinalExternalExecution(ctx, local, reverse, input)
		if e != nil {
			return nil, e
		}
	}
	report := FinalHistoricalSummary{SourceSHA: in.Binding.SourceSHA, OperationID: in.Binding.OperationID}
	observed := sha256.New()
	var aiPages []*AIExternalPageQualification
	for {
		page, e := c.NextPage(ctx)
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		events, e := page.Events()
		if e != nil {
			return nil, e
		}
		if len(events) == 0 {
			if external == nil {
				return nil, ErrAIExternalInput
			}
			qualified, e := c.PrepareAICommandPersistencePage(ctx, page, external)
			if e != nil {
				return nil, e
			}
			count, gaps, hashes, e := finalVerifyAIPage(ctx, qualified)
			if e != nil {
				return nil, e
			}
			if e = c.QualifyAIReadOnlyPage(ctx, page, local); e != nil {
				return nil, e
			}
			report.AICommands += count
			report.UnverifiableReferences += gaps
			for _, hash := range hashes {
				_, _ = observed.Write([]byte(hash + "\n"))
			}
			aiPages = append(aiPages, qualified)
			continue
		}
		selectors, e := MongoHistoricalSQLBatchSelectors(events)
		if e != nil {
			return nil, e
		}
		batch, e := PrepareSQLBusinessOwnerBatch(ctx, current, selectors, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return nil, e
		}
		joint, e := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, batch, global)
		if e != nil {
			return nil, e
		}
		if e = c.QualifyWholeSourceJointPage(ctx, page, joint); e != nil {
			return nil, e
		}
		count, gaps, hashes, e := finalVerifyEventPage(ctx, c, joint, events)
		if e != nil {
			return nil, e
		}
		report.EventReferences += count
		report.UnverifiableReferences += gaps
		for _, hash := range hashes {
			_, _ = observed.Write([]byte(hash + "\n"))
		}
	}
	receipt := c.Receipt()
	if !receipt.SourceCoverageComplete || report.EventReferences != in.Copies[0].Expected.Records+in.Copies[3].Expected.Records || report.AICommands != in.Copies[1].Expected.Records || c.pending != nil || len(c.remaining) != 0 {
		return nil, ErrFinalHistoricalEOF
	}
	for _, v := range receipt.ConsumedRecords {
		report.SourceReferences += v
	}
	// This genuine read-only seal checks every authenticated AI source row and
	// unique original command exactly once. It is never recorded or applied.
	if _, e = c.SealAICommandPersistencePages(ctx, aiPages); e != nil {
		return nil, e
	}
	report.SourcesSHA256 = aiJSONHash(c.receipts)
	report.PersistedReferencesSHA256 = hex.EncodeToString(observed.Sum(nil))
	o := &FinalHistoricalObservation{sql: current, mongo: global, reverse: reverse, report: report}
	o.self = o
	o.seal = aiJSONHash(report)
	if o.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrFinalHistoricalEOF
	}
	return o, nil
}

func finalVerifyEventPage(ctx context.Context, c *HistoricalCoordinator, joint *WholeSourceJointPage, events []*VerifiedSourceEvent) (uint64, uint64, []string, error) {
	var sqlAttachments []sqlevaluation.SQLHistoricalBatchAttachment
	var mongoAttachments []MongoHistoricalBatchAttachment
	var gaps uint64
	var hashes []string
	for _, handle := range events {
		facts, e := handle.Facts()
		if e != nil {
			return 0, 0, nil, e
		}
		candidate, e := joint.Candidate(ctx, handle)
		if e != nil {
			return 0, 0, nil, e
		}
		assessment, outcome, e := qualifiedCASOwnerIDs(facts, candidate)
		if e != nil {
			return 0, 0, nil, e
		}
		var stored evidence.HistoricalReferenceEntryV1
		var binding string
		if facts.Source.Database == "mysql" {
			stored, e = sqlevaluation.SQLHistoricalStoredReference(ctx, joint.sql.facts, assessment, outcome, facts.EventID)
			if e == nil {
				binding, e = sqlevaluation.SQLHistoricalBatchBinding(ctx, joint.sql.facts, assessment, outcome, facts.EventType, candidate.ActualOriginalRun)
			}
		} else {
			q, qe := joint.mongo.ResolveSource(ctx, handle)
			if qe != nil {
				return 0, 0, nil, qe
			}
			local := q.Local()
			binding = local.BusinessBindingSHA256
			name, slot, id := "answersheets", "legacy_submission_evidence", local.AnswerSheetID
			if facts.EventType == "interpretation.report.generated" {
				name, slot, id = "report_generations", "historical_generated_evidence", local.GenerationID
			}
			raw, re := mongoCASRow(joint.mongo.data, name, id)
			if re != nil {
				return 0, 0, nil, re
			}
			set, se := mongoCASSet(raw, slot)
			if se != nil || set == nil {
				return 0, 0, nil, ErrFinalHistoricalEOF
			}
			found := false
			for _, entry := range set.Entries {
				if entry.EventID == facts.EventID {
					if found {
						return 0, 0, nil, ErrSourceIdentity
					}
					stored = entry.Clone()
					found = true
				}
			}
			if !found {
				return 0, 0, nil, ErrFinalHistoricalEOF
			}
		}
		if e != nil || stored.Proof == nil || stored.Validate() != nil {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		if finalMatchEventEntry(c.binding, qualifiedCASRow{handle: handle, facts: facts, candidate: candidate, bindingSHA: binding}, stored) != nil {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		if stored.Proof.Class == evidence.Unverifiable {
			gaps++
		}
		if facts.Source.Database == "mysql" {
			sqlAttachments = append(sqlAttachments, sqlevaluation.SQLHistoricalBatchAttachment{AssessmentID: assessment, OutcomeID: outcome, Entry: stored, ContentDigest: facts.ContentDigest})
		} else {
			mongoAttachments = append(mongoAttachments, MongoHistoricalBatchAttachment{Source: handle, Entry: stored})
		}
		hashes = append(hashes, aiJSONHash(stored))
	}
	if len(sqlAttachments) > 0 {
		if _, e := sqlevaluation.PrepareSQLHistoricalBatchCAS(ctx, joint.sql.facts, sqlAttachments); e != nil {
			return 0, 0, nil, e
		}
	}
	if len(mongoAttachments) > 0 {
		if _, e := PrepareMongoHistoricalBatchCAS(ctx, joint.mongo, c.authenticated, mongoAttachments); e != nil {
			return 0, 0, nil, e
		}
	}
	if joint.ValidateBorrowedSnapshot(ctx) != nil {
		return 0, 0, nil, ErrFinalHistoricalEOF
	}
	return uint64(len(events)), gaps, hashes, nil
}

func finalVerifyAIPage(ctx context.Context, p *AIExternalPageQualification) (uint64, uint64, []string, error) {
	if p == nil || p.self != p || p.seal != p.persistenceDigest() || p.owner == nil || p.owner.reverse.ValidateBorrowedSnapshot(ctx) != nil {
		return 0, 0, nil, ErrFinalHistoricalEOF
	}
	all := append([]store.CommandRetirementEvidence(nil), p.evidence...)
	if p.handoff != nil {
		all = append(all, p.handoff.evidence...)
	}
	var gaps uint64
	var hashes []string
	columns := aiReverseSpecByTable("ai_messaging_operations").columns
	for _, expected := range all {
		node := p.owner.reverse.byTable["ai_messaging_operations"][expected.CommandID]
		if node == nil {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		spec := aiReverseSpecByTable("ai_messaging_operations")
		projection, _ := aiCommandHandoffProjection(spec)
		rows, names, _, readErr := p.owner.reverse.read(ctx, "SELECT "+projection+" FROM `ai_messaging_operations` FORCE INDEX(PRIMARY) WHERE `command_id` = ? LIMIT 2", 2, expected.CommandID)
		if readErr != nil || len(rows) != 1 || !reflect.DeepEqual(names, columns) || aiReverseRowSHA(columns, rows[0]) != node.observation.RowSHA256 {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		row := rows[0]
		var actual store.CommandRetirementEvidence
		raw := row["retirement_evidence"]
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if strictJSON(raw) != nil || d.Decode(&actual) != nil || d.Decode(new(json.RawMessage)) != io.EOF || store.ValidateCommandRetirementObservation(actual) != nil {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		// Preserve historical time and the original Q manifest fingerprint. A
		// fresh actual Q separately re-verifies closure; it never relabels the
		// stored earlier observation as having used the current run or Q hash.
		if finalMatchRetirementEvidence(expected, actual) != nil {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		if expected.Conclusion == "transferred_verified" {
			if node.retired || row.text("retired") != "0" || row["retired_at"] != nil {
				return 0, 0, nil, ErrFinalHistoricalEOF
			}
		} else if aiCommandRetirementOperationExpected(columns, row, actual) != nil {
			return 0, 0, nil, ErrFinalHistoricalEOF
		}
		if expected.Conclusion == "unverifiable" {
			gaps++
		}
		hashes = append(hashes, aiJSONHash(actual))
	}
	return uint64(len(all)), gaps, hashes, nil
}

func finalMatchEventEntry(binding HistoricalCoordinatorBinding, row qualifiedCASRow, stored evidence.HistoricalReferenceEntryV1) error {
	if stored.Proof == nil || stored.Validate() != nil {
		return ErrFinalHistoricalEOF
	}
	expected, e := qualifiedCASEntry(binding, row, stored.Proof.Verification.VerifiedAt)
	if e != nil || !reflect.DeepEqual(stored, expected) {
		return ErrFinalHistoricalEOF
	}
	return nil
}

func finalMatchRetirementEvidence(expected, actual store.CommandRetirementEvidence) error {
	if store.ValidateCommandRetirementObservation(actual) != nil {
		return ErrFinalHistoricalEOF
	}
	expected.VerifiedAt = actual.VerifiedAt
	expected.References = append([]store.CommandRetirementReference(nil), expected.References...)
	if expected.Conclusion != "transferred_verified" {
		if len(expected.References) != len(actual.References) {
			return ErrFinalHistoricalEOF
		}
		for i, ref := range expected.References {
			if ref.Kind == "migration_manifest" {
				if actual.References[i].Kind != ref.Kind || !evidenceHash(actual.References[i].ID) {
					return ErrFinalHistoricalEOF
				}
				expected.References[i] = actual.References[i]
			}
		}
	}
	if !reflect.DeepEqual(expected, actual) {
		return ErrFinalHistoricalEOF
	}
	return nil
}
