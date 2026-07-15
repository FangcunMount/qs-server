// seed_brief2 imports the legacy BRIEF-2 parent-form norm source into the
// canonical behavioral_rating/brief2 ModelCatalog shape. It never invents the
// item-to-scale mapping: operators must export that mapping from the legacy
// assessment mode before applying the migration.
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
	"sort"
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
	defaultModelCode            = "gXkk9W"
	defaultQuestionnaire        = "gXkk9W"
	defaultQuestionnaireVersion = "4.0.1"
	defaultNormVersion          = "brief2-parent-cn-legacy-gXkk9W-v1"
	defaultFormVariant          = "parent"
	brief2NormFactorCount       = 13
)

//go:embed data/brief2-parent-cn-legacy-gXkk9W-v1.json.gz.b64
var embeddedBrief2NormSource []byte

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
		fmt.Fprintln(os.Stderr, "seed brief2 failed:", err)
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
	flag.StringVar(&cfg.normSource, "norm-source", "", "optional path to a normalized BRIEF-2 norm JSON source (default: embedded versioned data)")
	flag.StringVar(&cfg.factorMap, "factor-map", "", "JSON file mapping legacy factor codes to questionnaire question codes")
	flag.StringVar(&cfg.normVersion, "norm-version", defaultNormVersion, "immutable norm table version")
	flag.StringVar(&cfg.formVariant, "form-variant", defaultFormVariant, "BRIEF-2 form variant")
	flag.BoolVar(&cfg.apply, "apply", false, "write norm, draft and published model (default: validate only)")
	flag.BoolVar(&cfg.force, "force", false, "replace the existing draft and published model; norm content must still be identical")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	if cfg.factorMap == "" {
		return errors.New("--factor-map is required; the supplied files contain no item-to-scale mapping")
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
	normTable, catalog, err := buildNormTable(source, cfg.normVersion, cfg.formVariant)
	if err != nil {
		return err
	}

	fmt.Printf("plan: model=%s questionnaire=%s@%s norm=%s factors=%d strata=%d mapped_questions=%d excluded_questions=%d\n",
		cfg.modelCode, cfg.questionnaireCode, cfg.questionnaireVer, normTable.TableVersion, len(normTable.Factors), len(source.Scores), mapping.mappedQuestionCount(), mapping.excludedQuestionCount())
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
	if questionnaire.GetVersion().Value() != cfg.questionnaireVer {
		return fmt.Errorf("questionnaire %s active version is %s, want %s", cfg.questionnaireCode, questionnaire.GetVersion().Value(), cfg.questionnaireVer)
	}
	definition, err := buildDefinition(questionnaire, mapping, catalog, normTable.TableVersion, cfg.formVariant)
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
		if err := normRepo.UpsertNorm(txCtx, normTable); err != nil {
			return fmt.Errorf("upsert norm %s: %w", normTable.TableVersion, err)
		}
		return seedModel(txCtx, db, cfg, questionnaire.GetVersion().Value(), definition, definitionJSON, normRepo)
	}); err != nil {
		return err
	}
	fmt.Printf("seeded BRIEF-2 model %s -> questionnaire %s@%s, norm=%s\n", cfg.modelCode, cfg.questionnaireCode, questionnaire.GetVersion().Value(), normTable.TableVersion)
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
		Algorithm:      domain.AlgorithmBrief2,
		ProductChannel: domain.ProductChannelBehaviorAbility,
		Title:          "BRIEF-2 执行功能行为评定量表（家长版）",
		Description:    "BRIEF-2 家长版，9 个临床分量表、3 个指数及总执行功能指数。",
		Category:       "behavior_ability",
		ApplicableAges: []string{"5-13"},
		Reporters:      []string{"parent"},
		Now:            now,
	})
	if err != nil {
		return fmt.Errorf("new BRIEF-2 model: %w", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: cfg.questionnaireCode, QuestionnaireVersion: questionnaireVersion}, now); err != nil {
		return fmt.Errorf("bind questionnaire: %w", err)
	}
	if err := model.UpdateDefinitionWithV2(domain.DefinitionPayload{Format: domain.PayloadFormatBehavioralRatingDefaultV1, Data: definitionJSON}, definition, now); err != nil {
		return fmt.Errorf("save DefinitionV2: %w", err)
	}
	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo}
	if issues := handler.ValidateForPublish(ctx, model); len(issues) > 0 {
		return fmt.Errorf("generated BRIEF-2 model is not publishable: %s", issues[0].Message)
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
	Factors map[string]string    `json:"factors"`
	Briefs  map[string]normBrief `json:"briefs"`
	Scores  []normScoreStratum   `json:"scores"`
}

