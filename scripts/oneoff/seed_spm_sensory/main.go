// seed_spm_sensory imports Sensory Processing Measure (SPM) into the
// behavioral-rating norm workflow. It is intentionally separate from the
// cognitive/raven `spm` algorithm.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	surveyquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/scripts/oneoff/internal/modelseed"
)

const (
	defaultModelCode              = "bJFKi3"
	defaultQuestionnaire          = "bJFKi3"
	defaultQuestionnaireVersion   = "4.0.1"
	defaultNormVersion            = "spm-sensory-cn-legacy-bJFKi3-v1"
	defaultFormVariant            = "home"
	tasteSmellFactorCode          = "wcgKM7uV"
	tasteSmellFactorTitle         = "味觉与嗅觉（仅计入 TOT）"
	spmReverseBalanceQuestionCode = "jenu1Rox"
)

//go:embed data/spm-sensory-cn-legacy-bJFKi3-v1.json.gz.b64
var embeddedSPMNormSource []byte

var normOrder = []string{"SOC", "VIS", "HEA", "TOU", "BOD", "BAL", "PLA", "TOT"}
var expectedQuestionCounts = map[string]int{"SOC": 10, "VIS": 11, "HEA": 8, "TOU": 11, "BOD": 10, "BAL": 11, "PLA": 9}

type config struct {
	mongoURI          string
	mongoDB           string
	modelCode         string
	questionnaireCode string
	questionnaireVer  string
	normSource        string
	factorMap         string
	normVersion       string
	formVariant       string
	apply             bool
	force             bool
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "seed SPM sensory failed:", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.StringVar(&cfg.modelCode, "model-code", defaultModelCode, "assessment model code")
	flag.StringVar(&cfg.questionnaireCode, "questionnaire-code", defaultQuestionnaire, "existing published questionnaire code")
	flag.StringVar(&cfg.questionnaireVer, "questionnaire-version", defaultQuestionnaireVersion, "required published questionnaire version")
	flag.StringVar(&cfg.normSource, "norm-source", "", "optional path to a normalized SPM norm JSON source (default: embedded versioned data)")
	flag.StringVar(&cfg.factorMap, "factor-map", "", "JSON map from the seven SPM factor codes to questionnaire question codes")
	flag.StringVar(&cfg.normVersion, "norm-version", defaultNormVersion, "immutable norm table version")
	flag.StringVar(&cfg.formVariant, "form-variant", defaultFormVariant, "SPM form variant")
	flag.BoolVar(&cfg.apply, "apply", false, "write norm, draft and published model (default: validate only)")
	flag.BoolVar(&cfg.force, "force", false, "replace existing draft and published model; norm content must remain identical")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	if cfg.factorMap == "" {
		return errors.New("--factor-map is required; provide the seven clinical factors plus the taste/smell item group for bJFKi3")
	}
	if cfg.modelCode == "" || cfg.questionnaireCode == "" || cfg.questionnaireVer == "" || cfg.normVersion == "" || cfg.formVariant == "" {
		return errors.New("model-code, questionnaire-code, questionnaire-version, norm-version and form-variant are required")
	}
	source, err := loadNormSource(cfg.normSource)
	if err != nil {
		return err
	}
	mapping, err := loadFactorMap(cfg.factorMap)
	if err != nil {
		return err
	}
	if err := mapping.validateTarget(cfg.questionnaireCode, cfg.questionnaireVer); err != nil {
		return err
	}
	table, catalog, percentileFallbacks, err := buildNormTable(source, cfg.normVersion, cfg.formVariant)
	if err != nil {
		return err
	}
	fmt.Printf("plan: model=%s questionnaire=%s@%s norm=%s factors=%d lookups=%d percentile_fallbacks=%d mapped_questions=%d\n", cfg.modelCode, cfg.questionnaireCode, cfg.questionnaireVer, table.TableVersion, len(table.Factors), lookupCount(table), percentileFallbacks, mapping.mappedQuestionCount())
	if !cfg.apply {
		fmt.Println("dry-run validated source files; Mongo questionnaire and existing model will be checked with --apply")
		return nil
	}
	if cfg.mongoURI == "" {
		return errors.New("--mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("ping mongo: %w", err)
	}
	db := client.Database(cfg.mongoDB)
	questionnaireRepo := mongoquestionnaire.NewRepository(db)
	questionnaire, err := questionnaireRepo.FindPublishedByCode(ctx, cfg.questionnaireCode)
	if err != nil {
		return fmt.Errorf("load published questionnaire %s: %w", cfg.questionnaireCode, err)
	}
	if questionnaire == nil {
		return fmt.Errorf("published questionnaire %s not found", cfg.questionnaireCode)
	}
	if cfg.questionnaireVer != questionnaire.GetVersion().Value() {
		return fmt.Errorf("questionnaire %s active version is %s, want %s", cfg.questionnaireCode, questionnaire.GetVersion().Value(), cfg.questionnaireVer)
	}
	definition, err := buildDefinition(questionnaire, mapping.Factors, catalog, table.TableVersion)
	if err != nil {
		return err
	}
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		return fmt.Errorf("marshal DefinitionV2: %w", err)
	}
	normRepo := mongomodelcatalog.NewNormRepository(db)
	runner := modelseed.NewMongoTransactionRunner(client)
	if err := modelseed.RunAtomically(ctx, runner, func(txCtx context.Context) error {
		if err := normRepo.UpsertNorm(txCtx, table); err != nil {
			return fmt.Errorf("upsert norm %s: %w", table.TableVersion, err)
		}
		return seedModel(txCtx, db, cfg, questionnaire.GetVersion().Value(), definition, definitionJSON, normRepo)
	}); err != nil {
		return err
	}
	fmt.Printf("seeded SPM sensory model %s -> questionnaire %s@%s, norm=%s\n", cfg.modelCode, cfg.questionnaireCode, questionnaire.GetVersion().Value(), table.TableVersion)
	return nil
}

