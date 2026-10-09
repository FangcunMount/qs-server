package retirement

import (
	"context"
	"sort"
	"strconv"
	"time"

	sheetdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/bson"
)

func (r *MongoOwnerResolution) block(reason string) {
	r.local.BlockingReasons = append(r.local.BlockingReasons, reason)
	r.local.OwnerLocalTerminal = false
}

func (r *MongoOwnerResolution) readSubmission(ctx context.Context) error {
	id, _ := strconv.ParseUint(r.source.Submitted.AnswerSheetID, 10, 64)
	var row sheetmongo.AnswerSheetPO
	raw, err := r.readOne(ctx, "answersheets", id, &row)
	if err != nil {
		return err
	}
	fields, _ := exactBSONFields(raw)
	for _, key := range []string{"org_id", "testee_id", "filler_id"} {
		v, ok := mongoExactInteger(fields[key])
		if !ok || v <= 0 {
			return ErrMongoOwnerResolution
		}
	}
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		return err
	}
	actual, err := eventevidencebinding.AnswerSheet(p)
	if err != nil {
		return ErrMongoOwnerResolution
	}
	wanted, err := eventevidencebinding.AnswerSheet(*r.source.Submitted)
	if err != nil || actual != wanted {
		return ErrMongoOwnerConflict
	}
	if err = checkMongoExistingHistory(row.LegacySubmissionEvidence, r.source, actual); err != nil {
		return err
	}
	if row.DurableAcceptance != nil && row.DurableAcceptance.EventEvidence != nil {
		if err = r.checkMongoExistingSingle(row.DurableAcceptance.EventEvidence, row.DurableAcceptance.EventID, actual, "answersheet.submitted"); err != nil {
			return err
		}
	}
	r.local.AnswerSheetID = id
	r.local.TesteeID = row.TesteeID
	r.local.BusinessBindingSHA256 = actual
	r.local.FrozenAdmissionPurpose = row.Admission.Purpose
	r.local.OwnerLocalTerminal = true
	if p.OrgID != r.source.OrgID {
		return ErrSourceOrganization
	}
	if row.DurableAcceptance != nil && (row.DurableAcceptance.SchemaVersion != 1 || row.DurableAcceptance.EventID != r.source.EventID || !BusinessTimeEqual(row.DurableAcceptance.AcceptedAt, row.FilledAt)) {
		return ErrMongoOwnerConflict
	}
	switch p.Admission.Purpose {
	case eventpayload.AdmissionPurposeIndependentQuestionnaire:
		if r.sqlFacts != nil {
			return ErrMongoOwnerConflict
		}
		// This is a positive frozen purpose, never an inference from SQL absence.
		r.local.Gaps = append(r.local.Gaps, "independent_admission_sql_absence_and_global_responsibility_not_checked")
	case eventpayload.AdmissionPurposeAssessment:
		if r.sqlFacts == nil || !r.sqlFacts.HasVerifiedAnswerSheetAssociation(id) {
			return ErrMongoOwnerResolution
		}
		facts := r.sqlFacts.Snapshot()
		o := facts.Owner
		if o.AnswerSheetID != id || o.OrgID != p.OrgID || o.TesteeID != p.TesteeID || o.QuestionnaireCode != p.QuestionnaireCode || o.QuestionnaireVersion != p.QuestionnaireVersion || o.ModelKind != p.Admission.ModelKind || o.ModelAlgorithm != p.Admission.ModelAlgorithm || o.ModelCode != p.Admission.ModelCode || o.ModelVersion != p.Admission.ModelVersion {
			return ErrMongoOwnerConflict
		}
		clock, err := mongoSQLSubmissionClock(o, p.SubmittedAt)
		if err != nil {
			return err
		}
		r.local.SQLSubmissionClock = clock
		if !clock.ExactBusinessMilliseconds {
			r.local.Gaps = append(r.local.Gaps, "storage_precision_gap")
		}
		r.local.AssessmentID = o.AssessmentID
		r.checkSQLTerminal(ctx, facts)
	default:
		return ErrMongoOwnerResolution
	}
	return nil
}

