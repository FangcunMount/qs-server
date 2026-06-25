package evaluationinput

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleToSnapshotMapsFactorScoringAndInterpretRules(t *testing.T) {
	maxScore := 100.0
	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("total"),
		"总分",
		scaledefinition.WithIsTotalScore(true),
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1"), meta.NewCode("Q2")}),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategyCnt),
		scaledefinition.WithScoringParams(scaledefinition.NewScoringParams().WithCntOptionContents([]string{"经常"})),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(scaledefinition.NewScoreRange(0, 60), scaledefinition.RiskLevelLow, "低风险", "保持"),
			scaledefinition.NewInterpretationRule(scaledefinition.NewScoreRange(60, 100), scaledefinition.RiskLevelHigh, "高风险", "干预"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor returned error: %v", err)
	}
	medicalScale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SDS"),
		"SDS",
		scaledefinition.WithID(meta.FromUint64(101)),
		scaledefinition.WithQuestionnaire(meta.NewCode("Q-SDS"), "1.0.0"),
		scaledefinition.WithScaleVersion("2.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
		scaledefinition.WithFactors([]*scaledefinition.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale returned error: %v", err)
	}

	snapshot := scaleToSnapshot(medicalScale)
	if snapshot == nil {
		t.Fatal("snapshot is nil")
		return
	}
	if snapshot.ID != 101 ||
		snapshot.Code != "SDS" ||
		snapshot.ScaleVersion != "2.0.0" ||
		snapshot.Title != "SDS" ||
		snapshot.QuestionnaireCode != "Q-SDS" ||
		snapshot.QuestionnaireVersion != "1.0.0" ||
		snapshot.Status != "published" {
		t.Fatalf("unexpected scale snapshot: %#v", snapshot)
	}
	if len(snapshot.Factors) != 1 {
		t.Fatalf("factor count = %d, want 1", len(snapshot.Factors))
	}
	got := snapshot.Factors[0]
	if got.Code != "total" || got.Title != "总分" || !got.IsTotalScore {
		t.Fatalf("unexpected factor snapshot: %#v", got)
	}
	if got.ScoringStrategy != "cnt" || len(got.ScoringParams.CntOptionContents) != 1 || got.ScoringParams.CntOptionContents[0] != "经常" {
		t.Fatalf("unexpected scoring params: %#v", got.ScoringParams)
	}
	if len(got.QuestionCodes) != 2 || got.QuestionCodes[0] != "Q1" || got.QuestionCodes[1] != "Q2" {
		t.Fatalf("unexpected question codes: %#v", got.QuestionCodes)
	}
	if got.MaxScore == nil || *got.MaxScore != maxScore {
		t.Fatalf("max score = %v, want %v", got.MaxScore, maxScore)
	}
	if len(got.InterpretRules) != 2 ||
		got.InterpretRules[0].Min != 0 ||
		got.InterpretRules[0].Max != 60 ||
		got.InterpretRules[0].RiskLevel != "low" ||
		got.InterpretRules[0].Conclusion != "低风险" ||
		got.InterpretRules[0].Suggestion != "保持" ||
		got.InterpretRules[1].RiskLevel != "high" ||
		got.InterpretRules[1].Conclusion != "高风险" ||
		got.InterpretRules[1].Suggestion != "干预" {
		t.Fatalf("unexpected interpret rules: %#v", got.InterpretRules)
	}
}

func TestAnswerSheetToSnapshotPreservesRawValuesAndScores(t *testing.T) {
	answer, err := answersheet.NewAnswer(
		meta.NewCode("Q1"),
		questionnaire.TypeRadio,
		answersheet.NewOptionValue("A"),
		3.5,
	)
	if err != nil {
		t.Fatalf("NewAnswer returned error: %v", err)
	}
	questionnaireRef, err := answersheet.NewQuestionnaireRef("Q-SDS", "1.0.0", "SDS Questionnaire")
	if err != nil {
		t.Fatalf("NewQuestionnaireRef returned error: %v", err)
	}
	sheet := answersheet.Reconstruct(
		meta.FromUint64(9001),
		questionnaireRef,
		actor.NewFillerRef(101, actor.FillerTypeSelf),
		[]answersheet.Answer{answer},
		time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
		3.5,
	)

	snapshot := answerSheetToSnapshot(sheet)
	if snapshot.ID != 9001 || snapshot.QuestionnaireCode != "Q-SDS" || snapshot.QuestionnaireVersion != "1.0.0" {
		t.Fatalf("unexpected answer sheet snapshot: %#v", snapshot)
	}
	if len(snapshot.Answers) != 1 {
		t.Fatalf("answer count = %d, want 1", len(snapshot.Answers))
	}
	got := snapshot.Answers[0]
	if got.QuestionCode != "Q1" || got.Score != 3.5 || got.Value != "A" {
		t.Fatalf("unexpected answer snapshot: %#v", got)
	}
}

func TestQuestionnaireToSnapshotPreservesOptionScores(t *testing.T) {
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q1")),
		questionnaire.WithStem("How often?"),
		questionnaire.WithQuestionType(questionnaire.TypeRadio),
		questionnaire.WithOption("A", "Never", 0),
		questionnaire.WithOption("B", "Often", 3),
	)
	if err != nil {
		t.Fatalf("NewQuestion returned error: %v", err)
	}
	qnr, err := questionnaire.NewQuestionnaire(
		meta.NewCode("Q-SDS"),
		"SDS Questionnaire",
		questionnaire.WithVersion(questionnaire.Version("1.0.0")),
		questionnaire.WithQuestions([]questionnaire.Question{question}),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire returned error: %v", err)
	}

	snapshot := questionnaireToSnapshot(qnr)
	if snapshot.Code != "Q-SDS" || snapshot.Version != "1.0.0" || snapshot.Title != "SDS Questionnaire" {
		t.Fatalf("unexpected questionnaire snapshot: %#v", snapshot)
	}
	if len(snapshot.Questions) != 1 || len(snapshot.Questions[0].Options) != 2 {
		t.Fatalf("unexpected question/options: %#v", snapshot.Questions)
	}
	if got := snapshot.Questions[0].Options[1]; got.Code != "B" || got.Content != "Often" || got.Score != 3 {
		t.Fatalf("unexpected option snapshot: %#v", got)
	}
}