func seedModel(ctx context.Context, db *mongo.Database, cfg config, questionnaireVersion string, definition *modeldefinition.Definition, definitionJSON []byte, normRepo *mongomodelcatalog.NormRepository) error {
	draftRepo := mongomodelcatalog.NewDraftRepository(db)
	publishedRepository := mongomodelcatalog.NewRepository(db)
	publishedRepo := publishedRepository
	existing, err := draftRepo.FindByCode(ctx, cfg.modelCode)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("find draft %s: %w", cfg.modelCode, err)
	}
	state, err := modelseed.InspectActivePublished(ctx, publishedRepository.Collection(), cfg.modelCode, cfg.questionnaireCode, questionnaireVersion)
	if err != nil {
		return err
	}
	if err := state.ValidateReplacement(cfg.force, existing != nil, cfg.modelCode, cfg.questionnaireCode, questionnaireVersion); err != nil {
		return err
	}

	now := time.Now().UTC()
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           cfg.modelCode,
		Kind:           domain.KindBehavioralRating,
		SubKind:        domain.SubKindEmpty,
		Algorithm:      domain.AlgorithmSPMSensory,
		ProductChannel: domain.ProductChannelBehaviorAbility,
		Title:          "SPM 感觉统合量表",
		Description:    "Sensory Processing Measure，评估社会参与与感觉处理功能。",
		Category:       "behavior_ability",
		Reporters:      []string{"parent"},
		Now:            now,
	})
	if err != nil {
		return fmt.Errorf("new SPM sensory model: %w", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: cfg.questionnaireCode, QuestionnaireVersion: questionnaireVersion}, now); err != nil {
		return fmt.Errorf("bind questionnaire: %w", err)
	}
	if err := model.UpdateDefinitionWithV2(domain.DefinitionPayload{Format: domain.PayloadFormatBehavioralRatingDefaultV1, Data: definitionJSON}, definition, now); err != nil {
		return fmt.Errorf("save DefinitionV2: %w", err)
	}
	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo}
	if issues := handler.ValidateForPublish(ctx, model); len(issues) > 0 {
		return fmt.Errorf("generated SPM sensory model is not publishable: %s", issues[0].Message)
	}
	publisher := publication.Publisher{Registry: appdefinition.NewRegistry(handler)}
	if err := model.MarkPublished(now); err != nil {
		return fmt.Errorf("mark model published: %w", err)
	}
	snapshot, err := publisher.BuildSnapshot(ctx, model)
	if err != nil {
		return fmt.Errorf("build published snapshot: %w", err)
	}
	if existing != nil {
		result, err := draftRepo.Collection().DeleteMany(ctx, bson.M{"code": cfg.modelCode})
		if err != nil {
			return fmt.Errorf("purge draft %s: %w", cfg.modelCode, err)
		}
		if result.DeletedCount == 0 {
			return fmt.Errorf("purge draft %s: deleted=0 after preflight found an active draft", cfg.modelCode)
		}
	}
	if cfg.force {
		if err := modelseed.RetireMatchingPublished(ctx, publishedRepository.Collection(), cfg.modelCode, cfg.questionnaireCode, questionnaireVersion, state.MatchingCount, now); err != nil {
			return err
		}
	}
	if err := publishedRepo.Save(ctx, snapshot); err != nil {
		return fmt.Errorf("save published snapshot: %w", err)
	}
	if err := draftRepo.Create(ctx, model); err != nil {
		return fmt.Errorf("create published draft: %w", err)
	}
	return nil
}

