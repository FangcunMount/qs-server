// audit_ai_explanation_prompt_evaluation_size is a read-only capacity audit
// for Prompt evaluation Run documents. It never updates MongoDB.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	isoaiexplanation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	mongoMaxBSONBytes              = 16 << 20
	requiredCandidateSlots         = 35
	maxGenerationExecutionsPerSlot = 2
	maxSemanticExecutionsPerSlot   = 2
)

type config struct {
	mongoURI      string
	mongoHost     string
	mongoPort     string
	mongoUsername string
	mongoPassword string
	mongoAuthDB   string
	mongoDB       string
	maxRuns       int64
	reviewRunID   string
	jsonOut       bool
	timeout       time.Duration
}

type sizeDistribution struct {
	Observed int   `json:"observed"`
	Missing  int   `json:"missing"`
	P50      int64 `json:"p50_bytes"`
	P95      int64 `json:"p95_bytes"`
	Max      int64 `json:"max_bytes"`
}

type runSize struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	BSONBytes int64  `json:"bson_bytes"`
}

type projection struct {
	Available           bool   `json:"available"`
	Reason              string `json:"reason,omitempty"`
	CandidateSlots      int    `json:"candidate_slots"`
	GenerationMax       int    `json:"generation_executions_max"`
	SemanticMax         int    `json:"semantic_executions_max"`
	P95ProjectedBytes   int64  `json:"p95_projected_bytes,omitempty"`
	MaxProjectedBytes   int64  `json:"max_projected_bytes,omitempty"`
	MongoMaxBSONBytes   int64  `json:"mongo_max_bson_bytes"`
	P95HeadroomBytes    int64  `json:"p95_headroom_bytes,omitempty"`
	MaxHeadroomBytes    int64  `json:"max_headroom_bytes,omitempty"`
	P95WithinMongoLimit bool   `json:"p95_within_mongo_limit,omitempty"`
	MaxWithinMongoLimit bool   `json:"max_within_mongo_limit,omitempty"`
}

type report struct {
	ObservedAt                   time.Time        `json:"observed_at"`
	Collection                   string           `json:"collection"`
	MatchedRuns                  int64            `json:"matched_runs"`
	ScannedRuns                  int              `json:"scanned_runs"`
	Truncated                    bool             `json:"truncated"`
	GenerationExecutions         int              `json:"generation_executions"`
	SemanticExecutions           int              `json:"semantic_executions"`
	RunBSON                      sizeDistribution `json:"run_bson"`
	RunBSONWithoutOutputPayloads sizeDistribution `json:"run_bson_without_stored_output_payloads"`
	GenerationRawOutput          sizeDistribution `json:"generation_raw_output"`
	GenerationNormalizedOutput   sizeDistribution `json:"generation_normalized_output"`
	SemanticRawOutput            sizeDistribution `json:"semantic_raw_output"`
	SemanticNormalizedOutput     sizeDistribution `json:"semantic_normalized_output"`
	LargestRuns                  []runSize        `json:"largest_runs"`
	V2ObservedOutputProjection   projection       `json:"v2_observed_output_projection"`
}

type measurements struct {
	runBSON                      []int64
	runBSONWithoutOutputPayloads []int64
	generationRaw                []int64
	generationNormalized         []int64
	semanticRaw                  []int64
	semanticNormalized           []int64
	generationRawMissing         int
	generationNormalizedMissing  int
	semanticRawMissing           int
	semanticNormalizedMissing    int
	generationExecutions         int
	semanticExecutions           int
	largestRuns                  []runSize
}