type normBrief struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

type normScoreStratum struct {
	Ages   []int             `json:"ages"`
	Sex    int               `json:"sex"`
	Scores map[string]string `json:"scores"`
}

func loadNormSource(path string) (normSource, error) {
	output, err := readNormSource(path, embeddedBrief2NormSource)
	if err != nil {
		return normSource{}, err
	}
	var source normSource
	if err := json.Unmarshal(output, &source); err != nil {
		return normSource{}, fmt.Errorf("decode BRIEF-2 norm JSON source: %w", err)
	}
	if len(source.Factors) != brief2NormFactorCount || len(source.Scores) == 0 {
		return normSource{}, fmt.Errorf("unexpected BRIEF-2 source: factors=%d strata=%d", len(source.Factors), len(source.Scores))
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
	QuestionnaireCode    string              `json:"questionnaire_code"`
	QuestionnaireVersion string              `json:"questionnaire_version"`
	Factors              factorMap           `json:"factors"`
	ExcludedQuestions    map[string][]string `json:"excluded_question_codes"`
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

func (m factorMapping) excludedQuestionCount() int {
	count := 0
	for _, codes := range m.ExcludedQuestions {
		count += len(codes)
	}
	return count
}

func buildNormTable(source normSource, version, formVariant string) (*modelnorm.Norm, normCatalog, error) {
	catalog, err := newNormCatalog(source)
	if err != nil {
		return nil, normCatalog{}, err
	}
	table := &modelnorm.Norm{TableVersion: version, FormVariant: formVariant, Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2}
	for _, factorCode := range catalog.order {
		table.Factors = append(table.Factors, modelnorm.FactorTable{FactorCode: factorCode})
	}
	byCode := make(map[string]*modelnorm.FactorTable, len(table.Factors))
	for i := range table.Factors {
		byCode[table.Factors[i].FactorCode] = &table.Factors[i]
	}
	for index, stratum := range source.Scores {
		minAge, maxAge, gender, err := stratumScope(stratum)
		if err != nil {
			return nil, normCatalog{}, fmt.Errorf("stratum %d: %w", index, err)
		}
		for group, rawRows := range stratum.Scores {
			rows, err := decodeNormRows(rawRows)
			if err != nil {
				return nil, normCatalog{}, fmt.Errorf("stratum %d group %s: %w", index, group, err)
			}
			for _, row := range rows {
				rawScore, err := parseNumber(row["RawScore"])
				if err != nil {
					return nil, normCatalog{}, fmt.Errorf("stratum %d group %s raw score: %w", index, group, err)
				}
				for normName, factorCode := range catalog.byNormName {
					tScoreText, percentileText := row[normName+"_T"], row[normName+"_%ile"]
					if tScoreText == "" || percentileText == "" {
						continue
					}
					tScore, err := parseNumber(tScoreText)
					if err != nil {
						return nil, normCatalog{}, fmt.Errorf("%s T score: %w", normName, err)
					}
					percentile, err := parseNumber(percentileText)
					if err != nil {
						return nil, normCatalog{}, fmt.Errorf("%s percentile: %w", normName, err)
					}
					item := byCode[factorCode]
					item.Lookup = append(item.Lookup, modelnorm.LookupEntry{RawScoreMin: rawScore, RawScoreMax: rawScore, MinAgeMonths: minAge, MaxAgeMonths: maxAge, Gender: gender, TScore: tScore, Percentile: percentile})
				}
			}
		}
	}
	for _, factor := range table.Factors {
		if len(factor.Lookup) == 0 {
			return nil, normCatalog{}, fmt.Errorf("norm source does not contain lookup rows for factor %s", factor.FactorCode)
		}
	}
	return table, catalog, nil
}

type normCatalog struct {
	byNormName map[string]string
	titles     map[string]string
	order      []string
}

func newNormCatalog(source normSource) (normCatalog, error) {
	byNormName := make(map[string]string, len(source.Factors))
	titles := make(map[string]string, len(source.Factors))
	for factorCode, normName := range source.Factors {
		if factorCode == "" || normName == "" {
			return normCatalog{}, errors.New("factor/norm relation contains an empty code")
		}
		if _, exists := byNormName[normName]; exists {
			return normCatalog{}, fmt.Errorf("duplicate norm name %s", normName)
		}
		byNormName[normName] = factorCode
		titles[factorCode] = source.Briefs[normName].Title
	}
	order := make([]string, 0, len(source.Factors))
	for _, normName := range []string{"Inhibit", "Self-Monitor", "Shift", "Emotional-Control", "Initate", "Working-Memory", "Plan-Organize", "Task-Monitor", "Orgainization-of-Materials", "BRI", "ERI", "CRI", "GEC"} {
		factorCode, ok := byNormName[normName]
		if !ok {
			return normCatalog{}, fmt.Errorf("norm source missing %s", normName)
		}
		order = append(order, factorCode)
	}
	return normCatalog{byNormName: byNormName, titles: titles, order: order}, nil
}

func stratumScope(value normScoreStratum) (int, int, string, error) {
	if len(value.Ages) == 0 {
		return 0, 0, "", errors.New("ages is required")
	}
	ages := append([]int(nil), value.Ages...)
	sort.Ints(ages)
	if ages[0] <= 0 || ages[len(ages)-1]-ages[0] != len(ages)-1 {
		return 0, 0, "", fmt.Errorf("ages %v must be a contiguous positive range", value.Ages)
	}
	gender := ""
	switch value.Sex {
	case 1:
		gender = "male"
	case 2:
		gender = "female"
	default:
		return 0, 0, "", fmt.Errorf("unsupported sex %d", value.Sex)
	}
	return ages[0] * 12, ages[len(ages)-1]*12 + 11, gender, nil
}

func decodeNormRows(raw string) ([]map[string]string, error) {
	var rows []map[string]string
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("empty score rows")
	}
	return rows, nil
}

func parseNumber(value string) (float64, error) {
	parsed := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(value), "><="))
	if parsed == "" {
		return 0, errors.New("empty number")
	}
	return strconv.ParseFloat(parsed, 64)
}

