package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
)

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatalf("migrate personality models failed: %v", err)
	}
}

type config struct {
	mongoURI string
	mongoDB  string
	apply    bool
	skipMBTI bool
	skipSBTI bool
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.BoolVar(&cfg.apply, "apply", false, "write draft + published models (default dry-run)")
	flag.BoolVar(&cfg.skipMBTI, "skip-mbti", false, "skip MBTI migration")
	flag.BoolVar(&cfg.skipSBTI, "skip-sbti", false, "skip SBTI migration")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	ctx := context.Background()
	models, err := collectDraftModels(ctx, cfg)
	if err != nil {
		return err
	}
	for _, model := range models {
		fmt.Printf("plan: migrate %s/%s algorithm=%s questionnaire=%s@%s\n",
			model.Kind, model.Code, model.Algorithm,
			model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion,
		)
	}
	if !cfg.apply {
		fmt.Println("dry-run complete (pass --apply to write)")
		return nil
	}
	if cfg.mongoURI == "" {
		return fmt.Errorf("mongo-uri is required (or set MONGO_URI)")
	}

	applyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	client, err := mongo.Connect(applyCtx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	db := client.Database(cfg.mongoDB)
	draftRepo := mongomodelcatalog.NewDraftRepository(db)
	publishedRepo := mongomodelcatalog.NewPublishedModelRepoAdapter(mongomodelcatalog.NewRepository(db))

	for _, model := range models {
		if err := draftRepo.Create(applyCtx, model); err != nil {
			return fmt.Errorf("create draft %s: %w", model.Code, err)
		}
		snapshot, err := aminfra.BuildPersonalityPublishedSnapshot(model)
		if err != nil {
			return fmt.Errorf("build snapshot %s: %w", model.Code, err)
		}
		if err := publishedRepo.Save(applyCtx, snapshot); err != nil {
			return fmt.Errorf("save published %s: %w", model.Code, err)
		}
		now := time.Now().UTC()
		if err := model.MarkPublished(now); err != nil {
			return fmt.Errorf("mark published %s: %w", model.Code, err)
		}
		if err := draftRepo.Update(applyCtx, model); err != nil {
			return fmt.Errorf("update draft %s: %w", model.Code, err)
		}
	}
	fmt.Printf("migrated %d personality model(s)\n", len(models))
	return nil
}

func collectDraftModels(ctx context.Context, cfg config) ([]*domain.AssessmentModel, error) {
	snapshots, err := rulesetInfra.DefaultEmbeddedSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	models := make([]*domain.AssessmentModel, 0)
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.Definition.Kind != domain.KindMBTIMigration && snapshot.Definition.Kind != domain.KindSBTIMigration {
			continue
		}
		if snapshot.Definition.Kind == domain.KindMBTIMigration && cfg.skipMBTI {
			continue
		}
		if snapshot.Definition.Kind == domain.KindSBTIMigration && cfg.skipSBTI {
			continue
		}
		payload, err := modeltypology.DecodeFromSnapshot(snapshot)
		if err != nil {
			return nil, err
		}
		runtime, err := payload.ToRuntimeSpec()
		if err != nil {
			return nil, err
		}
		runtimeBytes, err := jsonMarshal(runtime)
		if err != nil {
			return nil, err
		}
		kind, subKind, algorithm, _ := domain.LegacyKindMapping(snapshot.Definition.Kind)
		model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
			Code:      snapshot.Definition.Code,
			Kind:      kind,
			SubKind:   subKind,
			Algorithm: algorithm,
			Title:     snapshot.Definition.Title,
			Now:       now,
		})
		if err != nil {
			return nil, err
		}
		if err := model.BindQuestionnaire(snapshot.Binding, now); err != nil {
			return nil, err
		}
		if err := model.UpdateDefinition(domain.DefinitionPayload{
			Format: domain.PayloadFormatPersonalityTypologyV1,
			Data:   runtimeBytes,
		}, now); err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, nil
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
