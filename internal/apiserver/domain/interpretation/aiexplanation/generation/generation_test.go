package generation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	aiexplanationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestGenerationLifecycleKeepsSuccessfulArtifactSeparate(t *testing.T) {
	createdAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	generation, err := New(validNewInput(t, createdAt))
	if err != nil {
		t.Fatal(err)
	}
	if generation.Status() != StatusPending || generation.Version() != 1 {
		t.Fatalf("initial state = %s v%d", generation.Status(), generation.Version())
	}

	runID := meta.FromUint64(301)
	if err := generation.Begin(runID, createdAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	artifactID := meta.FromUint64(401)
	if err := generation.Succeed(runID, artifactID, createdAt.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if generation.Status() != StatusGenerated || generation.ArtifactID() != artifactID || generation.Version() != 3 {
		t.Fatalf("terminal state = %s artifact=%s v%d", generation.Status(), generation.ArtifactID(), generation.Version())
	}
	if err := generation.Begin(meta.FromUint64(302), createdAt.Add(3*time.Second)); err == nil {
		t.Fatal("generated explanation was started again")
	}
}

func TestFailedGenerationCanStartNewRunWithoutChangingFrozenInput(t *testing.T) {
	createdAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	generation, err := New(validNewInput(t, createdAt))
	if err != nil {
		t.Fatal(err)
	}
	run1 := meta.FromUint64(301)
	if err := generation.Begin(run1, createdAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := generation.Fail(run1, createdAt.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	wantFingerprint := generation.Input().Fingerprint()
	run2 := meta.FromUint64(302)
	if err := generation.Begin(run2, createdAt.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if generation.LatestRunID() != run2 || generation.Input().Fingerprint() != wantFingerprint {
		t.Fatal("retry changed the frozen generation request")
	}
}

func TestGenerationRejectsInputFingerprintOutsideKey(t *testing.T) {
	input := validNewInput(t, time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC))
	input.Key.InputFingerprint = aiexplanation.NewFingerprint([]byte("different"))
	if _, err := New(input); err == nil {
		t.Fatal("expected input fingerprint mismatch")
	}
}

func validNewInput(t *testing.T, createdAt time.Time) NewInput {
	t.Helper()
	snapshot, err := aiexplanationinput.NewSnapshot([]byte(`{"schema_version":"ai-explanation-input/v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	profileFingerprint := aiexplanation.NewFingerprint([]byte("profile"))
	executionFingerprint := aiexplanation.NewFingerprint([]byte("route"))
	return NewInput{
		ID: meta.FromUint64(101),
		Key: Key{
			SourceReportID: meta.FromUint64(201), Audience: policy.AudienceParticipant,
			Profile:          aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: profileFingerprint},
			InputFingerprint: snapshot.Fingerprint(), ExecutionSpecFingerprint: executionFingerprint,
		},
		Association: aiexplanation.Association{OrgID: 9, AssessmentID: meta.FromUint64(501), TesteeID: 601},
		RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: "601"},
		Input:       snapshot,
		Prompt: aiexplanation.PromptRef{
			TemplateID: "cross-dimension-participant-scale", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123",
		},
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{
			Route: "participant_scale_v1", RouteRevision: "v1", ResolvedProvider: "provider-a",
			ResolvedModel: "model-a", Fingerprint: executionFingerprint,
		},
		CreatedAt: createdAt,
	}
}