func buildDefinition(questionnaire *surveyquestionnaire.Questionnaire, mapping factorMapping, catalog normCatalog, normVersion, formVariant string) (*modeldefinition.Definition, error) {
	if questionnaire == nil {
		return nil, errors.New("questionnaire is nil")
	}
	if len(catalog.order) != brief2NormFactorCount {
		return nil, fmt.Errorf("BRIEF-2 factor catalog has %d factors, want %d", len(catalog.order), brief2NormFactorCount)
	}
	questionScores := make(map[string]map[string]float64)
	for _, question := range questionnaire.GetQuestions() {
		if len(question.GetOptions()) == 0 {
			continue
		}
		optionScores := make(map[string]float64, len(question.GetOptions()))
		for _, option := range question.GetOptions() {
			optionScores[option.GetCode().Value()] = option.GetScore()
		}
		questionScores[question.GetCode().Value()] = optionScores
	}
	if len(questionScores) == 0 {
		return nil, fmt.Errorf("questionnaire %s has no scored questions", questionnaire.GetCode().Value())
	}

	leafCodes := catalog.order[:9]
	factorMap := mapping.Factors
	leafSet := make(map[string]struct{}, len(leafCodes))
	for _, code := range leafCodes {
		leafSet[code] = struct{}{}
		if len(factorMap[code]) == 0 {
			return nil, fmt.Errorf("factor map is missing leaf factor %s", code)
		}
	}
	for code := range factorMap {
		if _, ok := leafSet[code]; !ok {
			return nil, fmt.Errorf("factor map contains unsupported factor %s; only the nine clinical-scale codes are allowed", code)
		}
	}

	seenQuestions := make(map[string]string, len(questionScores))
	excludedQuestions := make(map[string]string, mapping.excludedQuestionCount())
	for reason, questionCodes := range mapping.ExcludedQuestions {
		if reason == "" || len(questionCodes) == 0 {
			return nil, errors.New("excluded question groups require a non-empty reason and at least one question code")
		}
		for _, questionCode := range questionCodes {
			if _, ok := questionScores[questionCode]; !ok {
				return nil, fmt.Errorf("excluded question %s (%s) is missing or unscored", questionCode, reason)
			}
			if previous, duplicate := excludedQuestions[questionCode]; duplicate {
				return nil, fmt.Errorf("question %s is excluded by both %s and %s", questionCode, previous, reason)
			}
			excludedQuestions[questionCode] = reason
		}
	}
	definition := &modeldefinition.Definition{}
	for index, code := range catalog.order {
		role := factor.FactorRoleDimension
		switch index {
		case 9, 10, 11:
			role = factor.FactorRoleIndex
		case 12:
			role = factor.FactorRoleTotal
		}
		definition.Measure.Factors = append(definition.Measure.Factors, factor.Factor{Code: code, Title: catalog.titles[code], Role: role})
		definition.Calibration.NormRefs = append(definition.Calibration.NormRefs, modelnorm.Ref{FactorCode: code, NormTableVersion: normVersion})
		definition.Conclusions = append(definition.Conclusions, brief2Conclusion(code, catalog.titles[code], index == 12))
	}
	for _, code := range leafCodes {
		sources := make([]factor.ScoringSource, 0, len(factorMap[code]))
		for _, questionCode := range factorMap[code] {
			if reason, excluded := excludedQuestions[questionCode]; excluded {
				return nil, fmt.Errorf("factor %s references excluded question %s (%s)", code, questionCode, reason)
			}
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
		if _, ok := seenQuestions[questionCode]; ok {
			continue
		}
		if _, ok := excludedQuestions[questionCode]; !ok {
			return nil, fmt.Errorf("questionnaire question %s is not assigned to a BRIEF-2 clinical scale", questionCode)
		}
	}

	indexSources := [][]string{
		{leafCodes[0], leafCodes[1]},
		{leafCodes[2], leafCodes[3]},
		{leafCodes[4], leafCodes[5], leafCodes[6], leafCodes[7], leafCodes[8]},
		{catalog.order[9], catalog.order[10], catalog.order[11]},
	}
	for offset, children := range indexSources {
		factorCode := catalog.order[9+offset]
		sources := make([]factor.ScoringSource, 0, len(children))
		for _, child := range children {
			sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: child})
			definition.Measure.FactorGraph.Edges = append(definition.Measure.FactorGraph.Edges, factor.FactorEdge{ParentCode: factorCode, ChildCode: child})
		}
		definition.Measure.Scoring = append(definition.Measure.Scoring, factor.Scoring{FactorCode: factorCode, Sources: sources, Strategy: factor.ScoringStrategySum})
	}
	definition.Measure.FactorGraph.Roots = []string{catalog.order[12]}
	definition.Measure.FactorGraph.SortOrders = make(map[string]int, len(catalog.order))
	for index, code := range catalog.order {
		definition.Measure.FactorGraph.SortOrders[code] = index + 1
	}
	definition.Execution.Brief2 = &modeldefinition.Brief2Spec{
		FormVariant:       formVariant,
		PrimaryFactorCode: catalog.order[12],
		IndexFactorCodes:  append([]string(nil), catalog.order[9:12]...),
	}
	definition.ReportMap = modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{
		Code:       "brief2_scores",
		Title:      "BRIEF-2 分量表与指数",
		Kind:       modeldefinition.ReportSectionKindFactorScores,
		SourceRefs: append([]string(nil), catalog.order...),
	}}}
	if issues := modeldefinition.Validate(*definition); len(issues) > 0 {
		return nil, fmt.Errorf("generated DefinitionV2 is invalid: %s", issues[0].Message)
	}
	return definition, nil
}