func TestResolverComposesSnapshotReadersUsingAnswerSheetExactVersion(t *testing.T) {
	scaleSnapshot := &scalesnapshot.ScaleSnapshot{
		Code:                 "SDS",
		Title:                "SDS",
		QuestionnaireCode:    "Q-SDS",
		QuestionnaireVersion: "2.0.0",
	}
	answerSnapshot := &port.AnswerSheetSnapshot{
		ID:                   2001,
		QuestionnaireCode:    "Q-SDS",
		QuestionnaireVersion: "2.0.0",
	}
	questionnaireSnapshot := &port.QuestionnaireSnapshot{Code: "Q-SDS", Version: "2.0.0"}
	qReader := &questionnaireReaderStub{snapshot: questionnaireSnapshot}
	scaleCatalog := &scaleCatalogStub{snapshot: scaleSnapshot}
	resolver, err := NewResolver(
		scaleCatalog,
		NewScaleModelInputProvider(scaleCatalog, answerSheetReaderStub{snapshot: answerSnapshot}, qReader),
	)
	if err != nil {
		t.Fatalf("NewResolver returned error: %v", err)
	}

	snapshot, err := resolver.Resolve(context.Background(), port.InputRef{
		ModelRef:             port.ModelRef{Kind: port.EvaluationModelKindScale, Code: "SDS", Version: "2.0.0", Title: "SDS"},
		AnswerSheetID:        2001,
		QuestionnaireCode:    "ignored",
		QuestionnaireVersion: "ignored",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	scalePayload, ok := port.ScalePayload(snapshot)
	if !ok || scalePayload != scaleSnapshot || snapshot.AnswerSheet != answerSnapshot || snapshot.Questionnaire != questionnaireSnapshot {
		t.Fatalf("unexpected composed snapshot: %#v", snapshot)
	}
	if snapshot.Model == nil {
		t.Fatal("expected model snapshot")
	}
	if snapshot.Model.Kind != port.EvaluationModelKindScale ||
		snapshot.Model.Code != "SDS" ||
		snapshot.Model.Version != "2.0.0" ||
		snapshot.Model.Title != "SDS" {
		t.Fatalf("unexpected model snapshot: %#v", snapshot.Model)
	}
	payload, ok := snapshot.Model.Payload.(port.ScaleModelPayload)
	if !ok || payload.Scale != scaleSnapshot {
		t.Fatalf("unexpected scale model payload: %#v", snapshot.Model.Payload)
	}
	payload, ok = snapshot.ModelPayload.(port.ScaleModelPayload)
	if !ok || payload.Scale != scaleSnapshot {
		t.Fatalf("unexpected input model payload: %#v", snapshot.ModelPayload)
	}
	if scaleCatalog.ref.Code != "SDS" || scaleCatalog.ref.Version != "2.0.0" {
		t.Fatalf("scale catalog ref = %#v, want SDS/2.0.0", scaleCatalog.ref)
	}
	if qReader.code != "Q-SDS" || qReader.version != "2.0.0" {
		t.Fatalf("questionnaire reader called with %s/%s, want answer sheet exact version", qReader.code, qReader.version)
	}
}

func TestQuestionnaireSnapshotReaderExactVersionMissCarriesFailureReason(t *testing.T) {
	reader := NewRepositoryQuestionnaireSnapshotReader(questionnaireRepoStub{})

	_, err := reader.GetQuestionnaire(context.Background(), "Q-SDS", "9.9.9")
	if err == nil {
		t.Fatal("expected exact version miss to fail")
	}
	if got := FailureReason(err); got == "" || got == err.Error() {
		t.Fatalf("expected domain failure reason to wrap api error, got %q", got)
	}
	var kindCarrier port.FailureKindCarrier
	if !stderrors.As(err, &kindCarrier) {
		t.Fatalf("expected failure kind carrier, got %T", err)
	}
	if got := kindCarrier.FailureKind(); got != port.FailureKindQuestionnaireVersionMismatch {
		t.Fatalf("failure kind = %s, want %s", got, port.FailureKindQuestionnaireVersionMismatch)
	}
}

func TestModelInputProviderRegistryRejectsDuplicateAndUnknownKind(t *testing.T) {
	if _, err := NewModelInputProviderRegistry(fakeInputProvider{kind: port.EvaluationModelKindScale}, fakeInputProvider{kind: port.EvaluationModelKindScale}); err == nil {
		t.Fatal("expected duplicate provider kind error")
	}
	registry, err := NewModelInputProviderRegistry(fakeInputProvider{kind: port.EvaluationModelKindScale})
	if err != nil {
		t.Fatalf("NewModelInputProviderRegistry returned error: %v", err)
	}
	if _, err := registry.Resolve(port.EvaluationModelKindMBTI); err == nil {
		t.Fatal("expected unknown provider kind error")
	}
}

func TestNewResolverReturnsProviderRegistryError(t *testing.T) {
	if _, err := NewResolver(&scaleCatalogStub{}, fakeInputProvider{kind: port.EvaluationModelKindScale}, fakeInputProvider{kind: port.EvaluationModelKindScale}); err == nil {
		t.Fatal("NewResolver error = nil, want duplicate provider kind error")
	}
	if _, err := NewResolver(&scaleCatalogStub{}, nil); err == nil {
		t.Fatal("NewResolver error = nil, want nil provider error")
	}
}

func TestRepositoryResolverUnsupportedRuleSetKindCarriesFailureKind(t *testing.T) {
	resolver, err := NewResolver(&scaleCatalogStub{})
	if err != nil {
		t.Fatalf("NewResolver returned error: %v", err)
	}
	_, err = resolver.Resolve(context.Background(), port.InputRef{
		ModelRef: port.ModelRef{Kind: port.EvaluationModelKindMBTI, Code: "MBTI-16P"},
	})
	if err == nil {
		t.Fatal("expected unsupported model kind error")
	}
	var kindCarrier port.FailureKindCarrier
	if !stderrors.As(err, &kindCarrier) {
		t.Fatalf("expected failure kind carrier, got %T", err)
	}
	if got := kindCarrier.FailureKind(); got != port.FailureKindUnsupportedModel {
		t.Fatalf("failure kind = %s, want %s", got, port.FailureKindUnsupportedModel)
	}
}

type fakeInputProvider struct {
	kind port.EvaluationModelKind
}

func (p fakeInputProvider) Kind() port.EvaluationModelKind {
	return p.kind
}

func (p fakeInputProvider) ResolveInput(context.Context, port.InputRef) (*port.InputSnapshot, error) {
	return &port.InputSnapshot{}, nil
}

type scaleCatalogStub struct {
	snapshot *scalesnapshot.ScaleSnapshot
	err      error
	ref      port.ModelRef
}

func (s *scaleCatalogStub) GetScale(context.Context, string) (*scalesnapshot.ScaleSnapshot, error) {
	return s.snapshot, s.err
}

func (s *scaleCatalogStub) GetScaleByRef(_ context.Context, ref port.ModelRef) (*scalesnapshot.ScaleSnapshot, error) {
	s.ref = ref
	return s.snapshot, s.err
}

type answerSheetReaderStub struct {
	snapshot *port.AnswerSheetSnapshot
	err      error
}

func (s answerSheetReaderStub) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return s.snapshot, s.err
}

type questionnaireReaderStub struct {
	snapshot *port.QuestionnaireSnapshot
	err      error
	code     string
	version  string
}

func (s *questionnaireReaderStub) GetQuestionnaire(_ context.Context, code, version string) (*port.QuestionnaireSnapshot, error) {
	s.code = code
	s.version = version
	return s.snapshot, s.err
}

type questionnaireRepoStub struct{}

func (questionnaireRepoStub) Create(context.Context, *questionnaire.Questionnaire) error { return nil }
func (questionnaireRepoStub) FindByCode(context.Context, string) (*questionnaire.Questionnaire, error) {
	return nil, stderrors.New("not implemented")
}
func (questionnaireRepoStub) FindPublishedByCode(context.Context, string) (*questionnaire.Questionnaire, error) {
	return nil, stderrors.New("not implemented")
}
func (questionnaireRepoStub) FindLatestPublishedByCode(context.Context, string) (*questionnaire.Questionnaire, error) {
	return nil, stderrors.New("not implemented")
}
func (questionnaireRepoStub) FindByCodeVersion(context.Context, string, string) (*questionnaire.Questionnaire, error) {
	return nil, nil
}
func (questionnaireRepoStub) FindBaseByCode(context.Context, string) (*questionnaire.Questionnaire, error) {
	return nil, stderrors.New("not implemented")
}
func (questionnaireRepoStub) FindBasePublishedByCode(context.Context, string) (*questionnaire.Questionnaire, error) {
	return nil, stderrors.New("not implemented")
}
func (questionnaireRepoStub) FindBaseByCodeVersion(context.Context, string, string) (*questionnaire.Questionnaire, error) {
	return nil, stderrors.New("not implemented")
}
func (questionnaireRepoStub) LoadQuestions(context.Context, *questionnaire.Questionnaire) error {
	return nil
}
func (questionnaireRepoStub) Update(context.Context, *questionnaire.Questionnaire) error { return nil }
func (questionnaireRepoStub) CreatePublishedSnapshot(context.Context, *questionnaire.Questionnaire, bool) error {
	return nil
}
func (questionnaireRepoStub) SetActivePublishedVersion(context.Context, string, string) error {
	return nil
}
func (questionnaireRepoStub) ClearActivePublishedVersion(context.Context, string) error {
	return nil
}
func (questionnaireRepoStub) Remove(context.Context, string) error     { return nil }
func (questionnaireRepoStub) HardDelete(context.Context, string) error { return nil }
func (questionnaireRepoStub) HardDeleteFamily(context.Context, string) error {
	return nil
}
func (questionnaireRepoStub) ExistsByCode(context.Context, string) (bool, error) {
	return false, nil
}
func (questionnaireRepoStub) HasPublishedSnapshots(context.Context, string) (bool, error) {
	return false, nil
}