type normSource struct {
	Factors map[string]string `json:"factors"`
	Briefs  map[string]brief  `json:"briefs"`
	Scores  string            `json:"scores"`
}

type brief struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

type normRow map[string]string

func loadNormSource(path string) (normSource, error) {
	output, err := readNormSource(path, embeddedSPMNormSource)
	if err != nil {
		return normSource{}, err
	}
	var source normSource
	if err := json.Unmarshal(output, &source); err != nil {
		return normSource{}, fmt.Errorf("decode SPM norm JSON source: %w", err)
	}
	if len(source.Factors) != len(normOrder) || source.Scores == "" {
		return normSource{}, fmt.Errorf("unexpected SPM source: factors=%d scores=%t", len(source.Factors), source.Scores != "")
	}
	return source, nil
}

func readNormSource(path string, embedded []byte) ([]byte, error) {
	if path != "" {
		output, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read norm source: %w", err)
		}
		return output, nil
	}
	compressed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(embedded)))
	if err != nil {
		return nil, fmt.Errorf("decode embedded norm source: %w", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open embedded norm source: %w", err)
	}
	defer func() { _ = reader.Close() }()
	output, err := io.ReadAll(io.LimitReader(reader, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("decompress embedded norm source: %w", err)
	}
	return output, nil
}

type factorMap map[string][]string

type factorMapping struct {
	QuestionnaireCode    string    `json:"questionnaire_code"`
	QuestionnaireVersion string    `json:"questionnaire_version"`
	Factors              factorMap `json:"factors"`
}

func loadFactorMap(path string) (factorMapping, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return factorMapping{}, fmt.Errorf("read factor map: %w", err)
	}
	var result factorMapping
	if err := json.Unmarshal(raw, &result); err != nil {
		return factorMapping{}, fmt.Errorf("decode factor map: %w", err)
	}
	if len(result.Factors) == 0 {
		return factorMapping{}, errors.New("factor map is empty")
	}
	return result, nil
}

func (m factorMapping) validateTarget(code, version string) error {
	if m.QuestionnaireCode == "" || m.QuestionnaireVersion == "" {
		return errors.New("factor map must declare questionnaire_code and questionnaire_version")
	}
	if m.QuestionnaireCode != code || m.QuestionnaireVersion != version {
		return fmt.Errorf("factor map targets questionnaire %s@%s, want %s@%s", m.QuestionnaireCode, m.QuestionnaireVersion, code, version)
	}
	return nil
}

func (m factorMapping) mappedQuestionCount() int {
	count := 0
	for _, codes := range m.Factors {
		count += len(codes)
	}
	return count
}

type normCatalog struct {
	byNormName map[string]string
	titles     map[string]string
	order      []string
}

