//go:build integration

package mongoconsistency

import (
	"testing"
	"time"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func auditHistoryReference(id, kind, binding string, class evidence.Class) *evidence.HistoricalReferenceSetV1 {
	digest := evidence.SourceDigest("mongo-selected-source-bson-v1", []byte(id))
	entry := evidence.HistoricalReferenceEntryV1{EventID: id, EventType: kind, Source: evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: "mongodb_objectid", PrimaryKeySHA256: evidence.SourceDigest("objectid", []byte(id)).SHA256, Digest: digest}, Proof: &evidence.EventEvidenceV1{Version: 1, Class: class, EventID: id, Digest: digest, BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "original-business-source", Version: "v1", OperationID: "12345-1", VerifiedAt: time.Date(2026, 10, 8, 3, 4, 5, 678000000, time.UTC), Reason: "pure_historical_message_body_absent", BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
	if kind == "interpretation.report.generated" {
		entry.Run = &evidence.HistoricalRunReferenceV1{RunID: "301", Attempt: 1}
	}
	return &evidence.HistoricalReferenceSetV1{Version: 1, Entries: []evidence.HistoricalReferenceEntryV1{entry}}
}

func TestHistoricalSetAuditCountsConclusionsWithoutClaimingStandardMessages(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	sheet, _ := newSheetFixture(t, 101)
	sheet.DurableAcceptance = nil
	sheet.Admission = &sheetmongo.AdmissionPO{Purpose: "independent_questionnaire", QuestionnaireCode: sheet.QuestionnaireCode, QuestionnaireVersion: sheet.QuestionnaireVersion}
	payload, err := sheetmongo.LegacySubmissionPayload(sheet)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := eventevidencebinding.AnswerSheet(payload)
	if err != nil {
		t.Fatal(err)
	}
	sheet.LegacySubmissionEvidence = auditHistoryReference("legacy-sheet-a", "answersheet.submitted", binding, evidence.RetiredVerified)
	second := auditHistoryReference("legacy-sheet-b", "answersheet.submitted", binding, evidence.Unverifiable).Entries[0]
	sheet.LegacySubmissionEvidence, err = sheet.LegacySubmissionEvidence.Append(second)
	if err != nil {
		t.Fatal(err)
	}
	mustInsertMany(t, ctx, db.Collection("answersheets"), []any{sheet})
	at := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	g := interpretmongo.ReportGenerationPO{BaseDocument: base.BaseDocument{DomainID: 201}, OutcomeID: 42, ReportType: "standard", TemplateVersion: "v1", Status: "generated", LatestRunID: 301, ReportID: 401, Version: 1}
	a := interpretmongo.InterpretReportPO{BaseDocument: base.BaseDocument{DomainID: 401}, GenerationID: 201, OutcomeID: 42, InterpretationRunID: 301, OrgID: 1, AssessmentID: 7, TesteeID: 8, ReportType: "standard", TemplateVersion: "v1", GeneratedAt: at, Model: &interpretmongo.ModelIdentityPO{Kind: "scale", Code: "m1", Version: "v1"}}
	r := interpretmongo.InterpretationRunPO{BaseDocument: base.BaseDocument{DomainID: 301}, GenerationID: 201, Attempt: 1, Status: "succeeded"}
	fact, err := (auditFacts{}).FindByID(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := interpretmongo.HistoricalGeneratedPayload(g, a, r, fact)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = eventevidencebinding.Generated(generated)
	if err != nil {
		t.Fatal(err)
	}
	g.HistoricalGeneratedEvidence = auditHistoryReference("legacy-generated-a", "interpretation.report.generated", binding, evidence.RetiredVerified)
	g.HistoricalGeneratedEvidence, err = g.HistoricalGeneratedEvidence.Append(auditHistoryReference("legacy-generated-b", "interpretation.report.generated", binding, evidence.RetiredVerified).Entries[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"report_generations", g}, {"interpret_report_artifacts", a}, {"interpretation_runs", r}} {
		mustInsertMany(t, ctx, db.Collection(item.name), []any{item.value})
	}
	if _, err := interpretmongo.NewReportRepository(db); err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(db, nil).WithOutcomeFacts(auditFacts{})
	for _, phase := range []appaudit.Phase{appaudit.PhaseAnswerSheetOutbox, appaudit.PhaseGeneratedTerminal} {
		result, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: phase, UpperBound: 1000, Limit: 10, MaxTime: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != 0 || result.EvidenceClasses["standard_reference"] != 0 {
			t.Fatalf("history claimed standard verification: %#v", result)
		}
		if phase == appaudit.PhaseAnswerSheetOutbox && (result.EvidenceClasses["retired_verified"] != 1 || result.EvidenceClasses["unverifiable"] != 1) {
			t.Fatal(result)
		}
		if phase == appaudit.PhaseGeneratedTerminal && result.EvidenceClasses["retired_verified"] != 2 {
			t.Fatal(result)
		}
	}
	// PO decoding must not silently ignore a forbidden body in the new slot.
	if _, err := db.Collection("answersheets").UpdateOne(ctx, bson.M{"domain_id": uint64(101)}, bson.M{"$set": bson.M{"legacy_submission_evidence.body": "forbidden legacy body"}}); err != nil {
		t.Fatal(err)
	}
	sheetResult, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: appaudit.PhaseAnswerSheetOutbox, UpperBound: 1000, Limit: 10, MaxTime: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(sheetResult.Findings) == 0 || len(sheetResult.EvidenceClasses) != 0 {
		t.Fatal("ignored unknown BSON body was accepted as a historical conclusion")
	}
	// An original reference is never repaired by the latest winner or current state.
	if _, err := db.Collection("interpretation_runs").UpdateOne(ctx, bson.M{"domain_id": uint64(301)}, bson.M{"$set": bson.M{"status": "running"}}); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: appaudit.PhaseGeneratedTerminal, UpperBound: 1000, Limit: 10, MaxTime: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) == 0 || result.EvidenceClasses["retired_verified"] != 0 {
		t.Fatal("open original graph passed history audit")
	}
	if names, err := db.ListCollectionNames(ctx, bson.M{}); err != nil {
		t.Fatal(err)
	} else {
		for _, name := range names {
			if name == "rm_outbox" || name == "domain_event_outbox" {
				t.Fatalf("historical audit created %s", name)
			}
		}
	}
}
