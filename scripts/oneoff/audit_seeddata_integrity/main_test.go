package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	evaluationevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func validAuditConfig(t *testing.T) config {
	t.Helper()
	return config{
		MySQLDSN: "user:password@tcp(localhost:3306)/qs?parseTime=true",
		MongoURI: "mongodb://localhost:27017", MongoDB: "qs", OrgID: 1,
		BatchID: "hist-20250101-20260727-v2", From: "2025-01-01", To: "2026-07-27",
		Timezone: "Asia/Shanghai", OutputPath: filepath.Join(t.TempDir(), "audit.json"),
		PageSize: 500, MaxFindings: 1000, MaxCandidates: 10000, Timeout: time.Hour,
	}
}

func TestAuditConfigIsReadOnlyByDefault(t *testing.T) {
	cfg := validAuditConfig(t)
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate audit config: %v", err)
	}
	if cfg.Apply {
		t.Fatal("audit must not apply by default")
	}
}

func TestApplyRequiresReceiptBackupAndExactConfirmation(t *testing.T) {
	cfg := validAuditConfig(t)
	cfg.Apply = true
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "audit-report") {
		t.Fatalf("missing receipt error = %v", err)
	}
	cfg.AuditReport = "/secure/audit.json"
	cfg.BackupSuffix = "hist_20260802"
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), applyConfirmation) {
		t.Fatalf("missing confirmation error = %v", err)
	}
	cfg.Confirm = applyConfirmation
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "services-stopped") {
		t.Fatalf("missing services-stopped confirmation error = %v", err)
	}
	cfg.ConfirmServicesStopped = true
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate apply config: %v", err)
	}
}

func TestApplyRejectsAuditReportAsOutput(t *testing.T) {
	cfg := validAuditConfig(t)
	cfg.Apply = true
	cfg.AuditReport = cfg.OutputPath
	cfg.BackupSuffix = "hist_20260802"
	cfg.Confirm = applyConfirmation
	cfg.ConfirmServicesStopped = true
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("same audit/output path error = %v", err)
	}
}

func TestBusinessWindowIsInclusiveExclusiveShanghaiTime(t *testing.T) {
	cfg := validAuditConfig(t)
	from, to, err := cfg.businessWindow()
	if err != nil {
		t.Fatal(err)
	}
	if got := from.Format(time.RFC3339); got != "2025-01-01T00:00:00+08:00" {
		t.Fatalf("from = %s", got)
	}
	if got := to.Format(time.RFC3339); got != "2026-07-28T00:00:00+08:00" {
		t.Fatalf("to exclusive = %s", got)
	}
}