// brief2ReportProfile keeps the explanatory copy with the BRIEF-2 definition
// rather than relying on the client to infer meaning from a T score alone. The
// result is a screening interpretation, not a clinical diagnosis.
type brief2ReportProfile struct {
	aspect   string
	context  string
	strategy string
	impact   string
}

var brief2ReportProfiles = map[string]brief2ReportProfile{
	"抑制": {
		aspect:   "抑制冲动、等待以及停止不合适行为",
		context:  "兴奋、等待、被打断或规则要求较多时",
		strategy: "用简短而一致的规则、视觉提示和“先停一停再行动”的练习，并及时肯定能够等待或自我停止的行为",
		impact:   "遵守规则、课堂秩序、同伴互动或安全行为",
	},
	"自我监控": {
		aspect:   "觉察自己的言行对他人和事情结果的影响",
		context:  "与同伴互动、完成后复查或需要调整行为方式时",
		strategy: "在具体情境后共同回顾“我做了什么、别人有什么感受、下次可以怎样做”，并一次只练习一个可观察的目标",
		impact:   "人际互动、遵守约定和从经验中调整行为",
	},
	"情景转换": {
		aspect:   "适应变化、在活动之间切换以及转换解决问题的方法",
		context:  "计划临时改变、结束喜欢的活动或任务要求转换时",
		strategy: "提前预告变化，使用可视化日程和倒计时；从小幅度、可预期的转换开始练习，并为替代方案提供选择",
		impact:   "日常过渡、课堂转换和面对变化时的合作",
	},
	"情绪控制": {
		aspect:   "识别、表达和调节情绪反应",
		context:  "受挫、被拒绝、疲劳或要求较高时",
		strategy: "在情绪平稳时练习给情绪命名、识别身体信号和使用冷静步骤；成人先共情并示范，再讨论可行的解决办法",
		impact:   "家庭互动、同伴关系以及完成日常要求的能力",
	},
	"任务启动": {
		aspect:   "主动开始任务，并产生完成任务的想法或步骤",
		context:  "面对不熟悉、步骤较多或缺少即时兴趣的任务时",
		strategy: "把任务拆成清晰的第一步，配合开始提示、计时器和完成后的即时反馈；逐步减少成人代劳",
		impact:   "作业、晨间准备和独立完成日常任务",
	},
	"工作记忆": {
		aspect:   "暂时保留信息，并按步骤完成任务",
		context:  "听取多步指令、心算、抄写或同时处理多项信息时",
		strategy: "把口头要求分成一至两步，配合清单、图示或复述确认；复杂任务可提供外部记录，完成一段再进入下一段",
		impact:   "学习效率、遵循指令和多步骤活动的完成质量",
	},
	"计划/组织": {
		aspect:   "预估任务要求、安排步骤并整理信息",
		context:  "长期作业、需要准备材料或需要自己安排时间时",
		strategy: "共同使用“目标—步骤—所需材料—完成时间”清单；先示范如何把大任务拆小，再逐渐让孩子承担计划和检查",
		impact:   "学习安排、问题解决和按时完成任务",
	},
	"任务监控": {
		aspect:   "在任务过程中检查进度、发现并修正错误",
		context:  "书面作业、需要持续注意或任务接近结束时",
		strategy: "设置中途检查点和固定的复查顺序，例如“看要求、做任务、对答案”；反馈具体指出已发现和修正的部分",
		impact:   "作业准确性、做事的完整度和独立性",
	},
	"材料组织": {
		aspect:   "整理、保管和及时找到个人物品",
		context:  "上学准备、收拾书包或在多个活动地点切换时",
		strategy: "为常用物品设置固定位置和标签，使用出门前/结束后的简短清单，并安排固定的整理时间而非临时催促",
		impact:   "上学准备、物品管理和日常生活效率",
	},
	"行为调节": {
		aspect:   "控制行为反应并觉察自身行为影响的整体能力",
		context:  "需要遵守规则、等待、与人协作或受挫时",
		strategy: "优先在家庭和学校统一少量关键规则，明确期望行为和即时反馈；将“暂停—想一想—再行动”作为共同练习流程",
		impact:   "课堂适应、亲子互动、同伴关系和安全行为",
	},
	"情绪调节": {
		aspect:   "适应变化并调节情绪反应的整体能力",
		context:  "计划改变、挫折、冲突或情绪被激发时",
		strategy: "提前预告变化，建立可重复使用的冷静流程，并在平稳时练习替代反应；成人保持一致、简洁的回应",
		impact:   "过渡情境、冲突处理和参与日常活动的稳定性",
	},
	"认知调节": {
		aspect:   "启动任务、保持信息、计划组织并监控完成过程的整体能力",
		context:  "学习任务、日常准备和需要独立完成多步骤活动时",
		strategy: "优先减少一次性信息量，使用外部清单和固定流程；家长与教师可共同选择一至两个最影响功能的目标持续跟进",
		impact:   "学习效率、时间管理和日常独立性",
	},
	"总分": {
		aspect:   "日常执行功能表现的整体水平",
		context:  "家庭、学校和社交等不同环境的日常要求中",
		strategy: "结合得分较高的分量表确定优先目标，在家庭和学校使用一致、可执行的支持方式，并定期根据实际变化调整",
		impact:   "学习、生活自理、情绪行为和人际适应的整体功能",
	},
}

