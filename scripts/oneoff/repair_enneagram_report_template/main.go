// repair_enneagram_report_template repairs one source-proven report-template
// mismatch on the active ENNEAGRAM_45@v16 snapshot.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/scripts/oneoff/internal/modelseed"
)

const (
	exitOK          = 0
	exitUnavailable = 1
	exitBlocked     = 2

	targetCode    = "ENNEAGRAM_45"
	targetVersion = "v16"
	oldTemplate   = "mbti"
	newTemplate   = "enneagram"
)

type config struct {
	mongoURI string
	mongoDB  string
	apply    bool
	jsonOut  bool
	timeout  time.Duration
}

type repairPlan struct {
	Mode           string    `json:"mode"`
	GeneratedAt    time.Time `json:"generated_at"`
	Code           string    `json:"code"`
	Version        string    `json:"version"`
	BeforeTemplate string    `json:"before_template"`
	AfterTemplate  string    `json:"after_template"`
	BeforeHash     string    `json:"before_hash"`
	AfterHash      string    `json:"after_hash"`
	Action         string    `json:"action"`
	Issues         []string  `json:"issues,omitempty"`

	documentID        any
	expectedUpdated   time.Time
	desiredDefinition *mongomodelcatalog.DefinitionPO
	desiredSource     bson.M
}

func (p repairPlan) blocked() bool { return len(p.Issues) > 0 }

func (p repairPlan) text() string {
	var out strings.Builder
	fmt.Fprintf(&out, "Enneagram report-template repair: mode=%s model=%s@%s action=%s generated_at=%s\n",
		p.Mode, p.Code, p.Version, p.Action, p.GeneratedAt.UTC().Format(time.RFC3339))
	if p.BeforeTemplate != "" || p.AfterTemplate != "" {
		fmt.Fprintf(&out, "- template: %s -> %s\n", p.BeforeTemplate, p.AfterTemplate)
	}
	if p.BeforeHash != "" || p.AfterHash != "" {
		fmt.Fprintf(&out, "- definition_hash: %s -> %s\n", p.BeforeHash, p.AfterHash)
	}
	for _, issue := range p.Issues {
		fmt.Fprintf(&out, "- BLOCKED: %s\n", issue)
	}
	if !p.blocked() && p.Action == "update" {
		out.WriteString("PASS: exact ENNEAGRAM_45@v16 repair is apply-safe\n")
	}
	if !p.blocked() && p.Action == "noop" {
		out.WriteString("PASS: ENNEAGRAM_45@v16 is already repaired\n")
	}
	return out.String()
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	cfg, err := parseConfig(args, stderr, getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, "repair enneagram report template:", err)
		return exitUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: connect mongo:", err)
		return exitUnavailable
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: ping mongo:", err)
		return exitUnavailable
	}

	collection := client.Database(cfg.mongoDB).Collection("assessment_models")
	plan, err := buildPlan(ctx, collection)
	if err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: build plan:", err)
		return exitUnavailable
	}
	plan.Mode = "dry-run"
	if cfg.apply {
		plan.Mode = "apply"
	}
	if err := writePlan(stdout, plan, cfg.jsonOut); err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: write plan:", err)
		return exitUnavailable
	}
	if plan.blocked() {
		return exitBlocked
	}
	if !cfg.apply || plan.Action == "noop" {
		return exitOK
	}
	if err := requireWritableReplicaSet(ctx, client); err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: apply guard:", err)
		return exitUnavailable
	}
	if err := applyPlan(ctx, client, collection, plan); err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: apply:", err)
		return exitUnavailable
	}
	verified, err := buildPlan(ctx, collection)
	if err != nil {
		fmt.Fprintln(stderr, "repair enneagram report template: post-apply verify:", err)
		return exitUnavailable
	}
	if verified.blocked() || verified.Action != "noop" || verified.BeforeHash != plan.AfterHash {
		fmt.Fprintln(stderr, "repair enneagram report template: post-apply state is not canonical")
		return exitUnavailable
	}
	fmt.Fprintln(stdout, "ENNEAGRAM_REPORT_TEMPLATE_REPAIR_OK")
	return exitOK
}

