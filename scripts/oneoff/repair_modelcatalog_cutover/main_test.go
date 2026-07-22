package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelconclusion "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modelfactor "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestParseConfigDefaultsToDryRun(t *testing.T) {
	t.Parallel()
	getenv := func(key string) string {
		return map[string]string{
			"MYSQL_DSN": "user:pass@tcp(mysql:3306)/qs",
			"MONGO_URI": "mongodb://mongo/qs",
			"MONGO_DB":  "qs",
		}[key]
	}
	var stderr bytes.Buffer
	cfg, err := parseConfig(nil, &stderr, getenv)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.apply || cfg.mongoDB != "qs" || cfg.timeout != 10*time.Minute {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestRunHelpExitsSuccessfullyWithoutConnecting(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--help"}, &stdout, &stderr, nil); code != exitOK {
		t.Fatalf("run(--help) exit = %d, stderr=%q", code, stderr.String())
	}
}

func TestRepairPlanTextIncludesExactBlockingReason(t *testing.T) {
	t.Parallel()
	plan := &repairPlan{Mode: "dry-run", Issues: []repairIssue{{
		Scope: "assessment_norms", Code: "brief-v1", Rule: "validate_import.failed",
		Message: "factor F1 lookup rows 1 and 2 overlap", Count: 1,
	}}}
	text := plan.Text()
	if !plan.Blocked() || !strings.Contains(text, "brief-v1") || !strings.Contains(text, "rows 1 and 2 overlap") {
		t.Fatalf("plan text = %q", text)
	}
}

func TestRepairPlanCoalescesRepeatedFindings(t *testing.T) {
	t.Parallel()
	plan := &repairPlan{Issues: []repairIssue{
		{Scope: "published_runtime", Code: "M1@v1", Rule: "outcome.not_found", Message: "outcome low is not defined", Count: 1},
		{Scope: "published_runtime", Code: "M1@v1", Rule: "outcome.not_found", Message: "outcome low is not defined", Count: 1},
		{Scope: "published_runtime", Code: "M1@v1", Rule: "outcome.not_found", Message: "outcome high is not defined", Count: 1},
	}}
	plan.coalesceIssues()
	if len(plan.Issues) != 2 || plan.findingCount() != 3 {
		t.Fatalf("issues=%#v groups=%d findings=%d", plan.Issues, len(plan.Issues), plan.findingCount())
	}
	text := plan.Text()
	if !strings.Contains(text, "issue_groups=2 findings=3") || !strings.Contains(text, "count=2") {
		t.Fatalf("plan text = %q", text)
	}
}

func TestPublicRepairItemIgnoresOneTimeNormalizationEvidence(t *testing.T) {
	t.Parallel()
	item := repairItem{Code: "M1", Version: "v1", Normalizations: []string{"range_endpoint:1"}, snapshot: &modelcatalogport.AssessmentSnapshot{Code: "M1"}}
	public := publicRepairItem(item)
	if public.snapshot != nil || public.Normalizations != nil || public.Code != "M1" || public.Version != "v1" {
		t.Fatalf("public item = %#v", public)
	}
}

func TestNormalizeLegacyDefinitionMakesOnlyUniqueCompatibilitySemanticsExplicit(t *testing.T) {
	t.Parallel()
	definition := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{Scoring: []modelfactor.Scoring{{
			FactorCode: "F1",
			Sources: []modelfactor.ScoringSource{
				{Kind: modelfactor.ScoringSourceQuestion, Code: "Q1"},
				{Kind: modelfactor.ScoringSourceQuestion, Code: "Q2", OptionScores: map[string]float64{"A": 2}},
				{Kind: modelfactor.ScoringSourceQuestion, Code: "Q3", OptionScores: map[string]float64{}},
			},
		}}},
		Conclusions: []modelconclusion.Conclusion{modelconclusion.NormConclusion{
			FactorCode: "F1",
			Rules: []modelconclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 59, Level: "low"},
				{MinScore: 60, MaxScore: 69, Level: "medium"},
				{MinScore: 70, MaxScore: 100, Level: "high"},
			},
		}},
		Outcomes: []modelconclusion.Outcome{{Code: "low"}, {Code: "medium"}, {Code: "high"}},
	}

	got := normalizeLegacyDefinition(definition)
	if !reflect.DeepEqual(got, []string{
		"scoring_mode:2", "contribution_defaults:4", "outcome_code:3", "range_adjacency:2", "range_endpoint:1",
	}) {
		t.Fatalf("normalizations = %v", got)
	}
	sources := definition.Measure.Scoring[0].Sources
	if sources[0].ScoringMode != modelfactor.QuestionScoringModeQuestionScore || sources[0].Sign != 1 || sources[0].Weight != 1 {
		t.Fatalf("question score source = %#v", sources[0])
	}
	if sources[1].ScoringMode != modelfactor.QuestionScoringModeOptionOverride || sources[1].Sign != 1 || sources[1].Weight != 1 {
		t.Fatalf("option override source = %#v", sources[1])
	}
	if sources[2].ScoringMode != "" {
		t.Fatalf("ambiguous empty option map must remain blocked: %#v", sources[2])
	}
	rules := definition.Conclusions[0].(modelconclusion.NormConclusion).Rules
	if rules[0].OutcomeCode != "low" || rules[0].MaxScore != 60 || rules[0].MaxInclusive ||
		rules[1].OutcomeCode != "medium" || rules[1].MaxScore != 70 || rules[1].MaxInclusive ||
		rules[2].OutcomeCode != "high" || !rules[2].MaxInclusive {
		t.Fatalf("normalized rules = %#v", rules)
	}
}

