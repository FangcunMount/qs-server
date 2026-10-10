package retirement

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// Exact fixed causes emitted by the actual six-type source/business resolvers.
// They represent historical retention/precision gaps after terminal ownership
// and current obligations have been independently proved. Unknown new causes
// fail closed; never classify an unknown current obligation as old evidence.
func qualifiedCASGap(gap string) bool {
	switch gap {
	case "trusted_original_run_resolution_required", "original_model_identity_absent", "outer_organization_absent", "frozen_admission_absent", "storage_precision_gap", "runtime_execution_history_absent", "canonical_outcome_original_run_absent", "evaluated_owner_original_success_not_retained", "failed_owner_actual_terminal_run_not_retained", "original_model_binding_requires_full_chain_verification", "missing_original_run_identity", "retry_authorized_execution_not_retained", "retry_original_authorization_not_retained", "retry_source_does_not_declare_execution_run", "original_failure_business_anchor_not_retained", "original_outcome_run_absent":
		return true
	default:
		return false
	}
}

// These actual full-catalog/global unknown categories have no original-owner
// resolver. They cannot be converted to a pure historical evidence gap. The
// ordinary cross-database scope flags stay on the persistence report; this is
// not a mutable report-to-authority constructor.
func qualifiedCASMongoGlobalKnown(report MongoResponsibilityCycleReport) bool {
	if !report.Complete || !report.LocalGraphChecked || report.ClassifiedRows != report.Rows || len(report.BlockingReasons) != 0 || report.ClassCounts["unsupported_standard_message_type"] != 0 || report.ClassCounts["maintenance_repair_authorization_unknown"] != 0 {
		return false
	}
	for _, gap := range report.CoverageGaps {
		switch gap {
		case "unknown_catalog_namespace_coverage_required", "unknown_standard_event_type_global_coverage_required", "catalog_repair_plan_execution_coverage_required":
			return false
		}
	}
	return true
}

func qualifiedCASOwnerIDs(f *DecodedSourceEvent, candidate HistoricalCandidate) (assessment, outcome uint64, err error) {
	if qualifiedCASSourceMatches(f, candidate) != nil {
		return 0, 0, ErrCoordinatorCASQualification
	}
	parse := func(value string) (uint64, error) {
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil || n == 0 || strconv.FormatUint(n, 10) != value {
			return 0, ErrCoordinatorCASQualification
		}
		return n, nil
	}
	if f.Source.Database == "mysql" {
		switch f.EventType {
		case "evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed":
		default:
			return 0, 0, ErrSourceEventType
		}
		assessment, err = parse(f.BusinessIDs["assessment_id"])
		if err != nil {
			return 0, 0, err
		}
		if f.EventType == "evaluation.outcome.committed" {
			outcome, err = parse(f.BusinessIDs["outcome_id"])
			if err != nil || candidate.OwnerID != strconv.FormatUint(outcome, 10) {
				return 0, 0, ErrCoordinatorCASQualification
			}
		} else if candidate.OwnerID != strconv.FormatUint(assessment, 10) {
			return 0, 0, ErrCoordinatorCASQualification
		}
		return assessment, outcome, nil
	}
	if f.Source.Database != "mongodb" || f.EventType != "answersheet.submitted" && f.EventType != "interpretation.report.generated" {
		return 0, 0, ErrSourceEventType
	}
	if id, parseErr := parse(candidate.OwnerID); parseErr != nil || id > 1<<63-1 {
		return 0, 0, ErrCoordinatorCASQualification
	}
	// Mongo IDs come from the actual local batch graph. No Assessment ID is
	// fabricated for the independent-questionnaire submission path.
	return 0, 0, nil
}

// Called only after qualifiedCASRows and actual stable binding reconstruction.
// Historical gaps stay explicit. Source row SHA, legacy content SHA and stable
// business binding stay different; this never manufactures an SDK fingerprint.
func qualifiedCASEntry(binding HistoricalCoordinatorBinding, row qualifiedCASRow, verifiedAt time.Time) (evidence.HistoricalReferenceEntryV1, error) {
	if row.sourceObservation != nil {
		return evidence.HistoricalReferenceEntryV1{}, ErrCoordinatorCASQualification
	}
	return qualifiedCASReferenceEntry(binding, row, verifiedAt, "actual-whole-source-joint-fresh-origin-ai14")
}