func mongoSQLSubmissionClock(owner sqlevaluation.SQLHistoricalOwner, original time.Time) (*MongoSQLSubmissionClockFact, error) {
	if owner.SubmittedAt == nil || original.IsZero() || owner.ActualSubmittedAtDataType != "datetime" {
		return nil, ErrMongoOwnerConflict
	}
	clock := &MongoSQLSubmissionClockFact{DataType: owner.ActualSubmittedAtDataType, Precision: owner.ActualSubmittedAtPrecision, ActualSQLTime: owner.SubmittedAt.UTC(), OriginalMongoTime: original.UTC().Truncate(time.Millisecond)}
	switch clock.Precision {
	case 0:
		clock.ComparisonRule = "sql-datetime0-utc-second-bucket/v1"
		if !clock.ActualSQLTime.Equal(clock.ActualSQLTime.Truncate(time.Second)) || !clock.ActualSQLTime.Equal(clock.OriginalMongoTime.Truncate(time.Second)) {
			return nil, ErrMongoOwnerConflict
		}
		// The column's lossy contract cannot certify complete original time.
	case 3, 6:
		clock.ComparisonRule = "cross-store-business-milliseconds/v1"
		clock.ExactBusinessMilliseconds = true
		if !BusinessTimeEqual(clock.ActualSQLTime, clock.OriginalMongoTime) {
			return nil, ErrMongoOwnerConflict
		}
	default:
		return nil, ErrMongoOwnerConflict
	}
	return clock, nil
}

func mongoSubmissionPayload(row sheetmongo.AnswerSheetPO) (eventpayload.AnswerSheetSubmittedData, error) {
	a := row.Admission
	if row.DomainID.IsZero() || row.OrgID == 0 || row.TesteeID == 0 || row.FillerID <= 0 || row.FillerType == "" || row.FilledAt.IsZero() || a == nil || a.QuestionnaireCode != row.QuestionnaireCode || a.QuestionnaireVersion != row.QuestionnaireVersion {
		return eventpayload.AnswerSheetSubmittedData{}, ErrMongoOwnerResolution
	}
	var admission sheetdomain.Admission
	var err error
	switch eventpayload.AdmissionPurpose(a.Purpose) {
	case eventpayload.AdmissionPurposeIndependentQuestionnaire:
		if a.ModelKind != "" || a.ModelSubKind != "" || a.ModelAlgorithm != "" || a.ModelCode != "" || a.ModelVersion != "" || a.ModelTitle != "" {
			return eventpayload.AnswerSheetSubmittedData{}, ErrMongoOwnerResolution
		}
		admission, err = sheetdomain.NewIndependentAdmission(a.QuestionnaireCode, a.QuestionnaireVersion)
	case eventpayload.AdmissionPurposeAssessment:
		admission, err = sheetdomain.NewAssessmentAdmission(a.QuestionnaireCode, a.QuestionnaireVersion, a.ModelKind, a.ModelSubKind, a.ModelAlgorithm, a.ModelCode, a.ModelVersion, a.ModelTitle)
	default:
		return eventpayload.AnswerSheetSubmittedData{}, ErrMongoOwnerResolution
	}
	if err != nil {
		return eventpayload.AnswerSheetSubmittedData{}, ErrMongoOwnerResolution
	}
	projected := admission.ToEventPayload()
	if string(projected.Purpose) != a.Purpose || projected.QuestionnaireCode != a.QuestionnaireCode || projected.QuestionnaireVersion != a.QuestionnaireVersion || projected.ModelKind != a.ModelKind || projected.ModelAlgorithm != a.ModelAlgorithm || projected.ModelCode != a.ModelCode || projected.ModelVersion != a.ModelVersion || projected.ModelTitle != a.ModelTitle {
		return eventpayload.AnswerSheetSubmittedData{}, ErrMongoOwnerResolution
	}
	p := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: row.DomainID.String(), QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, OrgID: row.OrgID, TesteeID: row.TesteeID, FillerID: uint64(row.FillerID), FillerType: row.FillerType, TaskID: row.TaskID, SubmittedAt: row.FilledAt, Admission: projected}
	if row.SubmitMeta != nil {
		p.RequestID = row.SubmitMeta.RequestID
	}
	if row.DurableAcceptance != nil && row.DurableAcceptance.RequestID != "" {
		if p.RequestID != "" && p.RequestID != row.DurableAcceptance.RequestID {
			return p, ErrMongoOwnerConflict
		}
		p.RequestID = row.DurableAcceptance.RequestID
	}
	if v := row.Attribution; v != nil {
		p.Attribution = &eventpayload.AttributionSnapshot{OriginType: v.OriginType, OriginID: v.OriginID, ClinicianID: v.ClinicianID, EntryID: v.EntryID, PlanID: v.PlanID, EnrollmentID: v.EnrollmentID, TaskID: v.TaskID, CapturedAt: v.CapturedAt, Version: v.Version, Mode: v.Mode}
	}
	return p, nil
}