func buildNormTable(source normSource, version, formVariant string) (*modelnorm.Norm, normCatalog, int, error) {
	catalog, err := newNormCatalog(source)
	if err != nil {
		return nil, normCatalog{}, 0, err
	}
	var rows []normRow
	if err := json.Unmarshal([]byte(source.Scores), &rows); err != nil {
		return nil, normCatalog{}, 0, fmt.Errorf("decode SPM score rows: %w", err)
	}
	if len(rows) == 0 {
		return nil, normCatalog{}, 0, errors.New("SPM score rows are empty")
	}
	table := &modelnorm.Norm{TableVersion: version, FormVariant: formVariant, Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmSPMSensory}
	for _, code := range catalog.order {
		table.Factors = append(table.Factors, modelnorm.FactorTable{FactorCode: code})
	}
	byCode := make(map[string]*modelnorm.FactorTable, len(catalog.order))
	for index := range table.Factors {
		code := table.Factors[index].FactorCode
		byCode[code] = &table.Factors[index]
	}
	percentileFallbacks := 0
	for rowIndex, row := range rows {
		tScore, err := parseNumber(row["T"])
		if err != nil {
			return nil, normCatalog{}, 0, fmt.Errorf("row %d T score: %w", rowIndex, err)
		}
		percentile, fallback, err := parsePercentile(row["%ile"])
		if err != nil {
			return nil, normCatalog{}, 0, fmt.Errorf("row %d percentile: %w", rowIndex, err)
		}
		if fallback {
			percentileFallbacks++
		}
		for normName, factorCode := range catalog.byNormName {
			rawRange := strings.TrimSpace(row[normName])
			if rawRange == "" {
				continue
			}
			min, max, err := parseRange(rawRange)
			if err != nil {
				return nil, normCatalog{}, 0, fmt.Errorf("row %d %s raw range: %w", rowIndex, normName, err)
			}
			byCode[factorCode].Lookup = append(byCode[factorCode].Lookup, modelnorm.LookupEntry{RawScoreMin: min, RawScoreMax: max, TScore: tScore, Percentile: percentile})
		}
	}
	for _, factor := range table.Factors {
		if len(factor.Lookup) == 0 {
			return nil, normCatalog{}, 0, fmt.Errorf("SPM source does not contain lookup rows for factor %s", factor.FactorCode)
		}
	}
	return table, catalog, percentileFallbacks, nil
}

func newNormCatalog(source normSource) (normCatalog, error) {
	catalog := normCatalog{byNormName: make(map[string]string, len(source.Factors)), titles: make(map[string]string, len(source.Factors)), order: make([]string, 0, len(normOrder))}
	for factorCode, normName := range source.Factors {
		if factorCode == "" || normName == "" {
			return normCatalog{}, errors.New("factor/norm relation contains an empty code")
		}
		if _, exists := catalog.byNormName[normName]; exists {
			return normCatalog{}, fmt.Errorf("duplicate norm name %s", normName)
		}
		catalog.byNormName[normName] = factorCode
		catalog.titles[factorCode] = source.Briefs[normName].Title
	}
	for _, normName := range normOrder {
		factorCode, ok := catalog.byNormName[normName]
		if !ok {
			return normCatalog{}, fmt.Errorf("SPM source missing %s", normName)
		}
		catalog.order = append(catalog.order, factorCode)
	}
	return catalog, nil
}

