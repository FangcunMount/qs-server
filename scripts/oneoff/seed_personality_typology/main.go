package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
)

const (
	mbtiQuestionnairePath = "scripts/oneoff/seed_personality_typology/data/mbti_questionnaire.json"
	sbtiQuestionnairePath = "scripts/oneoff/seed_personality_typology/data/sbti_questionnaire.json"
)

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatalf("seed personality typology failed: %v", err)
	}
}

type config struct {
	mongoURI      string
	mongoDB       string
	apply         bool
	force         bool
	skipMBTI      bool
	skipMBTI93    bool
	skipSBTI      bool
	skipBig5      bool
	skipEnneagram bool
	skipQuest     bool
	skipModel     bool
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.BoolVar(&cfg.apply, "apply", false, "write questionnaires and models (default dry-run)")
	flag.BoolVar(&cfg.force, "force", false, "replace existing questionnaire/model rows")
	flag.BoolVar(&cfg.skipMBTI, "skip-mbti", false, "skip MBTI OEJTS 32-question questionnaire and model")
	flag.BoolVar(&cfg.skipMBTI93, "skip-mbti93", false, "skip MBTI FC 93-question questionnaire and model")
	flag.BoolVar(&cfg.skipSBTI, "skip-sbti", false, "skip SBTI questionnaire and model")
	flag.BoolVar(&cfg.skipBig5, "skip-big5", false, "skip Big Five questionnaire and model")
	flag.BoolVar(&cfg.skipEnneagram, "skip-enneagram", false, "skip Enneagram questionnaire and model")
	flag.BoolVar(&cfg.skipQuest, "skip-questionnaires", false, "skip questionnaire seeding")
	flag.BoolVar(&cfg.skipModel, "skip-models", false, "skip assessment model seeding")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	plans, err := buildPlans(cfg)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		fmt.Printf("plan: %s questionnaire=%s model=%s\n", plan.label, plan.questionnaireVersion, plan.modelCode)
	}
	if !cfg.apply {
		fmt.Println("dry-run complete (pass --apply to write)")
		return nil
	}
	if cfg.mongoURI == "" {
		return fmt.Errorf("mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	db := client.Database(cfg.mongoDB)
	now := time.Now().UTC()

	if !cfg.skipQuest {
		questionnaireRepo := mongoQuestionnaire.NewRepository(db)
		for _, plan := range plans {
			if err := seedQuestionnaire(ctx, questionnaireRepo, plan.questionnaireSeed, cfg.force); err != nil {
				return err
			}
		}
	}

	if !cfg.skipModel {
		draftRepo := mongoassessmentmodel.NewDraftRepository(db)
		publishedRepo := mongoassessmentmodel.NewPublishedModelRepoAdapter(mongoassessmentmodel.NewRepository(db))
		for _, plan := range plans {
			payload, err := plan.modelPlan.Build()
			if err != nil {
				return fmt.Errorf("build payload %s: %w", plan.modelCode, err)
			}
			if err := validatePayloadAgainstQuestionnaire(payload, plan.questionnaireSeed); err != nil {
				return fmt.Errorf("validate %s: %w", plan.modelCode, err)
			}
			if err := seedAssessmentModel(ctx, draftRepo, publishedRepo, plan.modelPlan, payload, cfg.force, now); err != nil {
				return err
			}
		}
	}

	fmt.Printf("seeded %d personality typology target(s)\n", len(plans))
	return nil
}

type seedPlan struct {
	label                string
	modelCode            string
	questionnaireVersion string
	questionnaireSeed    questionnaireSeedFile
	modelPlan            modelSeedPlan
}

func buildPlans(cfg config) ([]seedPlan, error) {
	plans := make([]seedPlan, 0, 5)
	if !cfg.skipMBTI {
		plan, err := buildFileBackedPlan(
			"MBTI_OEJTS",
			mbtiQuestionnairePath,
			domain.AlgorithmMBTI,
			buildMBTIPayload,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if !cfg.skipMBTI93 {
		plan, err := buildFileBackedPlan(
			"MBTI_FC_93",
			mbti93QuestionnairePath,
			domain.AlgorithmMBTI,
			buildMBTI93Payload,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if !cfg.skipSBTI {
		plan, err := buildFileBackedPlan(
			"SBTI_FUN",
			sbtiQuestionnairePath,
			domain.AlgorithmSBTI,
			buildSBTIPayload,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if !cfg.skipBig5 {
		plan, err := buildFileBackedPlan(
			"BIG5_IPIP_50",
			big5QuestionnairePath,
			domain.AlgorithmBigFive,
			buildBig5Payload,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if !cfg.skipEnneagram {
		plan, err := buildFileBackedPlan(
			"ENNEAGRAM_45",
			enneagramQuestionnairePath,
			domain.AlgorithmPersonalityTypology,
			buildEnneagramPayload,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("nothing to seed: enable at least one personality model")
	}
	return plans, nil
}

func buildFileBackedPlan(
	label string,
	questionnairePath string,
	algorithm domain.Algorithm,
	build func() (*modeltypology.Payload, error),
) (seedPlan, error) {
	seed, err := loadQuestionnaireSeed(questionnairePath)
	if err != nil {
		return seedPlan{}, err
	}
	payload, err := build()
	if err != nil {
		return seedPlan{}, err
	}
	if err := validatePayloadAgainstQuestionnaire(payload, seed); err != nil {
		return seedPlan{}, err
	}
	return seedPlan{
		label:                label,
		modelCode:            label,
		questionnaireVersion: seed.Version,
		questionnaireSeed:    seed,
		modelPlan: modelSeedPlan{
			Code:      label,
			Algorithm: algorithm,
			Title:     payload.Title,
			Build:     build,
		},
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
