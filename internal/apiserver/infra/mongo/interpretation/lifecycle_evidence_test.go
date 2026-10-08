package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
	"strings"
	"testing"
	"time"
)

func fixtureProof(t *testing.T, kind, id string) *evidence.EventEvidenceV1 {
	t.Helper()
	p, err := evidence.NewStandard(evidence.StandardReference{EventID: id, Producer: "qs-server", Destination: "test." + kind, EventType: kind, SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: "2026-10-08T00:00:00.123456789Z", Fingerprint: strings.Repeat("a", 64)}, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestLifecycleEvidenceSurvivesBSONRestoreAndCannotAlias(t *testing.T) {
	now := time.Now()
	mapper := NewLifecycleMapper()
	g, err := generation.New(1, generation.Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Begin(2, now); err != nil {
		t.Fatal(err)
	}
	generated := fixtureProof(t, eventcatalog.InterpretationReportGenerated, "generated-id")
	if err := g.SucceedWithEvidence(2, 3, now, generated); err != nil {
		t.Fatal(err)
	}
	generated.Reference.Fingerprint = strings.Repeat("c", 64)
	if g.GeneratedEventEvidence().Reference.Fingerprint != strings.Repeat("a", 64) {
		t.Fatal("generation retained mutable proof")
	}
	po := mapper.GenerationToPO(g)
	raw, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ReportGenerationPO
	if err := bson.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.GenerationToDomain(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	if restored.GeneratedEventID() != "generated-id" || *restored.GeneratedEventEvidence().Reference != *g.GeneratedEventEvidence().Reference {
		t.Fatal("generation reference changed through BSON")
	}
	decoded.GeneratedEventEvidence.Reference.Scope = "org:999"
	if restored.GeneratedEventEvidence().Reference.Scope != "org:1" {
		t.Fatal("restored generation aliases PO proof")
	}
	decoded.GeneratedEventID = "wrong"
	if _, err := mapper.GenerationToDomain(&decoded); err == nil {
		t.Fatal("restored misbound generation proof")
	}

	run, err := interpretationrun.NewPending(2, meta.ID(1), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Start(now, "trace"); err != nil {
		t.Fatal(err)
	}
	if err := run.Fail(now, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	retry := fixtureProof(t, eventcatalog.InterpretationRetryRequested, "interpret-retry:1:1:automatic")
	if err := run.AttachRetryEventEvidence(retry); err != nil {
		t.Fatal(err)
	}
	runPO := mapper.RunToPO(run)
	raw, err = bson.Marshal(runPO)
	if err != nil {
		t.Fatal(err)
	}
	var decodedRun InterpretationRunPO
	if err := bson.Unmarshal(raw, &decodedRun); err != nil {
		t.Fatal(err)
	}
	restoredRun, err := mapper.RunToDomain(&decodedRun)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRun.RetryEventEvidence().EventID != retry.EventID {
		t.Fatal("retry proof dropped from BSON")
	}
	decodedRun.RetryEventEvidence.Reference.Scope = "org:999"
	if restoredRun.RetryEventEvidence().Reference.Scope != "org:1" {
		t.Fatal("restored run aliases PO proof")
	}
	decodedRun.RetryEventID = "other"
	if _, err := mapper.RunToDomain(&decodedRun); err == nil {
		t.Fatal("restored misbound retry proof")
	}
}
func TestNativeTransitionsRejectBackfillProofAndPreserveForceAuthorization(t *testing.T) {
	now := time.Now()
	g, _ := generation.New(1, generation.Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, now)
	_ = g.Begin(2, now)
	proof := fixtureProof(t, eventcatalog.InterpretationReportGenerated, "generated-id")
	proof.Origin = "backfilled_existing"
	if err := g.SucceedWithEvidence(2, 3, now, proof); err == nil || g.Status() != generation.StatusGenerating {
		t.Fatal("native completion used historical preparation")
	}

	run, _ := interpretationrun.NewPending(2, 1, 1)
	_ = run.Start(now, "trace")
	_ = run.Fail(now, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: false})
	proof = fixtureProof(t, eventcatalog.InterpretationRetryRequested, "interpret-retry:1:1:force:operator-id")
	if err := run.AuthorizeOneRetryWithEvidence(retrygovernance.AttemptOriginForce, "operator-id", now, proof); err != nil {
		t.Fatal(err)
	}
	got := run.RetryDecision()
	if got.Disposition != retrygovernance.DispositionAutomatic || got.ActionRequestID != "operator-id" || got.RetryEventID != proof.EventID || !got.NextAttemptAt.Equal(now) {
		t.Fatal("force CAS decision lost native evidence identity")
	}
}