func brief2Conclusion(factorCode, title string, primary bool) conclusion.NormConclusion {
	profile, ok := brief2ReportProfiles[title]
	if !ok {
		profile = brief2ReportProfile{
			aspect:   title + "相关的日常执行功能表现",
			context:  "任务要求增加、环境变化或疲劳时",
			strategy: "结合具体情境设置清晰、可观察的小目标，并给予一致的提示和反馈",
			impact:   "日常学习、生活和人际适应",
		}
	}
	return conclusion.NormConclusion{
		FactorCode: factorCode,
		ScoreBasis: conclusion.ScoreBasisTScore,
		Primary:    primary,
		Rules: []conclusion.ScoreRangeOutcome{
			{
				MinScore: 0, MaxScore: 59, Level: "normal", Title: "与同龄儿童相似",
				Summary:     fmt.Sprintf("在%s方面的表现与同龄儿童相近；本次问卷未提示明显困难。", profile.aspect),
				Description: fmt.Sprintf("可继续提供清晰、稳定的日常规则和作息，并在%s等压力较大的情境中留意表现变化。若困难只偶尔出现，宜结合睡眠、任务难度和环境变化综合观察。", profile.context),
			},
			{
				MinScore: 60, MaxScore: 64, Level: "mild", Title: "轻微执行功能障碍",
				Summary:     fmt.Sprintf("在%s方面可能偶有困难，通常在%s时更容易表现出来。", profile.aspect, profile.context),
				Description: fmt.Sprintf("建议先从一个高频场景开始：%s。连续观察一段时间，记录哪些提示有效，并避免把偶发困难直接等同于能力不足。", profile.strategy),
			},
			{
				MinScore: 65, MaxScore: 69, Level: "moderate", Title: "中度执行功能障碍",
				Summary:     fmt.Sprintf("问卷提示%s方面的困难较为明显，可能已影响%s。", profile.aspect, profile.impact),
				Description: fmt.Sprintf("建议家长与教师共同确定一至两个可观察的目标，并在多个场景使用一致支持：%s。定期根据实际完成情况调整目标和支持强度。", profile.strategy),
			},
			{
				MinScore: 70, MaxScore: 200, Level: "severe", Title: "严重执行功能障碍",
				Summary:     fmt.Sprintf("问卷提示%s方面存在显著困难，可能持续影响%s。", profile.aspect, profile.impact),
				Description: fmt.Sprintf("除实施日常支持外，建议尽快与学校或照护团队沟通，形成具体支持计划：%s。若困难已持续影响学习、家庭互动、同伴关系或生活自理，可携带本报告向儿童发育行为、心理或康复等专业人员咨询；本结果用于筛查和支持规划，不能单独作为诊断依据。", profile.strategy),
			},
		},
	}
}

func cloneScores(values map[string]float64) map[string]float64 {
	cloned := make(map[string]float64, len(values))
	for code, score := range values {
		cloned[code] = score
	}
	return cloned
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
