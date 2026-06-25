package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

//go:embed sbti_questionnaire.json
var questionnaireSeedJSON []byte

const questionnaireCode = "SBTI_FUN"

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatalf("seed sbti questionnaire failed: %v", err)
	}
}

type config struct {
	mongoURI string
	mongoDB  string
	apply    bool
	force    bool
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.BoolVar(&cfg.apply, "apply", false, "write questionnaire to MongoDB (default dry-run)")
	flag.BoolVar(&cfg.force, "force", false, "replace existing published SBTI_FUN snapshot")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	seed, err := loadSeed()
	if err != nil {
		return err
	}
	questions, err := buildDomainQuestions(seed)
	if err != nil {
		return err
	}
	q, err := buildQuestionnaire(seed, questions)
	if err != nil {
		return err
	}
	validator := domainQuestionnaire.Validator{}
	if errs := validator.ValidateForPublish(q); len(errs) > 0 {
		return fmt.Errorf("questionnaire validation failed: %v", errs[0])
	}

	if cfg.mongoURI == "" {
		return fmt.Errorf("mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	repo := mongoQuestionnaire.NewRepository(client.Database(cfg.mongoDB))
	existing, err := repo.FindByCodeVersion(ctx, questionnaireCode, seed.Version)
	if err != nil {
		return fmt.Errorf("check existing questionnaire: %w", err)
	}
	if existing != nil && !cfg.force {
		fmt.Printf("dry-run: SBTI_FUN@%s already exists (%d questions), skip\n", seed.Version, existing.QuestionCount())
		if !cfg.apply {
			fmt.Println("dry-run complete (no changes)")
			return nil
		}
		fmt.Println("skip apply: questionnaire already exists (use --force to replace)")
		return nil
	}

	fmt.Printf("plan: seed questionnaire %s@%s with %d questions\n", seed.Code, seed.Version, len(seed.Questions))
	if !cfg.apply {
		fmt.Println("dry-run complete (pass --apply to write)")
		return nil
	}

	if existing != nil && cfg.force {
		if err := repo.HardDeleteFamily(ctx, questionnaireCode); err != nil {
			return fmt.Errorf("delete existing questionnaire: %w", err)
		}
	}

	if err := repo.Create(ctx, q); err != nil {
		return fmt.Errorf("create questionnaire head: %w", err)
	}
	if err := repo.Update(ctx, q); err != nil {
		return fmt.Errorf("update questionnaire head: %w", err)
	}
	if err := repo.CreatePublishedSnapshot(ctx, q, true); err != nil {
		return fmt.Errorf("create published snapshot: %w", err)
	}
	if err := repo.SetActivePublishedVersion(ctx, questionnaireCode, seed.Version); err != nil {
		return fmt.Errorf("activate published version: %w", err)
	}

	fmt.Printf("seeded SBTI_FUN@%s with %d questions\n", seed.Version, len(seed.Questions))
	return nil
}

func loadSeed() (questionnaireSeedFile, error) {
	var seed questionnaireSeedFile
	if err := json.Unmarshal(questionnaireSeedJSON, &seed); err != nil {
		return questionnaireSeedFile{}, fmt.Errorf("parse embedded questionnaire seed: %w", err)
	}
	if seed.Code == "" {
		seed.Code = questionnaireCode
	}
	return seed, nil
}

func buildQuestionnaire(seed questionnaireSeedFile, questions []domainQuestionnaire.Question) (*domainQuestionnaire.Questionnaire, error) {
	qType := domainQuestionnaire.NormalizeQuestionnaireType(seed.Type)
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode(seed.Code),
		seed.Title,
		domainQuestionnaire.WithDesc(seed.Description),
		domainQuestionnaire.WithImgUrl(seed.ImgURL),
		domainQuestionnaire.WithVersion(domainQuestionnaire.NewVersion(seed.Version)),
		domainQuestionnaire.WithStatus(domainQuestionnaire.STATUS_PUBLISHED),
		domainQuestionnaire.WithType(qType),
		domainQuestionnaire.WithActivePublished(true),
	)
	if err != nil {
		return nil, err
	}
	if err := q.ReplaceQuestions(questions); err != nil {
		return nil, err
	}
	return q, nil
}

func buildDomainQuestions(seed questionnaireSeedFile) ([]domainQuestionnaire.Question, error) {
	questions := make([]domainQuestionnaire.Question, 0, len(seed.Questions))
	for i, item := range seed.Questions {
		options := make([]domainQuestionnaire.Option, 0, len(item.Options))
		for _, opt := range item.Options {
			option, err := domainQuestionnaire.NewOptionWithStringCode(opt.Code, opt.Content, opt.Score)
			if err != nil {
				return nil, fmt.Errorf("question %s option %s: %w", item.Code, opt.Code, err)
			}
			options = append(options, option)
		}
		qType := domainQuestionnaire.QuestionType(item.Type)
		question, err := domainQuestionnaire.NewQuestion(
			domainQuestionnaire.WithCode(meta.NewCode(item.Code)),
			domainQuestionnaire.WithStem(item.Stem),
			domainQuestionnaire.WithQuestionType(qType),
			domainQuestionnaire.WithOptions(options),
		)
		if err != nil {
			return nil, fmt.Errorf("build question #%d (%s): %w", i+1, item.Code, err)
		}
		questions = append(questions, question)
	}
	return questions, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