type reviewAssertion struct {
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Ordinal   int    `json:"ordinal"`
	Hard      bool   `json:"hard"`
	Evaluator string `json:"evaluator"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
}

type reviewSemanticDecision struct {
	Type    string `json:"type"`
	Scope   string `json:"scope"`
	Ordinal int    `json:"ordinal"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

type reviewSemanticResult struct {
	EvaluatorVersion        string                   `json:"evaluator_version"`
	Faithfulness            int                      `json:"faithfulness"`
	CrossDimensionQuality   int                      `json:"cross_dimension_quality"`
	SuggestionActionability int                      `json:"suggestion_actionability"`
	AudienceClarity         int                      `json:"audience_clarity"`
	Concision               int                      `json:"concision"`
	Rationale               string                   `json:"rationale"`
	Decisions               []reviewSemanticDecision `json:"decisions"`
}

type reviewCandidate struct {
	CaseID                   string               `json:"case_id"`
	SlotOrdinal              int                  `json:"slot_ordinal"`
	CandidateID              string               `json:"candidate_id"`
	GenerationExecutionID    string               `json:"generation_execution_id"`
	SemanticExecutionID      string               `json:"semantic_execution_id"`
	NormalizedOutput         json.RawMessage      `json:"normalized_output"`
	Assertions               []reviewAssertion    `json:"assertions"`
	SemanticNormalizedOutput json.RawMessage      `json:"semantic_normalized_output"`
	Semantic                 reviewSemanticResult `json:"semantic"`
}

type reviewExport struct {
	ObservedAt   time.Time         `json:"observed_at"`
	RunID        string            `json:"run_id"`
	Status       string            `json:"status"`
	Version      int64             `json:"version"`
	HumanReviews int               `json:"human_reviews"`
	Candidates   []reviewCandidate `json:"candidates"`
}

func main() {
	cfg := parseFlags()
	clientOptions, err := mongoClientOptions(cfg)
	if err != nil {
		fail("validate mongo configuration", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		fail("connect mongo", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fail("ping mongo", err)
	}

	collectionName := (&isoaiexplanation.PromptEvaluationRunPO{}).CollectionName()
	coll := client.Database(cfg.mongoDB).Collection(collectionName)
	if strings.TrimSpace(cfg.reviewRunID) != "" {
		result, err := loadReviewExport(ctx, coll, cfg.reviewRunID)
		if err != nil {
			fail("export Prompt evaluation review evidence", err)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fail("encode review evidence", err)
		}
		return
	}
	matched, values, err := loadMeasurements(ctx, coll, cfg.maxRuns)
	if err != nil {
		fail("scan Prompt evaluation Runs", err)
	}
	result := buildReport(time.Now().UTC(), collectionName, matched, values)
	if cfg.jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fail("encode report", err)
		}
		return
	}
	printHuman(result)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoHost, "mongo-host", os.Getenv("MONGO_HOST"), "MongoDB host when MONGO_URI is not used")
	flag.StringVar(&cfg.mongoPort, "mongo-port", envOr("MONGO_PORT", "27017"), "MongoDB port when MONGO_URI is not used")
	flag.StringVar(&cfg.mongoUsername, "mongo-username", os.Getenv("MONGO_USERNAME"), "MongoDB username when MONGO_URI is not used")
	flag.StringVar(&cfg.mongoPassword, "mongo-password", os.Getenv("MONGO_PASSWORD"), "MongoDB password when MONGO_URI is not used; prefer the environment variable")
	flag.StringVar(&cfg.mongoAuthDB, "mongo-auth-db", envOr("MONGO_AUTH_DB", "admin"), "MongoDB authentication database when MONGO_URI is not used")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database")
	flag.Int64Var(&cfg.maxRuns, "max-runs", 1000, "maximum newest Runs to scan; 0 scans all")
	flag.StringVar(&cfg.reviewRunID, "review-run-id", "", "export review evidence for one v2 Run ID instead of size measurements")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit machine-readable JSON")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "operation timeout")
	flag.Parse()
	if cfg.maxRuns < 0 {
		fmt.Fprintln(os.Stderr, "AI explanation Prompt evaluation size audit failed: --max-runs cannot be negative")
		os.Exit(1)
	}
	return cfg
}