// Only the actual opaque batch (including the private resolved-owner wrapper)
// may supply global original-Run absence. Editable snapshots and the legacy
// point-reader API are not an absence capability.
func (r *MongoOwnerResolution) originalSQLOutcomeRunAbsent(ctx context.Context, outcome sqlevaluation.SQLHistoricalOutcome) error {
	switch reader := r.sqlFacts.(type) {
	case *sqlevaluation.SQLHistoricalBatchOwnerFacts:
		return reader.OriginalOutcomeRunAbsent(ctx, outcome.ID, outcome.RunID)
	case *sqlMongoResolvedOwnerReader:
		if reader != nil && reader.actual != nil {
			return reader.actual.OriginalOutcomeRunAbsent(ctx, outcome.ID, outcome.RunID)
		}
	}
	return ErrMongoOwnerConflict
}

func (r *MongoOwnerResolution) checkSQLTerminal(ctx context.Context, facts sqlevaluation.SQLHistoricalFactsSnapshot) {
	o := facts.Owner
	if o.AssessmentID == 0 || o.OrgID != r.source.OrgID || o.TesteeID != r.local.TesteeID || (o.Status != "evaluated" && o.Status != "failed") {
		r.block("sql_business_owner_unfinished_or_conflicting")
	}
	resources := map[string]bool{}
	attempts := map[uint]sqlevaluation.SQLHistoricalRun{}
	for _, run := range facts.Runs {
		if run.Scope != "evaluation_run" || run.AssessmentID != o.AssessmentID || run.Attempt == 0 || run.ResourceID == "" || resources[run.ResourceID] || attempts[run.Attempt].ID != 0 {
			r.block("sql_runtime_identity_ambiguous")
		}
		resources[run.ResourceID] = true
		attempts[run.Attempt] = run
		if (run.Status != "succeeded" && run.Status != "failed") || run.FinishedAt == nil || run.LeaseExpiresAt != nil {
			r.block("sql_runtime_responsibility_unfinished")
		}
	}
	for _, run := range facts.Runs {
		if run.Status == "failed" {
			switch run.RetryDisposition {
			case "automatic", "manual_required":
				if attempts[run.Attempt+1].ID == 0 {
					r.block("sql_retry_responsibility_unclosed")
				}
			case "terminal":
				if run.NextAttemptAt != nil {
					r.block("sql_terminal_retry_clock_present")
				}
			case "":
				if run.Retryable {
					r.block("sql_retry_disposition_missing")
				}
			default:
				r.block("sql_retry_disposition_unknown")
			}
		}
	}
	if o.Status == "evaluated" && len(facts.Outcomes) != 1 {
		r.block("sql_outcome_absent_or_ambiguous")
	}
	missingOriginal := map[uint64]bool{}
	for _, outcome := range facts.Outcomes {
		if outcome.Invalid || outcome.AssessmentID != o.AssessmentID || outcome.OrgID != o.OrgID || outcome.TesteeID != o.TesteeID || outcome.RunID == "" {
			r.block("sql_original_outcome_runtime_conflict")
		} else if !resources[outcome.RunID] {
			if o.Status == "evaluated" && len(facts.Outcomes) == 1 && r.originalSQLOutcomeRunAbsent(ctx, outcome) == nil {
				missingOriginal[outcome.ID] = true
			} else {
				r.block("sql_original_outcome_runtime_conflict")
			}
		}
	}
	if len(facts.Runs) == 0 && (o.Status != "evaluated" || len(facts.Outcomes) != 1 || !missingOriginal[facts.Outcomes[0].ID]) {
		r.block("sql_runtime_history_absent")
	}
	ownerClock := func(actual *time.Time, original time.Time, dataType string, precision int) bool {
		matched, gap := sqlLocalOwnerClock(actual, original, dataType, precision, o.ClockComparisonRuleVersion)
		if gap {
			found := false
			for _, existing := range r.local.Gaps {
				if existing == "storage_precision_gap" {
					found = true
				}
			}
			if !found {
				r.local.Gaps = append(r.local.Gaps, "storage_precision_gap")
			}
		}
		return matched
	}
	switch o.Status {
	case "evaluated":
		if o.EvaluatedAt == nil || len(facts.Outcomes) != 1 {
			r.block("sql_evaluated_owner_canonical_facts_missing")
		} else {
			outcome := facts.Outcomes[0]
			if !ownerClock(o.EvaluatedAt, outcome.EvaluatedAt, o.ActualEvaluatedAtDataType, o.ActualEvaluatedAtPrecision) {
				r.block("sql_evaluated_owner_commit_clock_conflict")
			}
			closed := 0
			for _, run := range facts.Runs {
				if run.ResourceID == outcome.RunID && run.Status == "succeeded" && run.FinishedAt != nil && BusinessTimeEqual(*run.FinishedAt, outcome.EvaluatedAt) && !outcome.Invalid {
					closed++
				}
			}
			if closed != 1 && (closed != 0 || !missingOriginal[outcome.ID]) {
				r.block("sql_evaluated_owner_original_success_unproven")
			}
		}
	case "failed":
		if o.FailedAt == nil || len(facts.Outcomes) != 0 {
			r.block("sql_failed_owner_canonical_facts_conflict")
		} else {
			currentFailures := 0
			for _, run := range facts.Runs {
				if run.Status == "failed" && run.FinishedAt != nil && ownerClock(o.FailedAt, *run.FinishedAt, o.ActualFailedAtDataType, o.ActualFailedAtPrecision) {
					currentFailures++
				}
			}
			// This is only the current owner. It never fills an undeclared
			// original source Run or chooses a recent/nearest execution.
			if currentFailures != 1 {
				r.block("sql_failed_owner_current_terminal_run_unproven")
			}
		}
	}
	for _, responsibility := range facts.Responsibilities {
		if responsibility.Invalid || responsibility.Unfinished || responsibility.LeasePresent || responsibility.OrgID != o.OrgID || responsibility.AssessmentID != 0 && responsibility.AssessmentID != o.AssessmentID || responsibility.TesteeID != 0 && responsibility.TesteeID != o.TesteeID {
			r.block("sql_current_responsibility_unclosed")
		}
	}
	// The gap is retained only after exact canonical clocks/identity and every
	// current pending/lease/retry check passed. It never supplies a Run/attempt
	// or erases a conflicting retained row, even outside the selected owner.
	if r.local.OwnerLocalTerminal && len(r.local.BlockingReasons) == 0 && len(missingOriginal) != 0 {
		r.local.Gaps = append(r.local.Gaps, "original_outcome_run_absent")
	}
}