// The shared identity/gap rules do not decide qualification. Each private
// caller must first validate its own genuine opaque native composition.
func qualifiedCASReferenceEntry(binding HistoricalCoordinatorBinding, row qualifiedCASRow, verifiedAt time.Time, method string) (evidence.HistoricalReferenceEntryV1, error) {
	if method != "actual-whole-source-joint-fresh-origin-ai14" && method != "actual-fresh-owner-component-source-business-related-ai" || (method == "actual-fresh-owner-component-source-business-related-ai") != (row.sourceObservation != nil) || !coordinatorSourceSHA(binding.SourceSHA) || !aiLocalOperationID(binding.OperationID) || verifiedAt.IsZero() || qualifiedCASSourceMatches(row.facts, row.candidate) != nil || !evidence.ValidSHA256(row.bindingSHA) {
		return evidence.HistoricalReferenceEntryV1{}, ErrCoordinatorCASQualification
	}
	// A declared original Run may only be retained verbatim. Sources without
	// an original execution identity cannot acquire a current/latest Run.
	actualRun := row.candidate.ActualOriginalRun
	if actualRun != nil && (row.facts.OriginalRun.RunID == "" || actualRun.RunID != row.facts.OriginalRun.RunID || row.facts.OriginalRun.Attempt != nil && uint(*row.facts.OriginalRun.Attempt) != actualRun.Attempt) {
		return evidence.HistoricalReferenceEntryV1{}, ErrCoordinatorCASQualification
	}
	gaps := append([]string(nil), row.candidate.HistoricalGaps...)
	for _, gap := range gaps {
		if !qualifiedCASGap(gap) {
			return evidence.HistoricalReferenceEntryV1{}, ErrCoordinatorCASQualification
		}
	}
	if row.candidate.ActualOriginalRun == nil && (row.facts.OriginalRun.RunID != "" || len(row.candidate.OriginalRunMissing) != 0) {
		if !slices.Contains(gaps, "missing_original_run_identity") {
			gaps = append(gaps, "missing_original_run_identity")
		}
	}
	slices.Sort(gaps)
	gaps = slices.Compact(gaps)
	class, reason := evidence.RetiredVerified, ""
	if len(gaps) != 0 {
		class, reason = evidence.Unverifiable, strings.Join(gaps, ";")
	}
	// Existing Outcome CAS/audit contract requires this exact reason when
	// its declared original Run no longer exists. Never insert the current Run.
	if row.facts.EventType == "evaluation.outcome.committed" && row.candidate.ActualOriginalRun == nil {
		if row.facts.OriginalRun.RunID == "" || !slices.Contains(gaps, "original_outcome_run_absent") {
			return evidence.HistoricalReferenceEntryV1{}, ErrCoordinatorCASQualification
		}
		class, reason = evidence.Unverifiable, "original_outcome_run_absent"
	}
	entry := evidence.HistoricalReferenceEntryV1{EventID: row.facts.EventID, EventType: row.facts.EventType, Source: row.facts.Source,
		Proof: &evidence.EventEvidenceV1{Version: 1, Class: class, EventID: row.facts.EventID, Digest: row.facts.Source.Digest, BusinessBindingSHA256: row.bindingSHA, Origin: "retirement",
			Verification: evidence.Verification{Method: method, Version: "v1", OperationID: binding.OperationID, Reason: reason, VerifiedAt: verifiedAt.UTC().Truncate(time.Millisecond), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
	if row.candidate.ActualOriginalRun != nil {
		run := *row.candidate.ActualOriginalRun
		entry.Run = &run
	}
	if entry.Validate() != nil {
		return evidence.HistoricalReferenceEntryV1{}, ErrCoordinatorCASQualification
	}
	return entry, nil
}