func buildDefinition(questionnaire *surveyquestionnaire.Questionnaire, factorMap factorMap, catalog normCatalog, normVersion string) (*modeldefinition.Definition, error) {
	if questionnaire == nil {
		return nil, errors.New("questionnaire is nil")
	}
	leafCodes := catalog.order[:len(catalog.order)-1]
	scoringCodes := append(append([]string(nil), leafCodes...), tasteSmellFactorCode)
	reverseQuestions := make(map[string]struct{}, len(factorMap[catalog.byNormName["SOC"]])+1)
	for _, questionCode := range factorMap[catalog.byNormName["SOC"]] {
		reverseQuestions[questionCode] = struct{}{}
	}
	reverseQuestions[spmReverseBalanceQuestionCode] = struct{}{}
	questionScores := make(map[string]map[string]float64)
	for _, question := range questionnaire.GetQuestions() {
		if len(question.GetOptions()) == 0 {
			continue
		}
		questionCode := question.GetCode().Value()
		_, reverse := reverseQuestions[questionCode]
		if err := validateSPMOptionScores(questionCode, question.GetOptions(), reverse); err != nil {
			return nil, err
		}
		optionScores := make(map[string]float64, len(question.GetOptions()))
		for _, option := range question.GetOptions() {
			optionScores[option.GetCode().Value()] = option.GetScore()
		}
		questionScores[questionCode] = optionScores
	}
	if len(questionScores) == 0 {
		return nil, fmt.Errorf("questionnaire %s has no scored questions", questionnaire.GetCode().Value())
	}
	leafSet := make(map[string]struct{}, len(scoringCodes))
	for _, code := range leafCodes {
		leafSet[code] = struct{}{}
		if len(factorMap[code]) == 0 {
			return nil, fmt.Errorf("factor map is missing SPM factor %s", code)
		}
	}
	leafSet[tasteSmellFactorCode] = struct{}{}
	if len(factorMap[tasteSmellFactorCode]) == 0 {
		return nil, fmt.Errorf("factor map is missing SPM taste/smell item group %s", tasteSmellFactorCode)
	}
	for code := range factorMap {
		if _, ok := leafSet[code]; !ok {
			return nil, fmt.Errorf("factor map contains unsupported factor %s; only the seven non-total SPM factors and taste/smell item group are allowed", code)
		}
	}
	for normName, expected := range expectedQuestionCounts {
		factorCode := catalog.byNormName[normName]
		if got := len(factorMap[factorCode]); got != expected {
			return nil, fmt.Errorf("SPM factor %s (%s) has %d questions, want %d", normName, factorCode, got, expected)
		}
	}
	if got := len(factorMap[tasteSmellFactorCode]); got != 5 {
		return nil, fmt.Errorf("SPM taste/smell item group %s has %d questions, want 5", tasteSmellFactorCode, got)
	}

	definition := &modeldefinition.Definition{}
	for index, code := range catalog.order {
		role := factor.FactorRoleDimension
		if index == len(catalog.order)-1 {
			role = factor.FactorRoleTotal
		}
		definition.Measure.Factors = append(definition.Measure.Factors, factor.Factor{Code: code, Title: catalog.titles[code], Role: role})
		definition.Calibration.NormRefs = append(definition.Calibration.NormRefs, modelnorm.Ref{FactorCode: code, NormTableVersion: normVersion})
		definition.Conclusions = append(definition.Conclusions, spmConclusion(code, role == factor.FactorRoleTotal))
	}
	definition.Measure.Factors = append(definition.Measure.Factors, factor.Factor{Code: tasteSmellFactorCode, Title: tasteSmellFactorTitle, Role: factor.FactorRoleSubtest})
	seenQuestions := make(map[string]string, len(questionScores))
	for _, code := range scoringCodes {
		sources := make([]factor.ScoringSource, 0, len(factorMap[code]))
		for _, questionCode := range factorMap[code] {
			optionScores, ok := questionScores[questionCode]
			if !ok {
				return nil, fmt.Errorf("factor %s references missing or unscored questionnaire question %s", code, questionCode)
			}
			if owner, duplicate := seenQuestions[questionCode]; duplicate {
				return nil, fmt.Errorf("question %s is assigned to both %s and %s", questionCode, owner, code)
			}
			seenQuestions[questionCode] = code
			sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: questionCode, Sign: 1, OptionScores: cloneScores(optionScores)})
		}
		definition.Measure.Scoring = append(definition.Measure.Scoring, factor.Scoring{FactorCode: code, Sources: sources, Strategy: factor.ScoringStrategySum, OptionScoring: factor.OptionScoringStrict})
	}
	for questionCode := range questionScores {
		if _, ok := seenQuestions[questionCode]; !ok {
			return nil, fmt.Errorf("questionnaire question %s is not assigned to an SPM factor", questionCode)
		}
	}
	totalCode := catalog.byNormName["TOT"]
	totalComponentCodes := []string{
		catalog.byNormName["VIS"],
		catalog.byNormName["HEA"],
		catalog.byNormName["TOU"],
		tasteSmellFactorCode,
		catalog.byNormName["BOD"],
		catalog.byNormName["BAL"],
	}
	totalSources := make([]factor.ScoringSource, 0, len(totalComponentCodes))
	for _, childCode := range totalComponentCodes {
		if childCode == "" {
			return nil, errors.New("SPM total component is missing")
		}
		totalSources = append(totalSources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: childCode})
		definition.Measure.FactorGraph.Edges = append(definition.Measure.FactorGraph.Edges, factor.FactorEdge{ParentCode: totalCode, ChildCode: childCode})
	}
	definition.Measure.Scoring = append(definition.Measure.Scoring, factor.Scoring{FactorCode: totalCode, Sources: totalSources, Strategy: factor.ScoringStrategySum})
	definition.Measure.FactorGraph.Roots = []string{catalog.byNormName["SOC"], totalCode, catalog.byNormName["PLA"]}
	definition.Measure.FactorGraph.SortOrders = make(map[string]int, len(catalog.order)+1)
	for index, code := range catalog.order {
		definition.Measure.FactorGraph.SortOrders[code] = index + 1
	}
	definition.Measure.FactorGraph.SortOrders[tasteSmellFactorCode] = len(catalog.order) + 1
	definition.ReportMap = modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{Code: "spm_sensory_scores", Title: "SPM 感觉处理维度", Kind: modeldefinition.ReportSectionKindFactorScores, SourceRefs: append([]string(nil), catalog.order...)}}}
	if issues := modeldefinition.Validate(*definition); len(issues) > 0 {
		return nil, fmt.Errorf("generated DefinitionV2 is invalid: %s", issues[0].Message)
	}
	return definition, nil
}