func TestNormalizeLegacyDefinitionDoesNotGuessNonUnitOrFractionalGaps(t *testing.T) {
	t.Parallel()
	rules := []modelconclusion.ScoreRangeOutcome{
		{MinScore: 0, MaxScore: 10.5, OutcomeCode: "low"},
		{MinScore: 11.5, MaxScore: 100, OutcomeCode: "high"},
	}
	definition := &modeldefinition.Definition{Conclusions: []modelconclusion.Conclusion{
		modelconclusion.RiskConclusion{FactorCode: "F1", Rules: rules},
	}}
	got := normalizeLegacyDefinition(definition)
	normalized := definition.Conclusions[0].(modelconclusion.RiskConclusion).Rules
	if normalized[0].MaxScore != 10.5 || !reflect.DeepEqual(got, []string{"range_endpoint:1"}) {
		t.Fatalf("normalizations=%v rules=%#v", got, normalized)
	}
}

func TestNormalizeLegacyDefinitionDoesNotPromoteDisplayLevelToUnknownOutcomeCode(t *testing.T) {
	t.Parallel()
	definition := &modeldefinition.Definition{
		Outcomes: []modelconclusion.Outcome{{Code: "moderate"}},
		Conclusions: []modelconclusion.Conclusion{modelconclusion.RiskConclusion{
			FactorCode: "F1",
			Rules: []modelconclusion.ScoreRangeOutcome{{
				MinScore: 0, MaxScore: 10, Level: "中度", MaxInclusive: true,
			}},
		}},
	}
	if got := normalizeLegacyDefinition(definition); len(got) != 0 {
		t.Fatalf("normalizations = %v", got)
	}
	rule := definition.Conclusions[0].(modelconclusion.RiskConclusion).Rules[0]
	if rule.OutcomeCode != "" {
		t.Fatalf("outcome code = %q, want blocked empty code", rule.OutcomeCode)
	}
}