func parseConfig(args []string, stderr io.Writer, getenv func(string) string) (config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := config{}
	flags := flag.NewFlagSet("repair_enneagram_report_template", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.mongoURI, "mongo-uri", getenv("MONGO_URI"), "MongoDB URI")
	flags.StringVar(&cfg.mongoDB, "mongo-db", envOr(getenv("MONGO_DB"), "qs_server"), "MongoDB database")
	flags.BoolVar(&cfg.apply, "apply", false, "apply the exact guarded repair")
	flags.BoolVar(&cfg.jsonOut, "json", false, "emit JSON repair plan")
	flags.DurationVar(&cfg.timeout, "timeout", 2*time.Minute, "command timeout")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if cfg.mongoURI == "" {
		return config{}, fmt.Errorf("--mongo-uri or MONGO_URI is required")
	}
	if cfg.mongoDB == "" || cfg.timeout <= 0 {
		return config{}, fmt.Errorf("--mongo-db and a positive --timeout are required")
	}
	return cfg, nil
}

func envOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func writePlan(w io.Writer, plan repairPlan, jsonOut bool) error {
	if jsonOut {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	_, err := io.WriteString(w, plan.text())
	return err
}

type modelCollection interface {
	CountDocuments(context.Context, interface{}, ...*options.CountOptions) (int64, error)
	FindOne(context.Context, interface{}, ...*options.FindOneOptions) *mongo.SingleResult
	UpdateOne(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

func targetFilter() bson.M {
	return bson.M{
		"code": targetCode, "release_version": targetVersion,
		"record_role": "published_snapshot", "release_status": "active",
		"status": "published", "deleted_at": nil,
	}
}

func buildPlan(ctx context.Context, collection modelCollection) (repairPlan, error) {
	plan := repairPlan{GeneratedAt: time.Now().UTC(), Code: targetCode, Version: targetVersion, Action: "blocked"}
	count, err := collection.CountDocuments(ctx, targetFilter())
	if err != nil {
		return repairPlan{}, err
	}
	if count != 1 {
		plan.Issues = append(plan.Issues, fmt.Sprintf("active snapshot count=%d, want 1", count))
		return plan, nil
	}
	var po mongomodelcatalog.PublishedAssessmentModelPO
	if err := collection.FindOne(ctx, targetFilter()).Decode(&po); err != nil {
		return repairPlan{}, err
	}
	model := mongomodelcatalog.NewMapper().ToPublished(&po)
	if po.ID.IsZero() || po.UpdatedAt.IsZero() {
		plan.Issues = append(plan.Issues, "active snapshot _id/updated_at CAS material is incomplete")
		return plan, nil
	}
	prepared, desired, err := preparePlan(model)
	if err != nil {
		plan.Issues = append(plan.Issues, err.Error())
		return plan, nil
	}
	plan.BeforeTemplate = prepared.BeforeTemplate
	plan.AfterTemplate = prepared.AfterTemplate
	plan.BeforeHash = prepared.BeforeHash
	plan.AfterHash = prepared.AfterHash
	plan.Action = prepared.Action
	plan.documentID = po.ID
	plan.expectedUpdated = po.UpdatedAt
	if desired != nil {
		desiredPO := mongomodelcatalog.NewMapper().ToPO(desired)
		plan.desiredDefinition = desiredPO.DefinitionV2
		plan.desiredSource = desiredPO.Source
	}
	return plan, nil
}

type preparedPlan struct {
	BeforeTemplate string
	AfterTemplate  string
	BeforeHash     string
	AfterHash      string
	Action         string
}

func preparePlan(source *modelcatalogport.PublishedModel) (preparedPlan, *modelcatalogport.PublishedModel, error) {
	if source == nil || source.DefinitionV2 == nil {
		return preparedPlan{}, nil, fmt.Errorf("DefinitionV2 is required")
	}
	if source.Code != targetCode || source.Version != targetVersion ||
		source.Kind != domain.KindTypology || source.SubKind != domain.SubKindTypology ||
		source.Algorithm != domain.AlgorithmPersonalityTypology ||
		source.AlgorithmFamily != domain.AlgorithmFamilyFactorClassification ||
		source.DecisionKind != domain.DecisionKindTraitProfile {
		return preparedPlan{}, nil, fmt.Errorf("active snapshot runtime identity does not match the exact Enneagram trait-profile target")
	}
	beforeHash := modelcatalogport.DefinitionHashFromSource(source.Source)
	actualBeforeHash, err := modeldefinition.CanonicalContentHash(source.DefinitionV2)
	if err != nil {
		return preparedPlan{}, nil, err
	}
	if beforeHash == "" || beforeHash != actualBeforeHash {
		return preparedPlan{}, nil, fmt.Errorf("stored definition hash does not match the active DefinitionV2")
	}
	if len(source.DefinitionV2.Conclusions) != 1 {
		return preparedPlan{}, nil, fmt.Errorf("expected exactly one type conclusion")
	}
	typeConclusion, ok := source.DefinitionV2.Conclusions[0].(conclusion.TypeConclusion)
	if !ok || typeConclusion.Decision.Kind != binding.DecisionKindTraitProfile || len(typeConclusion.Profiles) != 0 || len(typeConclusion.Outcomes) != 0 {
		return preparedPlan{}, nil, fmt.Errorf("type conclusion is not the expected outcome-free trait profile")
	}
	if len(source.DefinitionV2.Outcomes) != 0 || len(source.DefinitionV2.InterpretationAssets.Outcomes) != 0 || len(source.DefinitionV2.InterpretationAssets.Profiles) != 0 {
		return preparedPlan{}, nil, fmt.Errorf("unexpected outcomes/profiles found; refusing to rewrite authored presentation")
	}
	if len(source.DefinitionV2.ReportMap.Sections) != 1 {
		return preparedPlan{}, nil, fmt.Errorf("expected exactly one report section")
	}
	section := source.DefinitionV2.ReportMap.Sections[0]
	if section.Code != "trait_profile" || section.Title != "九型人格" || section.Kind != "trait_profile" ||
		section.AdapterKey != "trait_profile" || section.CategoryLabel != "九型人格" ||
		(section.TemplateID != oldTemplate && section.TemplateID != newTemplate) {
		return preparedPlan{}, nil, fmt.Errorf("report section does not match the exact known Enneagram template mismatch")
	}

	desired, err := clonePublishedModel(source)
	if err != nil {
		return preparedPlan{}, nil, err
	}
	desired.DefinitionV2.ReportMap.Sections[0].TemplateID = newTemplate
	modeldefinition.MaterializeLayers(desired.DefinitionV2)
	if issues := modeldefinition.Validate(*desired.DefinitionV2); len(issues) > 0 {
		return preparedPlan{}, nil, fmt.Errorf("repaired DefinitionV2 is invalid: %s: %s", issues[0].Code, issues[0].Message)
	}
	afterHash, err := modeldefinition.CanonicalContentHash(desired.DefinitionV2)
	if err != nil {
		return preparedPlan{}, nil, err
	}
	modelcatalogport.AttachDefinitionHash(desired, afterHash)
	action := "update"
	if section.TemplateID == newTemplate && beforeHash == afterHash {
		action = "noop"
	}
	return preparedPlan{
		BeforeTemplate: section.TemplateID, AfterTemplate: newTemplate,
		BeforeHash: beforeHash, AfterHash: afterHash, Action: action,
	}, desired, nil
}

func clonePublishedModel(source *modelcatalogport.PublishedModel) (*modelcatalogport.PublishedModel, error) {
	desired := *source
	raw, err := json.Marshal(source.DefinitionV2)
	if err != nil {
		return nil, err
	}
	var definition modeldefinition.Definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		return nil, err
	}
	desired.DefinitionV2 = &definition
	desired.Source = make(map[string]any, len(source.Source))
	for key, value := range source.Source {
		desired.Source[key] = value
	}
	return &desired, nil
}

func requireWritableReplicaSet(ctx context.Context, client *mongo.Client) error {
	var hello struct {
		SetName           string `bson:"setName"`
		IsWritablePrimary bool   `bson:"isWritablePrimary"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return err
	}
	if hello.SetName == "" || !hello.IsWritablePrimary {
		return fmt.Errorf("MongoDB must be a writable Replica Set primary")
	}
	return nil
}

func applyPlan(ctx context.Context, client *mongo.Client, collection modelCollection, plan repairPlan) error {
	if plan.blocked() || plan.Action != "update" || plan.desiredDefinition == nil {
		return fmt.Errorf("repair plan is not apply-safe")
	}
	runner := modelseed.NewMongoTransactionRunner(client)
	return modelseed.RunAtomically(ctx, runner, func(txCtx context.Context) error {
		result, err := collection.UpdateOne(txCtx, bson.M{
			"_id": plan.documentID, "updated_at": plan.expectedUpdated,
			"definition_v2.report_map.sections.0.template_id": plan.BeforeTemplate,
		}, bson.M{"$set": bson.M{
			"definition_v2":             plan.desiredDefinition,
			"definition_schema_version": domain.SchemaVersionV2,
			"source":                    plan.desiredSource,
			"updated_at":                time.Now().UTC(),
		}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 || result.ModifiedCount != 1 {
			return fmt.Errorf("CAS update matched=%d modified=%d, want 1/1", result.MatchedCount, result.ModifiedCount)
		}
		return nil
	})
}
