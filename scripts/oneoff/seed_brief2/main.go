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
	publishedRepo := mongomodelcatalog.NewPublishedModelRepoAdapter(publishedRepository)
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
		definition.Conclusions = append(definition.Conclusions, brief2Conclusion(code, index == 12))
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

func brief2Conclusion(factorCode string, primary bool) conclusion.NormConclusion {
	return conclusion.NormConclusion{
		FactorCode: factorCode,
		ScoreBasis: conclusion.ScoreBasisTScore,
		Primary:    primary,
		Rules: []conclusion.ScoreRangeOutcome{
			{MinScore: 0, MaxScore: 59, Level: "normal", Title: "与同龄儿童相似"},
			{MinScore: 60, MaxScore: 64, Level: "mild", Title: "轻微执行功能障碍"},
			{MinScore: 65, MaxScore: 69, Level: "moderate", Title: "中度执行功能障碍"},
			{MinScore: 70, MaxScore: 200, Level: "severe", Title: "严重执行功能障碍"},
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
