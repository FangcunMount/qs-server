// audit_scale_models is a read-only integrity checker for canonical Scale
// drafts and published snapshots. It deliberately does not judge rows in the
// retired legacy `scales` collection.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

type config struct {
	mongoURI string
	mongoDB  string
	codes    string
	json     bool
	timeout  time.Duration
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "audit scale models failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit scale models failed: connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "audit scale models failed: ping mongo:", err)
		os.Exit(1)
	}
	report, err := audit(ctx, client.Database(cfg.mongoDB), splitCodes(cfg.codes))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit scale models failed:", err)
		os.Exit(1)
	}
	if cfg.json {
		_ = json.NewEncoder(os.Stdout).Encode(report)
	} else {
		printReport(report)
	}
	if report.ErrorCount > 0 {
		os.Exit(2)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.StringVar(&cfg.codes, "codes", "", "optional comma-separated scale model codes")
	flag.BoolVar(&cfg.json, "json", false, "write JSON report")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "MongoDB audit timeout")
	flag.Parse()
	return cfg
}

type report struct {
	DraftCount     int          `json:"draft_count"`
	PublishedCount int          `json:"published_count"`
	ErrorCount     int          `json:"error_count"`
	Issues         []auditIssue `json:"issues"`
}

type auditIssue struct {
	Scope   string `json:"scope"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

func audit(ctx context.Context, db *mongo.Database, codes []string) (*report, error) {
	drafts, err := loadDrafts(ctx, db, codes)
	if err != nil {
		return nil, err
	}
	published, err := loadPublished(ctx, db, codes)
	if err != nil {
		return nil, err
	}
	questionnaires := newQuestionnaireCache(db)
	report := &report{DraftCount: len(drafts), PublishedCount: len(published)}
	draftByCode := make(map[string]*domain.AssessmentModel, len(drafts))
	for _, draft := range drafts {
		draftByCode[draft.Code] = draft
		report.Issues = append(report.Issues, auditDraft(ctx, draft, questionnaires)...)
	}
	for _, snapshot := range published {
		report.Issues = append(report.Issues, auditPublished(ctx, snapshot, draftByCode[snapshot.Code], questionnaires)...)
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Code != report.Issues[j].Code {
			return report.Issues[i].Code < report.Issues[j].Code
		}
		if report.Issues[i].Scope != report.Issues[j].Scope {
			return report.Issues[i].Scope < report.Issues[j].Scope
		}
		return report.Issues[i].Rule < report.Issues[j].Rule
	})
	report.ErrorCount = len(report.Issues)
	return report, nil
}

func loadDrafts(ctx context.Context, db *mongo.Database, codes []string) ([]*domain.AssessmentModel, error) {
	filter := bson.M{"deleted_at": nil, "kind": string(domain.KindScale)}
	if len(codes) > 0 {
		filter["code"] = bson.M{"$in": codes}
	}
	cursor, err := db.Collection("assessment_models").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "code", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("find scale drafts: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	mapper := mongomodelcatalog.NewDraftMapper()
	models := make([]*domain.AssessmentModel, 0)
	for cursor.Next(ctx) {
		var po mongomodelcatalog.AssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, fmt.Errorf("decode scale draft: %w", err)
		}
		models = append(models, mapper.ToDomain(&po))
	}
	return models, cursor.Err()
}

func loadPublished(ctx context.Context, db *mongo.Database, codes []string) ([]*modelcatalogport.AssessmentSnapshot, error) {
	filter := bson.M{"deleted_at": nil, "status": "published", "model_kind": string(domain.KindScale)}
	if len(codes) > 0 {
		filter["model_code"] = bson.M{"$in": codes}
	}
	cursor, err := db.Collection("published_assessment_models").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "model_code", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("find published scale snapshots: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	mapper := mongomodelcatalog.NewMapper()
	models := make([]*modelcatalogport.AssessmentSnapshot, 0)
	for cursor.Next(ctx) {
		var po mongomodelcatalog.PublishedAssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, fmt.Errorf("decode published scale snapshot: %w", err)
		}
		models = append(models, mapper.ToPublished(&po))
	}
	return models, cursor.Err()
}

type questionnaireCache struct {
	collection *mongo.Collection
	items      map[string]*mongoquestionnaire.QuestionnairePO
}

func newQuestionnaireCache(db *mongo.Database) *questionnaireCache {
	return &questionnaireCache{collection: db.Collection("questionnaires"), items: make(map[string]*mongoquestionnaire.QuestionnairePO)}
}

func (c *questionnaireCache) load(ctx context.Context, code, version string) (*mongoquestionnaire.QuestionnairePO, error) {
	key := code + "@" + version
	if item, ok := c.items[key]; ok {
		return item, nil
	}
	var po mongoquestionnaire.QuestionnairePO
	err := c.collection.FindOne(ctx, bson.M{"code": code, "version": version, "record_role": "published_snapshot", "deleted_at": nil}).Decode(&po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("published questionnaire %s@%s not found", code, version)
		}
		return nil, err
	}
	c.items[key] = &po
	return &po, nil
}

func auditDraft(ctx context.Context, model *domain.AssessmentModel, questionnaires *questionnaireCache) []auditIssue {
	issues := make([]auditIssue, 0)
	if model == nil {
		return append(issues, issue("draft", "", "model", "model.required", "draft model is nil"))
	}
	if model.Algorithm != domain.AlgorithmScaleDefault {
		issues = append(issues, issue("draft", model.Code, "algorithm", "scale.algorithm", "scale algorithm must be scale_default"))
	}
	handler := appdefinition.ScaleDefinitionHandler{}
	for _, validation := range handler.ValidateForPublish(ctx, model) {
		issues = append(issues, issue("draft", model.Code, validation.Field, validation.Code, validation.Message))
	}
	questionnaire, err := questionnaires.load(ctx, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if err != nil {
		return append(issues, issue("draft", model.Code, "binding.questionnaire", "questionnaire.binding.invalid", err.Error()))
	}
	return append(issues, validateQuestionSources("draft", model.Code, model.DefinitionV2, questionnaire)...)
}

func auditPublished(ctx context.Context, snapshot *modelcatalogport.AssessmentSnapshot, draft *domain.AssessmentModel, questionnaires *questionnaireCache) []auditIssue {
	issues := make([]auditIssue, 0)
	if snapshot == nil {
		return append(issues, issue("published", "", "model", "model.required", "published snapshot is nil"))
	}
	if snapshot.SchemaVersion != domain.SchemaVersionV2 {
		issues = append(issues, issue("published", snapshot.Code, "schema_version", "schema.version", "published scale snapshot must use schema v2"))
	}
	if snapshot.Algorithm != domain.AlgorithmScaleDefault {
		issues = append(issues, issue("published", snapshot.Code, "algorithm", "scale.algorithm", "published scale algorithm must be scale_default"))
	}
	if snapshot.PayloadFormat != domain.PayloadFormatAssessmentScaleV1 {
		issues = append(issues, issue("published", snapshot.Code, "payload_format", "scale.payload_format", "published scale payload format must be assessmentmodel.scale.v1"))
	}
	if snapshot.DecisionKind != domain.DecisionKindScoreRange {
		issues = append(issues, issue("published", snapshot.Code, "decision_kind", "scale.decision_kind", "published scale decision kind must be score_range"))
	}
	for _, validation := range appdefinition.ValidateDefinitionV2(snapshot.DefinitionV2) {
		issues = append(issues, issue("published", snapshot.Code, validation.Field, validation.Code, validation.Message))
	}
	questionnaire, err := questionnaires.load(ctx, snapshot.QuestionnaireCode, snapshot.QuestionnaireVersion)
	if err != nil {
		return append(issues, issue("published", snapshot.Code, "questionnaire", "questionnaire.binding.invalid", err.Error()))
	}
	issues = append(issues, validateQuestionSources("published", snapshot.Code, snapshot.DefinitionV2, questionnaire)...)
	issues = append(issues, validateScalePayload(snapshot)...)
	if draft == nil {
		return append(issues, issue("published", snapshot.Code, "draft", "draft.missing", "published scale has no active draft model"))
	}
	if draft.Binding.QuestionnaireCode != snapshot.QuestionnaireCode || draft.Binding.QuestionnaireVersion != snapshot.QuestionnaireVersion {
		issues = append(issues, issue("published", snapshot.Code, "questionnaire", "draft.published.binding_mismatch", "draft and published questionnaire bindings differ"))
	}
	if draft.IsPublished() && snapshot.Version != fmt.Sprintf("v%d", draft.Revision()) {
		issues = append(issues, issue("published", snapshot.Code, "version", "draft.published.version_mismatch", fmt.Sprintf("published version %s does not equal draft revision v%d", snapshot.Version, draft.Revision())))
	}
	return issues
}

func validateQuestionSources(scope, code string, definition *modeldefinition.Definition, questionnaire *mongoquestionnaire.QuestionnairePO) []auditIssue {
	if definition == nil {
		return []auditIssue{issue(scope, code, "definition_v2", "definition_v2.required", "DefinitionV2 is required")}
	}
	if len(definition.Measure.Factors) == 0 {
		return []auditIssue{issue(scope, code, "definition_v2.measure.factors", "scale.factors.required", "scale DefinitionV2 must define at least one factor")}
	}
	questions := make(map[string]map[string]struct{}, len(questionnaire.Questions))
	for _, question := range questionnaire.Questions {
		options := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			options[option.Code] = struct{}{}
		}
		questions[question.Code] = options
	}
	issues := make([]auditIssue, 0)
	for _, scoring := range definition.Measure.Scoring {
		for _, source := range scoring.Sources {
			if source.Kind != factor.ScoringSourceQuestion {
				continue
			}
			options, ok := questions[source.Code]
			if !ok {
				issues = append(issues, issue(scope, code, "definition_v2.measure.scoring", "questionnaire.question.not_found", fmt.Sprintf("factor %s references question %s not present in bound published questionnaire", scoring.FactorCode, source.Code)))
				continue
			}
			for optionCode := range source.OptionScores {
				if _, ok := options[optionCode]; !ok {
					issues = append(issues, issue(scope, code, "definition_v2.measure.scoring", "questionnaire.option.not_found", fmt.Sprintf("factor %s question %s references option %s not present in bound published questionnaire", scoring.FactorCode, source.Code, optionCode)))
				}
			}
		}
	}
	return issues
}

func validateScalePayload(snapshot *modelcatalogport.AssessmentSnapshot) []auditIssue {
	if snapshot == nil || snapshot.DefinitionV2 == nil {
		return nil
	}
	actual, err := scalepayload.ParsePublishedPayload(snapshot.Payload)
	if err != nil {
		return []auditIssue{issue("published", snapshot.Code, "payload", "scale.payload.decode", err.Error())}
	}
	expected := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{Code: snapshot.Code, ScaleVersion: snapshot.Version, Title: snapshot.Title, QuestionnaireCode: snapshot.QuestionnaireCode, QuestionnaireVersion: snapshot.QuestionnaireVersion, Status: "published"}, snapshot.DefinitionV2)
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	if actualErr != nil || expectedErr != nil {
		return []auditIssue{issue("published", snapshot.Code, "payload", "scale.payload.encode", "cannot encode scale payload for comparison")}
	}
	if !bytes.Equal(actualJSON, expectedJSON) {
		return []auditIssue{issue("published", snapshot.Code, "payload", "scale.payload.definition_mismatch", "published scale payload does not equal the projection of DefinitionV2")}
	}
	return nil
}

func issue(scope, code, field, rule, message string) auditIssue {
	return auditIssue{Scope: scope, Code: code, Field: field, Rule: rule, Message: message}
}

func printReport(report *report) {
	fmt.Printf("scale drafts: %d\n", report.DraftCount)
	fmt.Printf("published scale snapshots: %d\n", report.PublishedCount)
	fmt.Printf("integrity issues: %d\n", report.ErrorCount)
	for _, item := range report.Issues {
		fmt.Printf("- [%s] %s %s (%s): %s\n", item.Scope, item.Code, item.Field, item.Rule, item.Message)
	}
}

func splitCodes(raw string) []string {
	seen := map[string]struct{}{}
	for _, code := range strings.Split(raw, ",") {
		if code = strings.TrimSpace(code); code != "" {
			seen[code] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
