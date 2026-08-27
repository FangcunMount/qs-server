package subjectexport

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type exportReaderStub struct {
	queries []ReadQuery
	pages   [][]*artifact.AIExplanationArtifact
}

func (s *exportReaderStub) ListParticipantArtifacts(_ context.Context, query ReadQuery) ([]*artifact.AIExplanationArtifact, error) {
	s.queries = append(s.queries, query)
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func TestExportUsesStableSnapshotAndSubjectBoundCursor(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	subject := Subject{OrgID: 7, TesteeID: meta.FromUint64(11)}
	reader := &exportReaderStub{pages: [][]*artifact.AIExplanationArtifact{
		{exportArtifact(t, 103, 203, subject, now.Add(-time.Hour)), exportArtifact(t, 102, 202, subject, now.Add(-2*time.Hour)), exportArtifact(t, 101, 201, subject, now.Add(-3*time.Hour))},
		{exportArtifact(t, 101, 201, subject, now.Add(-3*time.Hour))},
	}}
	service, err := NewService(reader, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Export(context.Background(), Query{Subject: subject, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first.SchemaVersion != SchemaVersionV1 || len(first.Items) != 2 || first.NextCursor == "" || !first.SnapshotAt.Equal(now) {
		t.Fatalf("unexpected first export page: %#v", first)
	}
	if first.Items[0].Release.ProviderRoute != "participant_scale_v1" || first.Items[0].Content.Summary == "" {
		t.Fatalf("export projection lost release receipt or final content: %#v", first.Items[0])
	}
	second, err := service.Export(context.Background(), Query{Subject: subject, PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.NextCursor != "" || !second.SnapshotAt.Equal(first.SnapshotAt) {
		t.Fatalf("unexpected second export page: %#v", second)
	}
	if len(reader.queries) != 2 || reader.queries[0].Limit != 3 || reader.queries[1].AfterArtifactID != meta.FromUint64(102) {
		t.Fatalf("unexpected export read queries: %#v", reader.queries)
	}
	other := Subject{OrgID: 7, TesteeID: meta.FromUint64(12)}
	if _, err := service.Export(context.Background(), Query{Subject: other, Cursor: first.NextCursor}); err == nil {
		t.Fatal("expected subject-bound cursor rejection")
	}
}

func TestExportRejectsCrossSubjectAndInvalidPageSize(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	subject := Subject{OrgID: 7, TesteeID: meta.FromUint64(11)}
	other := Subject{OrgID: 7, TesteeID: meta.FromUint64(12)}
	reader := &exportReaderStub{pages: [][]*artifact.AIExplanationArtifact{{exportArtifact(t, 101, 201, other, now.Add(-time.Hour))}}}
	service, _ := NewService(reader, func() time.Time { return now })
	if _, err := service.Export(context.Background(), Query{Subject: subject}); err == nil {
		t.Fatal("expected cross-subject artifact rejection")
	}
	if _, err := service.Export(context.Background(), Query{Subject: subject, PageSize: MaxPageSize + 1}); err == nil {
		t.Fatal("expected page-size rejection")
	}
}

func exportArtifact(t *testing.T, artifactID, generationID uint64, subject Subject, generatedAt time.Time) *artifact.AIExplanationArtifact {
	t.Helper()
	fingerprint := aiexplanation.NewFingerprint([]byte("release"))
	validatedAt := generatedAt.Add(-time.Minute)
	value, err := artifact.New(artifact.NewInput{
		ID: meta.FromUint64(artifactID), GenerationID: meta.FromUint64(generationID), RunID: meta.FromUint64(artifactID + 1000),
		Source: artifact.SourceRef{
			ReportID: meta.FromUint64(artifactID + 2000), OutcomeID: meta.FromUint64(artifactID + 3000),
			Association: aiexplanation.Association{OrgID: subject.OrgID, AssessmentID: meta.FromUint64(artifactID + 4000), TesteeID: subject.TesteeID.Uint64()},
			ReportType:  "standard", TemplateVersion: "v1", ContentSchemaVersion: "v1", BuilderIdentity: "builder/v1", ReportGeneratedAt: generatedAt.Add(-time.Hour),
		},
		Audience:      policy.AudienceParticipant,
		Profile:       aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: fingerprint},
		Prompt:        aiexplanation.PromptRef{TemplateID: "cross-dimension", Version: "v1", Fingerprint: fingerprint, GitBlobSHA: "abc123"},
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{Route: "participant_scale_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: fingerprint},
		InputSchema:   aiexplanation.InputSchemaVersionV1, InputFingerprint: fingerprint,
		OutputSchema: aiexplanation.OutputSchemaVersionV1, SafetyPolicy: "safety-v1",
		ProviderReceipt: aiexplanation.ProviderReceipt{InvocationID: "private-invocation", RequestID: "private-request", Provider: "provider-a", Model: "model-a", InputTokens: 1, OutputTokens: 1},
		Validation:      artifact.ValidationReceipt{SchemaValidatorVersion: "schema-v1", ReferenceValidatorVersion: "reference-v1", ProfileValidatorVersion: "profile-v1", SafetyValidatorVersion: "safety-v1", ValidatedAt: validatedAt},
		Content: output.Content{
			SchemaVersion: aiexplanation.OutputSchemaVersionV1, Summary: "综合洞察",
			IntegratedInsights: []output.IntegratedInsight{{
				Kind: output.InsightKindReinforcingPattern, Title: "模式", Content: "内容", WhyItMatters: "意义",
				EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:a"}, {Kind: output.EvidenceKindOverallResult, Ref: "overall_result"}},
			}},
			Suggestions: []output.Suggestion{{
				Origin: output.SuggestionOriginGeneratedLowRisk, Category: "awareness", Title: "建议", Goal: "逐步观察",
				Actions: []string{"记录变化"}, Rationale: "便于回顾", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:a"}},
			}},
			Limitations: []string{"仅供参考"},
		},
		GeneratedAt: generatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
