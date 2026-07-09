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

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	legacyscale "github.com/FangcunMount/qs-server/scripts/oneoff/backfill_draft_assessment_models_from_scales/legacyscale"
)

func main() {
	mongoURI := flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	apply := flag.Bool("apply", false, "write backfill rows (default dry-run)")
	overwrite := flag.Bool("overwrite", false, "update existing assessment_models draft rows")
	withPublished := flag.Bool("with-published", false, "also upsert active scale snapshots into published_assessment_models")
	skipExistingPublished := flag.Bool("skip-existing-published", true, "skip published upsert when the same code+version already exists")
	flag.Parse()

	if *mongoURI == "" {
		log.Fatal("mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	db := client.Database(*mongoDB)
	scaleRepo := legacyscale.NewRepository(db)
	draftRepo := mongomodelcatalog.NewDraftRepository(db)
	publishedRepo := mongomodelcatalog.NewRepository(db)

	plan, err := buildPlan(ctx, scaleRepo, draftRepo, publishedRepo, *overwrite, *withPublished, *skipExistingPublished)
	if err != nil {
		log.Fatalf("build plan: %v", err)
	}
	fmt.Printf("plan: create %d draft(s), update %d draft(s), skip %d draft(s), upsert %d published snapshot(s), skip %d published snapshot(s)\n",
		plan.createCount, plan.updateCount, plan.skipCount, plan.upsertPublishedCount, plan.skipPublishedCount)
	for _, item := range plan.items {
		fmt.Printf("  - %s: %s\n", item.code, item.action)
	}
	if !*apply {
		fmt.Println("dry-run complete (pass --apply to write)")
		return
	}

	now := time.Now().UTC()
	for _, scale := range plan.scales {
		if scale == nil {
			continue
		}
		model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, now)
		if err != nil {
			log.Fatalf("convert %s: %v", scale.GetCode().String(), err)
		}
		existing, findErr := draftRepo.FindByCode(ctx, model.Code)
		switch {
		case findErr == nil && existing != nil && *overwrite:
			if err := draftRepo.Update(ctx, model); err != nil {
				log.Fatalf("update draft %s: %v", model.Code, err)
			}
		case domain.IsNotFound(findErr):
			if err := draftRepo.Create(ctx, model); err != nil {
				log.Fatalf("create draft %s: %v", model.Code, err)
			}
		}
	}

	if *withPublished {
		for _, item := range plan.publishedItems {
			if item.skip {
				continue
			}
			if err := publishedRepo.UpsertPublishedModel(ctx, item.snapshot); err != nil {
				log.Fatalf("upsert published %s@%s: %v", item.snapshot.Code, item.snapshot.Version, err)
			}
		}
	}

	fmt.Println("backfill complete")
}

type planItem struct {
	code   string
	action string
}

type backfillPlan struct {
	scales               []*scaledefinition.MedicalScale
	items                []planItem
	publishedItems       []publishedPlanItem
	createCount          int
	updateCount          int
	skipCount            int
	upsertPublishedCount int
	skipPublishedCount   int
}

type publishedPlanItem struct {
	snapshot *port.PublishedModel
	skip     bool
}

func buildPlan(
	ctx context.Context,
	scaleRepo *legacyscale.Repository,
	draftRepo *mongomodelcatalog.DraftRepository,
	publishedRepo *mongomodelcatalog.Repository,
	overwrite, withPublished, skipExistingPublished bool,
) (*backfillPlan, error) {
	scales, err := scaleRepo.ListHeadScales(ctx)
	if err != nil {
		return nil, err
	}
	plan := &backfillPlan{scales: scales}
	for _, scale := range scales {
		if scale == nil {
			continue
		}
		code := scale.GetCode().String()
		existing, findErr := draftRepo.FindByCode(ctx, code)
		switch {
		case findErr == nil && existing != nil && overwrite:
			plan.updateCount++
			plan.items = append(plan.items, planItem{code: code, action: "update draft"})
		case findErr == nil && existing != nil:
			plan.skipCount++
			plan.items = append(plan.items, planItem{code: code, action: "skip existing draft"})
		case domain.IsNotFound(findErr):
			plan.createCount++
			plan.items = append(plan.items, planItem{code: code, action: "create draft"})
		default:
			return nil, fmt.Errorf("find draft %s: %w", code, findErr)
		}
	}
	if withPublished {
		publishedScales, err := scaleRepo.ListActivePublishedSnapshots(ctx)
		if err != nil {
			return nil, err
		}
		snapshots, err := rulesetInfra.ScaleSnapshotsFromMedicalScales(publishedScales)
		if err != nil {
			return nil, err
		}
		for _, snapshot := range snapshots {
			if snapshot == nil {
				continue
			}
			item := publishedPlanItem{snapshot: snapshot}
			if skipExistingPublished {
				_, err := publishedRepo.GetPublishedModelByRef(ctx, port.Ref{
					Kind:      domain.KindScale,
					Code:      snapshot.Code,
					Version:   snapshot.Version,
					Algorithm: snapshot.Algorithm,
				})
				if err == nil {
					item.skip = true
					plan.skipPublishedCount++
					plan.items = append(plan.items, planItem{
						code:   snapshot.Code + "@" + snapshot.Version,
						action: "skip existing published",
					})
					plan.publishedItems = append(plan.publishedItems, item)
					continue
				}
				if !domain.IsNotFound(err) {
					return nil, fmt.Errorf("find published %s@%s: %w", snapshot.Code, snapshot.Version, err)
				}
			}
			plan.upsertPublishedCount++
			plan.items = append(plan.items, planItem{
				code:   snapshot.Code + "@" + snapshot.Version,
				action: "upsert published",
			})
			plan.publishedItems = append(plan.publishedItems, item)
		}
	}
	return plan, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
