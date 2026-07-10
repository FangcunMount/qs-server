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
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioral "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitive "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scale "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

const pageSize = 100

type counters struct {
	draftCreate, draftSkip, draftConflict             int
	publishedCreate, publishedSkip, publishedConflict int
	normCreate, normConflict                          int
}

func main() {
	mongoURI := flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	apply := flag.Bool("apply", false, "write DefinitionV2 and norm rows (default dry-run)")
	flag.Parse()
	if *mongoURI == "" {
		log.Fatal("mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	opts := mongoBase.BaseRepositoryOptions{}
	db := client.Database(*mongoDB)
	drafts := mongomodelcatalog.NewDraftRepository(db, opts)
	published := mongomodelcatalog.NewRepository(db, opts)
	norms := mongomodelcatalog.NewNormRepository(db, opts)

	draftRows, err := allDrafts(ctx, drafts)
	if err != nil {
		log.Fatalf("list drafts: %v", err)
	}
	publishedRows, err := allPublished(ctx, published)
	if err != nil {
		log.Fatalf("list published models: %v", err)
	}

	result := counters{}
	for _, model := range draftRows {
		if err := migrateDraft(ctx, drafts, norms, model, *apply, &result); err != nil {
			log.Printf("draft %s: %v", model.Code, err)
			result.draftConflict++
		}
	}
	for _, model := range publishedRows {
		if err := migratePublished(ctx, published, norms, model, *apply, &result); err != nil {
			log.Printf("published %s@%s: %v", model.Code, model.Version, err)
			result.publishedConflict++
		}
	}
	fmt.Printf("definition_v2 migration (%s): drafts create=%d skip=%d conflict=%d; published create=%d skip=%d conflict=%d; norms create=%d conflict=%d\n",
		mode(*apply), result.draftCreate, result.draftSkip, result.draftConflict,
		result.publishedCreate, result.publishedSkip, result.publishedConflict, result.normCreate, result.normConflict)
}

func migrateDraft(ctx context.Context, repo *mongomodelcatalog.DraftRepository, norms port.NormRepository, model *domain.AssessmentModel, apply bool, result *counters) error {
	if model == nil || model.DefinitionV2 != nil {
		result.draftSkip++
		return nil
	}
	materialized, err := materialize(model.Kind, model.Algorithm, model.Definition.Data)
	if err != nil {
		return err
	}
	if err := upsertNorms(ctx, norms, materialized.Norms, apply, result); err != nil {
		return err
	}
	result.draftCreate++
	if !apply {
		return nil
	}
	if err := model.UpdateDefinitionWithV2(model.Definition, materialized.Definition, time.Now().UTC()); err != nil {
		return err
	}
	return repo.Update(ctx, model)
}

func migratePublished(ctx context.Context, repo *mongomodelcatalog.Repository, norms port.NormRepository, model *port.PublishedModel, apply bool, result *counters) error {
	if model == nil || model.DefinitionV2 != nil {
		result.publishedSkip++
		return nil
	}
	materialized, err := materialize(model.Kind, model.Algorithm, model.Payload)
	if err != nil {
		return err
	}
	if err := upsertNorms(ctx, norms, materialized.Norms, apply, result); err != nil {
		return err
	}
	result.publishedCreate++
	if !apply {
		return nil
	}
	model.DefinitionV2 = materialized.Definition
	return repo.UpsertPublishedModel(ctx, model)
}

func materialize(kind domain.Kind, algorithm domain.Algorithm, payload []byte) (shared.DefinitionMaterialization, error) {
	switch kind {
	case domain.KindScale:
		snapshot, err := scale.ParsePublishedPayload(payload)
		if err != nil {
			return shared.DefinitionMaterialization{}, err
		}
		return shared.DefinitionMaterialization{Definition: scale.DefinitionFromScaleSnapshot(snapshot)}, nil
	case domain.KindBehavioralRating:
		return behavioral.MaterializeDefinition(payload)
	case domain.KindCognitive:
		return cognitive.MaterializeDefinition(payload)
	case domain.KindTypology:
		return typology.MaterializeDefinition(payload, algorithm)
	default:
		return shared.DefinitionMaterialization{}, fmt.Errorf("unsupported model kind %q", kind)
	}
}

func upsertNorms(ctx context.Context, repo port.NormRepository, tables []*norm.Norm, apply bool, result *counters) error {
	for _, table := range tables {
		if table == nil {
			continue
		}
		result.normCreate++
		if !apply {
			continue
		}
		if err := repo.UpsertNorm(ctx, table); err != nil {
			result.normConflict++
			return err
		}
	}
	return nil
}

func allDrafts(ctx context.Context, repo *mongomodelcatalog.DraftRepository) ([]*domain.AssessmentModel, error) {
	return collectDraftPages(ctx, repo)
}

func collectDraftPages(ctx context.Context, repo *mongomodelcatalog.DraftRepository) ([]*domain.AssessmentModel, error) {
	items := make([]*domain.AssessmentModel, 0)
	for page := 1; ; page++ {
		rows, total, err := repo.List(ctx, port.ListFilter{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		items = append(items, rows...)
		if int64(len(items)) >= total || len(rows) == 0 {
			return items, nil
		}
	}
}

func allPublished(ctx context.Context, repo *mongomodelcatalog.Repository) ([]*port.PublishedModel, error) {
	items := make([]*port.PublishedModel, 0)
	for page := 1; ; page++ {
		rows, total, err := repo.ListPublishedModels(ctx, port.ListPublishedFilter{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		items = append(items, rows...)
		if int64(len(items)) >= total || len(rows) == 0 {
			return items, nil
		}
	}
}

func mode(apply bool) string {
	if apply {
		return "apply"
	}
	return "dry-run"
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