func loadReviewExport(ctx context.Context, coll *mongo.Collection, rawRunID string) (reviewExport, error) {
	runID, err := meta.ParseID(strings.TrimSpace(rawRunID))
	if err != nil {
		return reviewExport{}, fmt.Errorf("review Run ID is invalid: %w", err)
	}
	var document isoaiexplanation.PromptEvaluationEvidenceV2PO
	if err := coll.FindOne(ctx, bson.M{"domain_id": runID, "evidence_version": isoaiexplanation.PromptEvaluationEvidenceVersionV2}).Decode(&document); err != nil {
		return reviewExport{}, err
	}
	generationByID := make(map[string]domainevaluation.CandidateGenerationExecution, len(document.GenerationExecutions))
	for _, execution := range document.GenerationExecutions {
		generationByID[execution.ID] = execution
	}
	semanticByID := make(map[string]domainevaluation.SemanticEvaluationExecution, len(document.SemanticExecutions))
	for _, execution := range document.SemanticExecutions {
		semanticByID[execution.ID] = execution
	}
	result := reviewExport{
		ObservedAt: time.Now().UTC(), RunID: document.DomainID.String(), Status: document.Status,
		Version: document.Version, HumanReviews: len(document.HumanReviews), Candidates: make([]reviewCandidate, 0, len(document.Slots)),
	}
	for _, slot := range document.Slots {
		if slot.Candidate == nil || !slot.Candidate.ReviewReady {
			continue
		}
		generation, ok := generationByID[slot.Candidate.GenerationExecutionID]
		if !ok || !json.Valid(generation.NormalizedOutput) {
			return reviewExport{}, fmt.Errorf("candidate %s generation evidence is missing or invalid", slot.Candidate.ID)
		}
		semantic, ok := semanticByID[slot.Candidate.AcceptedSemanticExecutionID]
		if !ok || semantic.Result == nil || !json.Valid(semantic.NormalizedOutput) {
			return reviewExport{}, fmt.Errorf("candidate %s semantic evidence is missing or invalid", slot.Candidate.ID)
		}
		candidate := reviewCandidate{
			CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, CandidateID: slot.Candidate.ID,
			GenerationExecutionID: generation.ID, SemanticExecutionID: semantic.ID,
			NormalizedOutput:         append(json.RawMessage(nil), generation.NormalizedOutput...),
			SemanticNormalizedOutput: append(json.RawMessage(nil), semantic.NormalizedOutput...),
			Assertions:               make([]reviewAssertion, 0, len(slot.Candidate.Assertions)),
			Semantic: reviewSemanticResult{
				EvaluatorVersion:        semantic.Result.EvaluatorVersion,
				Faithfulness:            semantic.Result.Scores.Faithfulness,
				CrossDimensionQuality:   semantic.Result.Scores.CrossDimensionQuality,
				SuggestionActionability: semantic.Result.Scores.SuggestionActionability,
				AudienceClarity:         semantic.Result.Scores.AudienceClarity,
				Concision:               semantic.Result.Scores.Concision,
				Rationale:               semantic.Result.Rationale,
				Decisions:               make([]reviewSemanticDecision, 0, len(semantic.Result.Decisions)),
			},
		}
		for _, assertion := range slot.Candidate.Assertions {
			candidate.Assertions = append(candidate.Assertions, reviewAssertion{
				Type: assertion.Type, Scope: string(assertion.Scope), Ordinal: assertion.Ordinal, Hard: assertion.Hard,
				Evaluator: assertion.Evaluator, Status: string(assertion.Status), Detail: assertion.Detail,
			})
		}
		for _, decision := range semantic.Result.Decisions {
			candidate.Semantic.Decisions = append(candidate.Semantic.Decisions, reviewSemanticDecision{
				Type: decision.Type, Scope: string(decision.Scope), Ordinal: decision.Ordinal,
				Status: string(decision.Status), Detail: decision.Detail,
			})
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		if result.Candidates[i].CaseID == result.Candidates[j].CaseID {
			return result.Candidates[i].SlotOrdinal < result.Candidates[j].SlotOrdinal
		}
		return result.Candidates[i].CaseID < result.Candidates[j].CaseID
	})
	return result, nil
}

func mongoClientOptions(cfg config) (*options.ClientOptions, error) {
	if uri := strings.TrimSpace(cfg.mongoURI); uri != "" {
		return options.Client().ApplyURI(uri), nil
	}
	host := strings.TrimSpace(cfg.mongoHost)
	port := strings.TrimSpace(cfg.mongoPort)
	username := strings.TrimSpace(cfg.mongoUsername)
	authDB := strings.TrimSpace(cfg.mongoAuthDB)
	if host == "" || port == "" {
		return nil, fmt.Errorf("MONGO_URI or MONGO_HOST/MONGO_PORT is required")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, fmt.Errorf("mongo port is invalid")
	}
	if (username == "") != (cfg.mongoPassword == "") {
		return nil, fmt.Errorf("mongo username and password must be provided together")
	}
	result := options.Client().SetHosts([]string{net.JoinHostPort(host, port)})
	if username != "" {
		if authDB == "" {
			return nil, fmt.Errorf("mongo authentication database is required")
		}
		result.SetAuth(options.Credential{
			AuthSource: authDB,
			Username:   username,
			Password:   cfg.mongoPassword,
		})
	}
	return result, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "AI explanation Prompt evaluation size audit failed: %s: %v\n", stage, err)
	os.Exit(1)
}