func validateSPMOptionScores(questionCode string, options []surveyquestionnaire.Option, reverse bool) error {
	if len(options) != 4 {
		return fmt.Errorf("SPM question %s has %d options, want 4", questionCode, len(options))
	}
	normal := map[string]float64{"从不": 1, "偶尔": 2, "经常": 3, "常常": 3, "总是": 4}
	for _, option := range options {
		content := strings.TrimSpace(option.GetContent())
		expected, ok := normal[content]
		if !ok {
			return fmt.Errorf("SPM question %s has unsupported option %q", questionCode, content)
		}
		if reverse {
			expected = 5 - expected
		}
		if option.GetScore() != expected {
			direction := "正向"
			if reverse {
				direction = "反向"
			}
			return fmt.Errorf("SPM question %s option %q score is %.1f, want %.1f (%s计分)", questionCode, content, option.GetScore(), expected, direction)
		}
	}
	return nil
}

func spmConclusion(factorCode string, primary bool) conclusion.NormConclusion {
	return conclusion.NormConclusion{FactorCode: factorCode, ScoreBasis: conclusion.ScoreBasisTScore, Primary: primary, Rules: []conclusion.ScoreRangeOutcome{
		{MinScore: 0, MaxScore: 59, Level: "normal", Title: "与同龄儿童相似"},
		{MinScore: 60, MaxScore: 69, Level: "mild_moderate", Title: "轻度到中度困难"},
		{MinScore: 70, MaxScore: 200, Level: "severe", Title: "严重困难"},
	}}
}

func parseRange(raw string) (float64, float64, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	parts := strings.Split(cleaned, "-")
	if len(parts) == 1 {
		value, err := parseNumber(parts[0])
		return value, value, err
	}
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q", raw)
	}
	min, err := parseNumber(parts[0])
	if err != nil {
		return 0, 0, err
	}
	max, err := parseNumber(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if min > max {
		return 0, 0, fmt.Errorf("range min %.1f exceeds max %.1f", min, max)
	}
	return min, max, nil
}

func parsePercentile(raw string) (float64, bool, error) {
	if strings.TrimSpace(raw) == "" {
		// The supplied table omits values above its published percentile range.
		// Preserve its top-coded convention instead of inventing a new norm.
		return 99, true, nil
	}
	value, err := parseNumber(raw)
	return value, false, err
}

func parseNumber(raw string) (float64, error) {
	value := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "><="))
	if value == "" {
		return 0, errors.New("empty number")
	}
	return strconv.ParseFloat(value, 64)
}

func lookupCount(table *modelnorm.Norm) int {
	count := 0
	for _, factor := range table.Factors {
		count += len(factor.Lookup)
	}
	return count
}

func cloneScores(source map[string]float64) map[string]float64 {
	copy := make(map[string]float64, len(source))
	for code, score := range source {
		copy[code] = score
	}
	return copy
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