func (r *MongoOwnerResolution) readGenerated(ctx context.Context) error {
	if r.sqlFacts == nil {
		return ErrMongoOwnerResolution
	}
	indexes, indexErr := r.readArtifactIndexes(ctx)
	if indexErr != nil {
		return indexErr
	}
	r.metadata.artifactIndexes = indexes
	unique := false
	for _, raw := range r.metadata.artifactIndexes {
		var index struct {
			Key     bson.D `bson:"key"`
			Unique  bool   `bson:"unique"`
			Sparse  bool   `bson:"sparse"`
			Partial bson.D `bson:"partialFilterExpression"`
		}
		if bson.Unmarshal(raw, &index) != nil {
			return ErrMongoOwnerConflict
		}
		if index.Unique && !index.Sparse && len(index.Partial) == 0 && len(index.Key) == 1 && index.Key[0].Key == "generation_id" && (index.Key[0].Value == int32(1) || index.Key[0].Value == int64(1)) {
			unique = true
		}
	}
	if !unique {
		return ErrMongoOwnerConflict
	}
	p := r.source.Generated
	gid, _ := strconv.ParseUint(p.GenerationID, 10, 64)
	rid, _ := strconv.ParseUint(p.RunID, 10, 64)
	aid, _ := strconv.ParseUint(p.ReportID, 10, 64)
	oid, _ := strconv.ParseUint(p.OutcomeID, 10, 64)
	assessment, _ := strconv.ParseUint(p.AssessmentID, 10, 64)
	var g interpretmongo.ReportGenerationPO
	if _, err := r.readOne(ctx, "report_generations", gid, &g); err != nil {
		return err
	}
	if g.OutcomeID != oid || g.Status != "generated" || g.Version == 0 || g.LatestRunID == 0 || g.ReportID == 0 {
		return ErrMongoOwnerResolution
	}
	fact, err := r.sqlFacts.OutcomeRecord(oid)
	if err != nil || fact == nil {
		return ErrMongoOwnerResolution
	}
	if fact.ID().Uint64() != oid || fact.AssessmentID().Uint64() != assessment || fact.OrgID() != p.OrgID || fact.TesteeID() != p.TesteeID {
		return ErrSourceOrganization
	}
	facts := r.sqlFacts.Snapshot()
	if facts.Owner.AssessmentID != assessment || facts.Owner.OrgID != r.source.OrgID || facts.Owner.TesteeID != p.TesteeID {
		return ErrSourceOrganization
	}
	r.local.TesteeID = p.TesteeID
	r.local.AssessmentID = assessment
	r.local.GenerationID = gid
	r.local.ReportID = aid
	r.local.OutcomeID = oid
	r.local.OwnerLocalTerminal = true
	r.checkSQLTerminal(ctx, facts)
	graph := func(reportID, runID uint64) (string, error) {
		var a interpretmongo.InterpretReportPO
		var run interpretmongo.InterpretationRunPO
		if _, err := r.readOne(ctx, "interpret_report_artifacts", reportID, &a); err != nil {
			return "", err
		}
		if _, err := r.readOne(ctx, "interpretation_runs", runID, &run); err != nil {
			return "", err
		}
		original, err := interpretmongo.HistoricalGeneratedPayload(g, a, run, fact)
		if err != nil {
			return "", ErrMongoOwnerConflict
		}
		m := fact.Model()
		if a.Model.Kind != string(m.Kind) || a.Model.Algorithm != string(m.Algorithm) || a.Model.Code != m.Code || a.Model.Version != m.Version || a.Model.Title != m.Title {
			return "", ErrMongoOwnerConflict
		}
		return eventevidencebinding.Generated(original)
	}
	// Current winner is checked solely for current lifecycle responsibility.
	if _, err := graph(g.ReportID, g.LatestRunID); err != nil {
		return err
	}
	original, err := graph(aid, rid)
	if err != nil {
		return err
	}
	wanted, err := eventevidencebinding.Generated(*p)
	if err != nil || wanted != original {
		return ErrMongoOwnerConflict
	}
	if err = checkMongoExistingHistory(g.HistoricalGeneratedEvidence, r.source, original); err != nil {
		return err
	}
	if g.GeneratedEventEvidence != nil {
		if err = r.checkMongoExistingSingle(g.GeneratedEventEvidence, g.GeneratedEventID, original, "interpretation.report.generated"); err != nil {
			return err
		}
	}
	r.local.BusinessBindingSHA256 = original
	r.local.OriginalRun = &evidence.HistoricalRunReferenceV1{RunID: p.RunID, Attempt: p.Attempt}
	if err = r.readGenerationReverse(ctx, oid, gid, aid, rid); err != nil {
		return err
	}
	return nil
}