func loadMeasurements(ctx context.Context, coll *mongo.Collection, maxRuns int64) (int64, measurements, error) {
	matched, err := coll.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, measurements{}, err
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if maxRuns > 0 {
		findOptions.SetLimit(maxRuns)
	}
	cursor, err := coll.Find(ctx, bson.D{}, findOptions)
	if err != nil {
		return 0, measurements{}, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	values := measurements{}
	for cursor.Next(ctx) {
		raw := append(bson.Raw(nil), cursor.Current...)
		var document isoaiexplanation.PromptEvaluationRunPO
		if err := bson.Unmarshal(raw, &document); err != nil {
			return 0, measurements{}, err
		}
		for index := range document.Attempts {
			attempt := document.Attempts[index]
			if attempt.Stage != "generation" {
				continue
			}
			values.generationExecutions++
			appendPresentOrMissing(&values.generationRaw, &values.generationRawMissing, attempt.RawOutput)
			appendPresentOrMissing(&values.generationNormalized, &values.generationNormalizedMissing, attempt.NormalizedOutput)
			if attempt.SemanticExecution != nil {
				values.semanticExecutions++
				appendPresentOrMissing(&values.semanticRaw, &values.semanticRawMissing, attempt.SemanticExecution.RawOutput)
				appendPresentOrMissing(&values.semanticNormalized, &values.semanticNormalizedMissing, attempt.SemanticExecution.NormalizedOutput)
			}
		}
		withoutOutputPayloads, err := runBSONWithoutStoredOutputPayloads(len(raw), document.Attempts)
		if err != nil {
			return 0, measurements{}, err
		}
		values.runBSON = append(values.runBSON, int64(len(raw)))
		values.runBSONWithoutOutputPayloads = append(values.runBSONWithoutOutputPayloads, withoutOutputPayloads)
		values.largestRuns = append(values.largestRuns, runSize{
			RunID: document.DomainID.String(), Status: document.Status, BSONBytes: int64(len(raw)),
		})
	}
	if err := cursor.Err(); err != nil {
		return 0, measurements{}, err
	}
	return matched, values, nil
}

func appendPresentOrMissing(values *[]int64, missing *int, raw []byte) {
	if len(raw) == 0 {
		(*missing)++
		return
	}
	*values = append(*values, int64(len(raw)))
}

func runBSONWithoutStoredOutputPayloads(rawBSONBytes int, attempts []isoaiexplanation.EvaluationAttemptPO) (int64, error) {
	storedOutputPayloadBytes := 0
	for _, attempt := range attempts {
		if attempt.Stage != "generation" {
			continue
		}
		storedOutputPayloadBytes += len(attempt.RawOutput) + len(attempt.NormalizedOutput)
		if attempt.SemanticExecution != nil {
			storedOutputPayloadBytes += len(attempt.SemanticExecution.RawOutput) + len(attempt.SemanticExecution.NormalizedOutput)
		}
	}
	withoutOutputPayloads := int64(rawBSONBytes - storedOutputPayloadBytes)
	if withoutOutputPayloads < 0 {
		return 0, fmt.Errorf("stored output payloads exceed raw BSON size")
	}
	return withoutOutputPayloads, nil
}

func buildReport(observedAt time.Time, collectionName string, matched int64, values measurements) report {
	sort.Slice(values.largestRuns, func(i, j int) bool {
		if values.largestRuns[i].BSONBytes == values.largestRuns[j].BSONBytes {
			return values.largestRuns[i].RunID < values.largestRuns[j].RunID
		}
		return values.largestRuns[i].BSONBytes > values.largestRuns[j].BSONBytes
	})
	if len(values.largestRuns) > 10 {
		values.largestRuns = values.largestRuns[:10]
	}
	result := report{
		ObservedAt: observedAt, Collection: collectionName, MatchedRuns: matched, ScannedRuns: len(values.runBSON),
		Truncated: matched > int64(len(values.runBSON)), GenerationExecutions: values.generationExecutions,
		SemanticExecutions:           values.semanticExecutions,
		RunBSON:                      distribution(values.runBSON, 0),
		RunBSONWithoutOutputPayloads: distribution(values.runBSONWithoutOutputPayloads, 0),
		GenerationRawOutput:          distribution(values.generationRaw, values.generationRawMissing),
		GenerationNormalizedOutput:   distribution(values.generationNormalized, values.generationNormalizedMissing),
		SemanticRawOutput:            distribution(values.semanticRaw, values.semanticRawMissing),
		SemanticNormalizedOutput:     distribution(values.semanticNormalized, values.semanticNormalizedMissing),
		LargestRuns:                  values.largestRuns,
	}
	result.V2ObservedOutputProjection = projectV2(result)
	return result
}

func distribution(values []int64, missing int) sizeDistribution {
	if len(values) == 0 {
		return sizeDistribution{Missing: missing}
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return sizeDistribution{
		Observed: len(ordered), Missing: missing,
		P50: nearestRank(ordered, 0.50), P95: nearestRank(ordered, 0.95), Max: ordered[len(ordered)-1],
	}
}

func nearestRank(ordered []int64, percentile float64) int64 {
	if len(ordered) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(ordered)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(ordered) {
		index = len(ordered) - 1
	}
	return ordered[index]
}

func projectV2(value report) projection {
	result := projection{
		CandidateSlots:    requiredCandidateSlots,
		GenerationMax:     requiredCandidateSlots * maxGenerationExecutionsPerSlot,
		SemanticMax:       requiredCandidateSlots * maxSemanticExecutionsPerSlot,
		MongoMaxBSONBytes: mongoMaxBSONBytes,
	}
	if value.Truncated {
		result.Reason = "scan is truncated; rerun with --max-runs 0 before making a capacity decision"
		return result
	}
	distributions := []sizeDistribution{
		value.RunBSONWithoutOutputPayloads, value.GenerationRawOutput, value.GenerationNormalizedOutput,
		value.SemanticRawOutput, value.SemanticNormalizedOutput,
	}
	for _, observed := range distributions {
		if observed.Observed == 0 {
			result.Reason = "generation and semantic output samples are both required for projection"
			return result
		}
	}
	result.Available = true
	result.P95ProjectedBytes = value.RunBSONWithoutOutputPayloads.P95 + int64(result.GenerationMax)*(value.GenerationRawOutput.P95+value.GenerationNormalizedOutput.P95) + int64(result.SemanticMax)*(value.SemanticRawOutput.P95+value.SemanticNormalizedOutput.P95)
	result.MaxProjectedBytes = value.RunBSONWithoutOutputPayloads.Max + int64(result.GenerationMax)*(value.GenerationRawOutput.Max+value.GenerationNormalizedOutput.Max) + int64(result.SemanticMax)*(value.SemanticRawOutput.Max+value.SemanticNormalizedOutput.Max)
	result.P95HeadroomBytes = mongoMaxBSONBytes - result.P95ProjectedBytes
	result.MaxHeadroomBytes = mongoMaxBSONBytes - result.MaxProjectedBytes
	result.P95WithinMongoLimit = result.P95ProjectedBytes < mongoMaxBSONBytes
	result.MaxWithinMongoLimit = result.MaxProjectedBytes < mongoMaxBSONBytes
	return result
}

func printHuman(value report) {
	fmt.Printf("observed_at: %s\n", value.ObservedAt.Format(time.RFC3339))
	fmt.Printf("collection: %s\n", value.Collection)
	fmt.Printf("runs: matched=%d scanned=%d truncated=%t\n", value.MatchedRuns, value.ScannedRuns, value.Truncated)
	printDistribution("run_bson", value.RunBSON)
	printDistribution("run_bson_without_stored_output_payloads", value.RunBSONWithoutOutputPayloads)
	printDistribution("generation_raw_output", value.GenerationRawOutput)
	printDistribution("generation_normalized_output", value.GenerationNormalizedOutput)
	printDistribution("semantic_raw_output", value.SemanticRawOutput)
	printDistribution("semantic_normalized_output", value.SemanticNormalizedOutput)
	projection := value.V2ObservedOutputProjection
	if !projection.Available {
		fmt.Printf("v2_projection: unavailable (%s)\n", projection.Reason)
		return
	}
	fmt.Printf("v2_projection: p95=%d max=%d mongo_max=%d p95_headroom=%d max_headroom=%d\n",
		projection.P95ProjectedBytes, projection.MaxProjectedBytes, projection.MongoMaxBSONBytes,
		projection.P95HeadroomBytes, projection.MaxHeadroomBytes)
}

func printDistribution(name string, value sizeDistribution) {
	fmt.Printf("%s: observed=%d missing=%d p50=%d p95=%d max=%d\n", name, value.Observed, value.Missing, value.P50, value.P95, value.Max)
}