func TestMySQLDerivedHistoryTablesIncludesAllStatisticsAndAnalyticsTables(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).
		AddRow("statistics_assessment_fact").
		AddRow("analytics_pending_event"))
	tables, err := mysqlDerivedHistoryTables(context.Background(), db)
	if err != nil {
		t.Fatalf("mysqlDerivedHistoryTables() error = %v", err)
	}
	for _, want := range []string{"assessment", "statistics_assessment_fact", "analytics_pending_event"} {
		found := false
		for _, table := range tables {
			found = found || table == want
		}
		if !found {
			t.Fatalf("tables %v missing %s", tables, want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCanonicalSnapshotUpdateUsesCanonicalFieldsAndPreservesAuditOwnership(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0).UTC()
	snapshot := &modelcatalogport.AssessmentSnapshot{
		SchemaVersion: "definition-v2", Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		AlgorithmFamily: domain.AlgorithmFamilyTaskPerformance, DecisionKind: domain.DecisionKindAbilityLevel,
		Code: "SPM", Version: "v3", Status: "published", ReleaseStatus: domain.ReleaseStatusActive,
		QuestionnaireCode: "Q-SPM", QuestionnaireVersion: "3.0.0", PublishedAt: &now,
		DefinitionV2: &modeldefinition.Definition{},
		Source: map[string]any{
			modelcatalogport.SourceDefinitionContentHash: "hash",
			modelcatalogport.SourceDefinitionHashSchema:  "definition-v2/v1",
		},
	}
	update, err := canonicalSnapshotUpdate(snapshot)
	if err != nil {
		t.Fatalf("canonicalSnapshotUpdate() error = %v", err)
	}
	set := update["$set"].(bson.M)
	unset := update["$unset"].(bson.M)
	if set["algorithm_family"] != string(domain.AlgorithmFamilyTaskPerformance) || set["decision_kind"] != string(domain.DecisionKindAbilityLevel) {
		t.Fatalf("canonical set = %#v", set)
	}
	for _, field := range []string{"_id", "created_at", "created_by", "deleted_at", "deleted_by"} {
		if _, exists := set[field]; exists {
			t.Fatalf("canonical set unexpectedly owns %s: %#v", field, set)
		}
	}
	for _, field := range []string{"payload", "definition_payload", "is_active_published", "release_archived_at"} {
		if _, exists := unset[field]; !exists {
			t.Fatalf("canonical unset missing %s: %#v", field, unset)
		}
	}
}

func TestApplyRepairDocumentsChecksEveryWriteBoundary(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0).UTC()
	snapshot := &modelcatalogport.AssessmentSnapshot{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		AlgorithmFamily: domain.AlgorithmFamilyTaskPerformance, DecisionKind: domain.DecisionKindAbilityLevel,
		Code: "SPM", Version: "v3", Status: "published", ReleaseStatus: domain.ReleaseStatusActive,
		QuestionnaireCode: "Q-SPM", QuestionnaireVersion: "3.0.0", PublishedAt: &now,
		DefinitionV2: &modeldefinition.Definition{}, Source: map[string]any{},
	}
	plan := &repairPlan{ArchivedSnapshotCount: 2, Repairs: []repairItem{{Code: "SPM", Version: "v3", snapshot: snapshot}}}
	collection := &recordingRepairCollection{
		deleteResult:     &mongo.DeleteResult{DeletedCount: 2},
		updateOneResults: []*mongo.UpdateResult{{MatchedCount: 1}},
	}
	if err := applyRepairDocuments(context.Background(), collection, plan); err != nil {
		t.Fatalf("applyRepairDocuments() error = %v", err)
	}
	if collection.deleteCalls != 1 || collection.updateOneCalls != 1 || collection.updateManyCalls != 1 {
		t.Fatalf("calls delete=%d updateOne=%d updateMany=%d", collection.deleteCalls, collection.updateOneCalls, collection.updateManyCalls)
	}
	if !reflect.DeepEqual(collection.deleteFilter, archivedSnapshotFilter()) {
		t.Fatalf("delete filter = %#v", collection.deleteFilter)
	}
}

func TestApplyRepairDocumentsStopsOnSnapshotCASMismatch(t *testing.T) {
	t.Parallel()
	plan := minimalRepairPlan()
	collection := &recordingRepairCollection{
		deleteResult:     &mongo.DeleteResult{},
		updateOneResults: []*mongo.UpdateResult{{MatchedCount: 0}},
	}
	err := applyRepairDocuments(context.Background(), collection, plan)
	if err == nil || !strings.Contains(err.Error(), "matched=0 want=1") {
		t.Fatalf("applyRepairDocuments() error = %v", err)
	}
	if collection.updateManyCalls != 0 {
		t.Fatalf("legacy cleanup ran after CAS mismatch")
	}
}

func TestApplyRepairDocumentsPropagatesLegacyCleanupFailure(t *testing.T) {
	t.Parallel()
	plan := minimalRepairPlan()
	collection := &recordingRepairCollection{
		deleteResult:     &mongo.DeleteResult{},
		updateOneResults: []*mongo.UpdateResult{{MatchedCount: 1}},
		updateManyErr:    errors.New("write failed"),
	}
	err := applyRepairDocuments(context.Background(), collection, plan)
	if err == nil || !strings.Contains(err.Error(), "remove legacy model fields") {
		t.Fatalf("applyRepairDocuments() error = %v", err)
	}
}

func minimalRepairPlan() *repairPlan {
	snapshot := &modelcatalogport.AssessmentSnapshot{Code: "M1", Version: "v1", Status: "published", DefinitionV2: &modeldefinition.Definition{}, Source: map[string]any{}}
	return &repairPlan{Repairs: []repairItem{{Code: "M1", Version: "v1", snapshot: snapshot}}}
}

type recordingRepairCollection struct {
	deleteResult     *mongo.DeleteResult
	deleteErr        error
	updateOneResults []*mongo.UpdateResult
	updateOneErr     error
	updateManyErr    error
	deleteFilter     interface{}
	deleteCalls      int
	updateOneCalls   int
	updateManyCalls  int
}

func (c *recordingRepairCollection) DeleteMany(_ context.Context, filter interface{}, _ ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
	c.deleteCalls++
	c.deleteFilter = filter
	if c.deleteResult == nil {
		c.deleteResult = &mongo.DeleteResult{}
	}
	return c.deleteResult, c.deleteErr
}

func (c *recordingRepairCollection) UpdateOne(_ context.Context, _, _ interface{}, _ ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	c.updateOneCalls++
	if c.updateOneErr != nil {
		return nil, c.updateOneErr
	}
	if len(c.updateOneResults) == 0 {
		return &mongo.UpdateResult{}, nil
	}
	result := c.updateOneResults[0]
	c.updateOneResults = c.updateOneResults[1:]
	return result, nil
}

func (c *recordingRepairCollection) UpdateMany(_ context.Context, _, _ interface{}, _ ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	c.updateManyCalls++
	return &mongo.UpdateResult{}, c.updateManyErr
}