func TestMissingMySQLParentProducesExactDeletionCandidate(t *testing.T) {
	stageAt := time.Date(2025, 1, 1, 19, 18, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	artifactID := primitive.NewObjectID()
	evidence := reportEvidence{
		Stage: reportStage{ID: 99, ScenarioID: "2025-01-01/92/submit_answer/entry", BusinessAt: stageAt, ResourceID: 501},
		Artifact: &artifactRow{
			ObjectID: artifactID, DomainID: 501, GenerationID: 502, OutcomeID: 503,
			InterpretationRunID: 504, OrgID: 1, AssessmentID: 505, TesteeID: 506, GeneratedAt: stageAt.UTC(),
		},
	}
	findings, candidate := classifyReportEvidence(validAuditConfig(t), evidence)
	if candidate == nil {
		t.Fatalf("expected candidate; findings=%#v", findings)
	}
	if candidate.ReportID != "501" || candidate.AssessmentID != "505" || candidate.OutcomeID != "503" || candidate.ArtifactObject != artifactID.Hex() {
		t.Fatalf("candidate = %#v", candidate)
	}
	if len(findings) != 1 || findings[0].Code != "orphan_report_mysql_parent_missing" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestIdentityMismatchIsReportedButNotAutomaticallyDeleted(t *testing.T) {
	stageAt := time.Date(2025, 1, 1, 19, 18, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	artifact := &artifactRow{ObjectID: primitive.NewObjectID(), DomainID: 501, GenerationID: 502, OutcomeID: 503, InterpretationRunID: 504, OrgID: 1, AssessmentID: 505, TesteeID: 506, GeneratedAt: stageAt.UTC()}
	evidence := reportEvidence{
		Stage: reportStage{ID: 99, ScenarioID: "scenario", BusinessAt: stageAt, ResourceID: 501}, Artifact: artifact,
		Assessment: &assessmentRow{ID: 505, OrgID: 1, TesteeID: 999},
		Outcome:    &outcomeRow{ID: 503, OrgID: 1, AssessmentID: 505, TesteeID: 506},
		Generation: &generationRow{DomainID: 502, OutcomeID: 503, ReportID: 501, Status: "succeeded"},
		Run:        &runRow{DomainID: 504, Generation: 502, Status: "succeeded"},
		Catalog:    &catalogRow{AssessmentID: 505, OutcomeID: 503, GenerationID: 502, SourceKind: "artifact", SourceID: 501},
	}
	findings, candidate := classifyReportEvidence(validAuditConfig(t), evidence)
	if candidate != nil {
		t.Fatalf("identity mismatch must not be auto-deleted: %#v", candidate)
	}
	if len(findings) != 1 || findings[0].Code != "report_assessment_mismatch" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestMissingParentWithConflictingSurvivingParentBlocksDeletion(t *testing.T) {
	stageAt := time.Date(2025, 1, 1, 19, 18, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	artifact := &artifactRow{ObjectID: primitive.NewObjectID(), DomainID: 501, GenerationID: 502, OutcomeID: 503, InterpretationRunID: 504, OrgID: 1, AssessmentID: 505, TesteeID: 506, GeneratedAt: stageAt.UTC()}
	evidence := reportEvidence{
		Stage: reportStage{ID: 99, ScenarioID: "scenario", BusinessAt: stageAt, ResourceID: 501}, Artifact: artifact,
		// Assessment is missing, but the surviving Outcome belongs to a different
		// Assessment. Following that ID would widen cleanup beyond proven scope.
		Outcome: &outcomeRow{ID: 503, OrgID: 1, AssessmentID: 999, TesteeID: 506},
	}
	findings, candidate := classifyReportEvidence(validAuditConfig(t), evidence)
	if candidate != nil {
		t.Fatalf("conflicting surviving parent must block deletion: %#v", candidate)
	}
	codes := make(map[string]bool)
	for _, item := range findings {
		codes[item.Code] = true
	}
	if !codes["report_outcome_mismatch"] || !codes["orphan_auto_delete_blocked"] {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestOnlyOneMissingMySQLParentIsNotAutomaticallyDeleted(t *testing.T) {
	stageAt := time.Date(2025, 1, 1, 19, 18, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	evidence := reportEvidence{
		Stage: reportStage{ID: 99, ScenarioID: "scenario", BusinessAt: stageAt, ResourceID: 501},
		Artifact: &artifactRow{
			ObjectID: primitive.NewObjectID(), DomainID: 501, GenerationID: 502, OutcomeID: 503,
			InterpretationRunID: 504, OrgID: 1, AssessmentID: 505, TesteeID: 506, GeneratedAt: stageAt,
		},
		Outcome: &outcomeRow{ID: 503, OrgID: 1, AssessmentID: 505, TesteeID: 506},
	}
	findings, candidate := classifyReportEvidence(validAuditConfig(t), evidence)
	if candidate != nil {
		t.Fatalf("partial parent loss must not be auto-deleted: %#v", candidate)
	}
	codes := make(map[string]bool)
	for _, item := range findings {
		codes[item.Code] = true
	}
	if !codes["orphan_report_mysql_parent_missing"] || !codes["orphan_auto_delete_blocked"] {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestStagePayloadIdentityMismatchBlocksOrphanDeletion(t *testing.T) {
	stageAt := time.Date(2025, 1, 1, 19, 18, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	payload, _ := json.Marshal(map[string]string{"generation_id": "999", "run_id": "504"})
	evidence := reportEvidence{
		Stage:    reportStage{ID: 99, ScenarioID: "scenario", BusinessAt: stageAt, ResourceID: 501, PayloadJSON: payload},
		Artifact: &artifactRow{ObjectID: primitive.NewObjectID(), DomainID: 501, GenerationID: 502, OutcomeID: 503, InterpretationRunID: 504, OrgID: 1, AssessmentID: 505, TesteeID: 506, GeneratedAt: stageAt.UTC()},
	}
	findings, candidate := classifyReportEvidence(validAuditConfig(t), evidence)
	if candidate != nil {
		t.Fatalf("payload mismatch must block deletion: %#v", candidate)
	}
	codes := make(map[string]bool)
	for _, item := range findings {
		codes[item.Code] = true
	}
	if !codes["report_generation_payload_mismatch"] || !codes["orphan_auto_delete_blocked"] {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestBusinessTimeMismatchBlocksOrphanDeletion(t *testing.T) {
	stageAt := time.Date(2025, 1, 1, 19, 18, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	evidence := reportEvidence{
		Stage: reportStage{ID: 99, ScenarioID: "scenario", BusinessAt: stageAt, ResourceID: 501},
		Artifact: &artifactRow{
			ObjectID: primitive.NewObjectID(), DomainID: 501, GenerationID: 502, OutcomeID: 503,
			InterpretationRunID: 504, OrgID: 1, AssessmentID: 505, TesteeID: 506,
			GeneratedAt: stageAt.Add(time.Second),
		},
	}
	findings, candidate := classifyReportEvidence(validAuditConfig(t), evidence)
	if candidate != nil {
		t.Fatalf("business time mismatch must not be auto-deleted: %#v", candidate)
	}
	codes := make(map[string]bool)
	for _, item := range findings {
		codes[item.Code] = true
	}
	if !codes["report_business_time_mismatch"] || !codes["orphan_auto_delete_blocked"] {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestCleanupFiltersStayOnExactReportChainIDs(t *testing.T) {
	filters := mongoCleanupFilters(cleanupIDs{
		Reports: []uint64{11}, Assessments: []uint64{12}, Outcomes: []uint64{13}, Generations: []uint64{14}, Runs: []uint64{15},
	})
	byName := make(map[string]bson.M)
	for _, filter := range filters {
		byName[filter.Name] = filter.Filter
	}
	wantArtifact := bson.M{"domain_id": bson.M{"$in": []uint64{11}}}
	if !reflect.DeepEqual(byName["interpret_report_artifacts"], wantArtifact) {
		t.Fatalf("artifact filter = %#v", byName["interpret_report_artifacts"])
	}
	wantOutbox := bson.M{"aggregate_type": "ReportGeneration", "aggregate_id": bson.M{"$in": []string{"14"}}}
	if !reflect.DeepEqual(byName["domain_event_outbox"], wantOutbox) {
		t.Fatalf("outbox filter = %#v", byName["domain_event_outbox"])
	}
	encoded, _ := json.Marshal(filters)
	if strings.Contains(string(encoded), "testee_id") {
		t.Fatalf("cleanup must not widen by testee_id: %s", encoded)
	}
}

func TestMySQLOutboxCleanupIdentityMatchesOutcomeCommittedEvent(t *testing.T) {
	event := evaluationevent.NewOutcomeCommittedEvent(1, 505, 506, "503", "505:1", time.Unix(100, 0))
	if event.EventType() != "evaluation.outcome.committed" || event.AggregateType() != "Evaluation" || event.AggregateID() != "505" {
		t.Fatalf("event identity = type:%q aggregate:%q/%q", event.EventType(), event.AggregateType(), event.AggregateID())
	}

	payload, err := eventcodec.EncodeDomainEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			OutcomeID string `json:"outcome_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.OutcomeID != "503" {
		t.Fatalf("encoded data.outcome_id = %q", envelope.Data.OutcomeID)
	}
}

func TestStatisticsFactSourcesUseCanonicalIndexedIdentity(t *testing.T) {
	candidates := []orphanCandidate{
		{AssessmentID: "505", OutcomeID: "503", ReportID: "501", GenerationID: "502"},
		{AssessmentID: "505", OutcomeID: "503", ReportID: "501", GenerationID: "502"},
	}
	got, err := statisticsFactSources(candidates)
	if err != nil {
		t.Fatal(err)
	}
	want := []statisticsFactSource{
		{FactType: "assessment_created", SourceType: "assessment", SourceRef: "505"},
		{FactType: "outcome_committed", SourceType: "evaluation_outcome", SourceRef: "503"},
		{FactType: "report_generated", SourceType: "interpret_report", SourceRef: "501"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statistics fact sources = %#v, want %#v", got, want)
	}
}

func TestStatisticsFactJoinUsesSourceIndexColumns(t *testing.T) {
	for _, required := range []string{
		"FORCE INDEX (idx_statistics_assessment_fact_source)",
		"f.source_type=x.source_type",
		"f.source_ref=x.source_ref",
		"f.fact_type=x.fact_type",
	} {
		if !strings.Contains(statisticsFactSourceJoin, required) {
			t.Fatalf("statistics fact join missing %q: %s", required, statisticsFactSourceJoin)
		}
	}
	for _, forbidden := range []string{"f.assessment_id", "f.outcome_id", "f.report_id", " OR "} {
		if strings.Contains(statisticsFactSourceJoin, forbidden) {
			t.Fatalf("statistics fact join contains unindexed predicate %q: %s", forbidden, statisticsFactSourceJoin)
		}
	}
}

func TestDeletionPlanHashDetectsCandidateTampering(t *testing.T) {
	report := auditReport{Version: 1, OrgID: 1, BatchID: "batch", From: "2025-01-01", To: "2025-01-02", DeletionCandidates: []orphanCandidate{{StageID: 1, ReportID: "2", AssessmentID: "3", OutcomeID: "4", GenerationID: "5"}}}
	first, err := deletionPlanHash(report)
	if err != nil {
		t.Fatal(err)
	}
	report.DeletionCandidates[0].ReportID = "9"
	second, err := deletionPlanHash(report)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("plan hash must change when candidate identity changes")
	}
}

func TestWriteResultUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "audit.json")
	if err := writeResult(io.Discard, path, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