func (r *MongoOwnerResolution) checkMongoExistingSingle(proof *evidence.EventEvidenceV1, eventID, binding, eventType string) error {
	if proof == nil || proof.Validate() != nil || proof.EventID != eventID || proof.BusinessBindingSHA256 != binding {
		return ErrMongoOwnerConflict
	}
	if proof.Reference != nil {
		if proof.Reference.EventType != eventType || proof.Reference.Scope != "org:"+strconv.FormatUint(r.source.OrgID, 10) {
			return ErrMongoOwnerConflict
		}
		r.expectedStandard = append(r.expectedStandard, *proof.Reference)
	}
	return nil
}

func checkMongoExistingHistory(set *evidence.HistoricalReferenceSetV1, source *DecodedSourceEvent, binding string) error {
	if set == nil {
		return nil
	}
	if set.Validate() != nil {
		return ErrMongoOwnerConflict
	}
	for _, entry := range set.Entries {
		if entry.EventType != source.EventType || entry.Proof.BusinessBindingSHA256 != binding {
			return ErrMongoOwnerConflict
		}
		if entry.EventID == source.EventID && entry.Source != source.Source {
			return ErrMongoOwnerConflict
		}
	}
	return nil
}

func (r *MongoOwnerResolution) readGenerationReverse(ctx context.Context, outcome, generation, artifact, run uint64) error {
	gens, err := r.readRows(ctx, "report_generations", bson.D{{Key: "outcome_id", Value: int64(outcome)}})
	if err != nil {
		return err
	}
	known := map[uint64]interpretmongo.ReportGenerationPO{}
	for _, raw := range gens {
		var g interpretmongo.ReportGenerationPO
		if bson.Unmarshal(raw, &g) != nil || g.DomainID.IsZero() || g.OutcomeID != outcome || g.DeletedAt != nil {
			return ErrMongoOwnerConflict
		}
		if _, exists := known[g.DomainID.Uint64()]; exists {
			return ErrMongoOwnerConflict
		}
		known[g.DomainID.Uint64()] = g
		if g.Status != "generated" || g.ReportID == 0 || g.LatestRunID == 0 {
			r.block("related_generation_responsibility_unfinished")
		}
	}
	if _, ok := known[generation]; !ok {
		return ErrMongoOwnerConflict
	}
	artifacts, err := r.readRows(ctx, "interpret_report_artifacts", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "outcome_id", Value: int64(outcome)}}, bson.D{{Key: "generation_id", Value: int64(generation)}}}}})
	if err != nil {
		return err
	}
	ids := map[uint64]bool{}
	artifactByID := map[uint64]interpretmongo.InterpretReportPO{}
	fact, err := r.sqlFacts.OutcomeRecord(outcome)
	if err != nil {
		return ErrMongoOwnerConflict
	}
	model := fact.Model()
	perRun := map[uint64]bool{}
	original := 0
	for _, raw := range artifacts {
		var a interpretmongo.InterpretReportPO
		if bson.Unmarshal(raw, &a) != nil || a.DomainID.IsZero() || ids[a.DomainID.Uint64()] || a.DeletedAt != nil || a.OutcomeID != outcome || known[a.GenerationID].DomainID.IsZero() || a.OrgID <= 0 || uint64(a.OrgID) != r.local.OrgID || a.AssessmentID != r.local.AssessmentID || a.TesteeID != r.local.TesteeID {
			return ErrMongoOwnerConflict
		}
		ids[a.DomainID.Uint64()] = true
		g := known[a.GenerationID]
		if a.ReportType != g.ReportType || a.TemplateVersion != g.TemplateVersion || a.Model == nil || a.Model.Kind != string(model.Kind) || a.Model.Algorithm != string(model.Algorithm) || a.Model.Code != model.Code || a.Model.Version != model.Version || a.Model.Title != model.Title {
			return ErrMongoOwnerConflict
		}
		artifactByID[a.DomainID.Uint64()] = a
		if a.GenerationID == generation {
			if perRun[a.InterpretationRunID] {
				return ErrMongoOwnerConflict
			}
			perRun[a.InterpretationRunID] = true
			if a.DomainID.Uint64() == artifact && a.InterpretationRunID == run {
				original++
			}
		}
	}
	if original != 1 {
		return ErrMongoOwnerConflict
	}
	gids := make([]uint64, 0, len(known))
	runIDs := map[uint64]bool{}
	for gid := range known {
		gids = append(gids, gid)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })
	for _, gid := range gids {
		g := known[gid]
		rows, err := r.readRows(ctx, "interpretation_runs", bson.D{{Key: "generation_id", Value: int64(gid)}})
		if err != nil {
			return err
		}
		attempts := map[int]bool{}
		generationRuns := map[uint64]interpretmongo.InterpretationRunPO{}
		latest := false
		for _, raw := range rows {
			var run interpretmongo.InterpretationRunPO
			if bson.Unmarshal(raw, &run) != nil || run.DomainID.IsZero() || run.GenerationID != gid || run.Attempt <= 0 || attempts[run.Attempt] || run.DeletedAt != nil || runIDs[run.DomainID.Uint64()] {
				return ErrMongoOwnerConflict
			}
			attempts[run.Attempt] = true
			runIDs[run.DomainID.Uint64()] = true
			generationRuns[run.DomainID.Uint64()] = run
			if run.DomainID.Uint64() == g.LatestRunID {
				latest = true
			}
			if (run.Status != "succeeded" && run.Status != "failed") || run.FinishedAt == nil || run.LeaseExpiresAt != nil || (run.Status == "succeeded" && (run.NextAttemptAt != nil || run.RetryDisposition != "" && run.RetryDisposition != "terminal")) {
				r.block("interpretation_run_execution_unfinished")
			}
			if run.Status == "failed" && (run.RetryDisposition != "terminal" || run.NextAttemptAt != nil) {
				r.block("interpretation_run_retry_unclosed")
			}
		}
		if !latest {
			return ErrMongoOwnerConflict
		}
		if !ids[g.ReportID] {
			return ErrMongoOwnerConflict
		}
		winner := artifactByID[g.ReportID]
		if winner.GenerationID != gid || winner.InterpretationRunID != g.LatestRunID || generationRuns[g.LatestRunID].Status != "succeeded" {
			return ErrMongoOwnerConflict
		}
		for _, a := range artifactByID {
			if a.GenerationID == gid {
				originalRun, found := generationRuns[a.InterpretationRunID]
				if !found || originalRun.Status != "succeeded" {
					return ErrMongoOwnerConflict
				}
			}
		}
	}
	return nil
}
