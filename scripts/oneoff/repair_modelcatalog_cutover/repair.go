package main

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	mongoindexes "github.com/FangcunMount/qs-server/internal/pkg/mongodb"
	"github.com/FangcunMount/qs-server/scripts/oneoff/internal/modelseed"
)

const minimumUnifiedMongoMigration = int64(13)

var (
	legacyModelFields = bson.M{
		"payload":                   "",
		"payload_format":            "",
		"definition_payload":        "",
		"definition_payload_format": "",
		"is_active_published":       "",
	}
	mysqlHistoryTables = []string{
		"assessment", "assessment_score", "evaluation_outcome", "runtime_checkpoint",
		"assessment_task", "plan_enrollment", "behavior_footprint", "assessment_episode",
		"domain_event_outbox", "retry_event_hold", "event_delivery_dead_letter",
	}
	mongoHistoryCollections = []string{
		"answersheets", "answersheet_submit_idempotency", "archived_reports",
		"interpret_report_artifacts", "interpretation_runs", "report_generations",
		"report_query_catalog", "interpretation_admission_failures",
		"interpretation_attention_projections", "domain_event_outbox",
	}
	retiredReportCollections = []string{
		"interpret_reports",
		"cleanup_bak_orphans_archived_reports_orphan_reports_20260712",
		"cleanup_bak_orphans_interpret_reports_orphan_reports_20260712",
	}
)

type repairPlan struct {
	GeneratedAt              time.Time        `json:"generated_at"`
	Mode                     string           `json:"mode"`
	MySQLHistory             map[string]int64 `json:"mysql_history"`
	MongoHistory             map[string]int64 `json:"mongo_history"`
	HeadCount                int64            `json:"head_count"`
	ActiveSnapshotCount      int64            `json:"active_snapshot_count"`
	ArchivedSnapshotCount    int64            `json:"archived_snapshot_count"`
	LegacyModelDocumentCount int64            `json:"legacy_model_document_count"`
	NormCount                int64            `json:"norm_count"`
	Repairs                  []repairItem     `json:"repairs"`
	Issues                   []repairIssue    `json:"issues,omitempty"`
}

type repairItem struct {
	Code                 string `json:"code"`
	Version              string `json:"version"`
	Kind                 string `json:"kind"`
	SubKind              string `json:"sub_kind,omitempty"`
	Algorithm            string `json:"algorithm"`
	AlgorithmFamily      string `json:"algorithm_family"`
	DecisionKind         string `json:"decision_kind"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	DefinitionHash       string `json:"definition_hash"`

	headRevision int64
	snapshot     *modelcatalogport.AssessmentSnapshot
	head         *domain.AssessmentModel
}

type repairIssue struct {
	Scope   string `json:"scope"`
	Code    string `json:"code,omitempty"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Count   int64  `json:"count,omitempty"`
}

func (p *repairPlan) Blocked() bool { return p == nil || len(p.Issues) > 0 }

func (p *repairPlan) addIssue(scope, code, rule, message string, count int64) {
	p.Issues = append(p.Issues, repairIssue{Scope: scope, Code: code, Rule: rule, Message: message, Count: count})
}

func (p *repairPlan) Text() string {
	if p == nil {
		return "ModelCatalog cutover repair: unavailable\n"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "ModelCatalog cutover repair: mode=%s active=%d archived=%d heads=%d legacy=%d norms=%d issues=%d generated_at=%s\n",
		p.Mode, p.ActiveSnapshotCount, p.ArchivedSnapshotCount, p.HeadCount, p.LegacyModelDocumentCount, p.NormCount, len(p.Issues), p.GeneratedAt.Format(time.RFC3339))
	for _, item := range p.Repairs {
		fmt.Fprintf(&out, "- repair %s@%s identity=%s/%s/%s runtime=%s/%s questionnaire=%s@%s hash=%s\n",
			item.Code, item.Version, item.Kind, item.SubKind, item.Algorithm, item.AlgorithmFamily, item.DecisionKind,
			item.QuestionnaireCode, item.QuestionnaireVersion, item.DefinitionHash)
	}
	for _, issue := range p.Issues {
		fmt.Fprintf(&out, "- BLOCKED %s %s", issue.Scope, issue.Rule)
		if issue.Code != "" {
			fmt.Fprintf(&out, " code=%s", issue.Code)
		}
		if issue.Count != 0 {
			fmt.Fprintf(&out, " count=%d", issue.Count)
		}
		fmt.Fprintf(&out, ": %s\n", issue.Message)
	}
	if len(p.Issues) == 0 {
		fmt.Fprintln(&out, "PASS: repair plan is complete and apply-safe")
	}
	return out.String()
}

func buildRepairPlan(ctx context.Context, mysqlDB *sql.DB, db *mongo.Database) (*repairPlan, error) {
	if mysqlDB == nil || db == nil {
		return nil, fmt.Errorf("mysql and mongo databases are required")
	}
	plan := &repairPlan{
		GeneratedAt:  time.Now().UTC(),
		MySQLHistory: make(map[string]int64, len(mysqlHistoryTables)),
		MongoHistory: make(map[string]int64, len(mongoHistoryCollections)),
	}
	if err := inspectHistoryGuards(ctx, mysqlDB, db, plan); err != nil {
		return nil, err
	}
	if err := inspectMongoSchema(ctx, db, plan); err != nil {
		return nil, err
	}
	if err := inspectNorms(ctx, db, plan); err != nil {
		return nil, err
	}
	if err := inspectModels(ctx, db, plan); err != nil {
		return nil, err
	}
	sort.Slice(plan.Repairs, func(i, j int) bool { return plan.Repairs[i].Code < plan.Repairs[j].Code })
	sort.Slice(plan.Issues, func(i, j int) bool {
		left, right := plan.Issues[i], plan.Issues[j]
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Rule < right.Rule
	})
	return plan, nil
}

func inspectHistoryGuards(ctx context.Context, mysqlDB *sql.DB, db *mongo.Database, plan *repairPlan) error {
	tables, err := mysqlDerivedHistoryTables(ctx, mysqlDB)
	if err != nil {
		return err
	}
	for _, table := range tables {
		var count int64
		if err := mysqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); err != nil {
			return fmt.Errorf("count mysql %s: %w", table, err)
		}
		plan.MySQLHistory[table] = count
		if count != 0 {
			plan.addIssue("mysql_history", table, "table.not_empty", "historical MySQL data must be empty before catalog repair", count)
		}
	}
	var testeeSummaryCount int64
	if err := mysqlDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM testee
WHERE total_assessments <> 0
   OR last_assessment_at IS NOT NULL
   OR last_risk_level IS NOT NULL`).Scan(&testeeSummaryCount); err != nil {
		return fmt.Errorf("count testee assessment summaries: %w", err)
	}
	plan.MySQLHistory["testee.assessment_summary"] = testeeSummaryCount
	if testeeSummaryCount != 0 {
		plan.addIssue("mysql_history", "testee.assessment_summary", "cache.not_empty", "testee assessment summary cache must be reset before catalog repair", testeeSummaryCount)
	}
	for _, name := range mongoHistoryCollections {
		count, err := db.Collection(name).CountDocuments(ctx, bson.M{})
		if err != nil {
			return fmt.Errorf("count mongo %s: %w", name, err)
		}
		plan.MongoHistory[name] = count
		if count != 0 {
			plan.addIssue("mongo_history", name, "collection.not_empty", "historical Mongo data must be empty before catalog repair", count)
		}
	}
	for _, name := range retiredReportCollections {
		rows, err := db.ListCollectionNames(ctx, bson.M{"name": name})
		if err != nil {
			return fmt.Errorf("inspect retired collection %s: %w", name, err)
		}
		if len(rows) != 0 {
			plan.addIssue("mongo_history", name, "retired_collection.present", "retired report collection must be absent before catalog repair", 1)
		}
	}
	return nil
}

func mysqlDerivedHistoryTables(ctx context.Context, db *sql.DB) ([]string, error) {
	seen := make(map[string]struct{}, len(mysqlHistoryTables)+16)
	for _, table := range mysqlHistoryTables {
		seen[table] = struct{}{}
	}
	rows, err := db.QueryContext(ctx, `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND (LEFT(table_name, 11) = 'statistics_'
       OR LEFT(table_name, 10) = 'analytics_')`)
	if err != nil {
		return nil, fmt.Errorf("list statistics and analytics tables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan statistics or analytics table: %w", err)
		}
		if strings.Contains(table, "`") {
			return nil, fmt.Errorf("unsafe MySQL table name %q", table)
		}
		seen[table] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list statistics and analytics tables: %w", err)
	}
	result := make([]string, 0, len(seen))
	for table := range seen {
		result = append(result, table)
	}
	sort.Strings(result)
	return result, nil
}

func inspectMongoSchema(ctx context.Context, db *mongo.Database, plan *repairPlan) error {
	var state struct {
		Version int64 `bson:"version"`
		Dirty   bool  `bson:"dirty"`
	}
	if err := db.Collection("schema_migrations").FindOne(ctx, bson.M{}).Decode(&state); err != nil {
		return fmt.Errorf("read mongo migration state: %w", err)
	}
	if state.Dirty {
		plan.addIssue("schema_migrations", "", "state.dirty", fmt.Sprintf("mongo migration is dirty at version %d", state.Version), 1)
	}
	if state.Version < minimumUnifiedMongoMigration {
		plan.addIssue("schema_migrations", "", "version.too_old", fmt.Sprintf("mongo migration version %d is older than required %d", state.Version, minimumUnifiedMongoMigration), 1)
	}

	legacyIndexes := mongoindexes.ForbiddenLegacyIndexNames()
	for collection, required := range mongoindexes.RequiredUnifiedIndexNames() {
		cursor, err := db.Collection(collection).Indexes().List(ctx)
		if err != nil {
			return fmt.Errorf("list indexes for %s: %w", collection, err)
		}
		names := map[string]struct{}{}
		for cursor.Next(ctx) {
			var row struct {
				Name string `bson:"name"`
			}
			if err := cursor.Decode(&row); err != nil {
				_ = cursor.Close(ctx)
				return fmt.Errorf("decode index for %s: %w", collection, err)
			}
			names[row.Name] = struct{}{}
		}
		if err := cursor.Err(); err != nil {
			_ = cursor.Close(ctx)
			return fmt.Errorf("list indexes for %s: %w", collection, err)
		}
		if err := cursor.Close(ctx); err != nil {
			return fmt.Errorf("close index cursor for %s: %w", collection, err)
		}
		for _, name := range required {
			if _, ok := names[name]; !ok {
				plan.addIssue("mongo_indexes", collection+"/"+name, "required.missing", "required unified index is missing", 1)
			}
		}
		for _, name := range legacyIndexes[collection] {
			if _, ok := names[name]; ok {
				plan.addIssue("mongo_indexes", collection+"/"+name, "legacy.present", "forbidden legacy index is still present", 1)
			}
		}
	}
	return nil
}

func inspectNorms(ctx context.Context, db *mongo.Database, plan *repairPlan) error {
	repo := mongomodelcatalog.NewNormRepository(db)
	for page := 1; ; page++ {
		rows, total, err := repo.ListNorms(ctx, modelcatalogport.NormListFilter{Page: page, PageSize: 100})
		if err != nil {
			return fmt.Errorf("list norms: %w", err)
		}
		plan.NormCount = total
		for _, table := range rows {
			if err := modelnorm.ValidateImport(table); err != nil {
				plan.addIssue("assessment_norms", table.TableVersion, "validate_import.failed", err.Error(), 1)
			}
		}
		if int64(page*100) >= total {
			break
		}
	}
	return nil
}

func inspectModels(ctx context.Context, db *mongo.Database, plan *repairPlan) error {
	models := db.Collection("assessment_models")
	var err error
	plan.HeadCount, err = models.CountDocuments(ctx, bson.M{"deleted_at": nil, "record_role": "head"})
	if err != nil {
		return fmt.Errorf("count model heads: %w", err)
	}
	plan.ActiveSnapshotCount, err = models.CountDocuments(ctx, activeSnapshotFilter())
	if err != nil {
		return fmt.Errorf("count active snapshots: %w", err)
	}
	plan.ArchivedSnapshotCount, err = models.CountDocuments(ctx, archivedSnapshotFilter())
	if err != nil {
		return fmt.Errorf("count archived snapshots: %w", err)
	}
	plan.LegacyModelDocumentCount, err = models.CountDocuments(ctx, legacyModelFilter())
	if err != nil {
		return fmt.Errorf("count legacy model documents: %w", err)
	}

	normRepo := mongomodelcatalog.NewNormRepository(db)
	registry := appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appdefinition.TypologyDefinitionHandler{},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo},
		appdefinition.CognitiveDefinitionHandler{NormRepo: normRepo},
	)
	publisher := publication.Publisher{Registry: registry}
	draftRepo := mongomodelcatalog.NewDraftRepository(db)
	publishedRepo := mongomodelcatalog.NewRepository(db)

	heads, err := listAllHeads(ctx, draftRepo)
	if err != nil {
		return err
	}
	active, err := listAllActiveSnapshots(ctx, publishedRepo)
	if err != nil {
		return err
	}
	if int64(len(active)) != plan.ActiveSnapshotCount {
		return fmt.Errorf("active snapshot list count=%d differs from inventory=%d", len(active), plan.ActiveSnapshotCount)
	}

	headByCode := make(map[string]*domain.AssessmentModel, len(heads))
	for _, head := range heads {
		if _, duplicate := headByCode[head.Code]; duplicate {
			plan.addIssue("assessment_models", head.Code, "head.duplicate", "multiple model heads have the same code", 1)
			continue
		}
		headByCode[head.Code] = head
	}
	activeByCode := make(map[string]int, len(active))
	for _, snapshot := range active {
		activeByCode[snapshot.Code]++
	}
	for code, count := range activeByCode {
		if count != 1 {
			plan.addIssue("assessment_models", code, "active.duplicate", "expected exactly one active snapshot per model code", int64(count))
		}
	}

	for _, current := range active {
		head := headByCode[current.Code]
		if head == nil {
			plan.addIssue("assessment_models", current.Code, "head.missing", "active snapshot has no model head", 1)
			continue
		}
		if !head.IsPublished() {
			plan.addIssue("assessment_models", current.Code, "head.not_published", fmt.Sprintf("active snapshot head status is %q", head.Status), 1)
			continue
		}
		item, issues := canonicalRepairItem(ctx, db, publisher, registry, head, current)
		for _, issue := range issues {
			plan.Issues = append(plan.Issues, issue)
		}
		if len(issues) == 0 {
			plan.Repairs = append(plan.Repairs, item)
		}
	}
	for _, head := range heads {
		if head.IsPublished() && activeByCode[head.Code] == 0 {
			plan.addIssue("assessment_models", head.Code, "active.missing", "published model head has no active snapshot", 1)
		}
	}
	return nil
}

func canonicalRepairItem(
	ctx context.Context,
	db *mongo.Database,
	publisher publication.Publisher,
	registry appdefinition.Registry,
	head *domain.AssessmentModel,
	current *modelcatalogport.PublishedModel,
) (repairItem, []repairIssue) {
	issues := make([]repairIssue, 0)
	add := func(rule, message string) {
		issues = append(issues, repairIssue{Scope: "published_runtime", Code: current.Code + "@" + current.Version, Rule: rule, Message: message, Count: 1})
	}
	if head.DefinitionV2 == nil || current.DefinitionV2 == nil {
		add("definition_v2.required", "head and active snapshot must both contain DefinitionV2")
		return repairItem{}, issues
	}
	if head.Kind != current.Kind || head.SubKind != current.SubKind || head.Algorithm != current.Algorithm {
		add("identity.head_mismatch", fmt.Sprintf("head identity %s/%s/%s differs from active %s/%s/%s", head.Kind, head.SubKind, head.Algorithm, current.Kind, current.SubKind, current.Algorithm))
	}
	if head.Binding.QuestionnaireCode != current.QuestionnaireCode || head.Binding.QuestionnaireVersion != current.QuestionnaireVersion {
		add("questionnaire.head_mismatch", fmt.Sprintf("head binding %s@%s differs from active %s@%s", head.Binding.QuestionnaireCode, head.Binding.QuestionnaireVersion, current.QuestionnaireCode, current.QuestionnaireVersion))
	}
	questionnaireCount, err := db.Collection("questionnaires").CountDocuments(ctx, bson.M{
		"deleted_at": nil, "record_role": "published_snapshot", "release_status": "active", "status": "published",
		"code": head.Binding.QuestionnaireCode, "version": head.Binding.QuestionnaireVersion,
	})
	if err != nil {
		add("questionnaire.lookup_failed", err.Error())
	} else if questionnaireCount != 1 {
		add("questionnaire.active_not_exact", fmt.Sprintf("exact active questionnaire snapshot count is %d, want 1", questionnaireCount))
	}

	headHash, err := modeldefinition.CanonicalContentHash(head.DefinitionV2)
	if err != nil {
		add("definition.head_hash_failed", err.Error())
	}
	activeHash, err := modeldefinition.CanonicalContentHash(current.DefinitionV2)
	if err != nil {
		add("definition.active_hash_failed", err.Error())
	} else if headHash != "" && activeHash != headHash {
		add("definition.head_active_mismatch", "head and active snapshot DefinitionV2 content hashes differ")
	}

	clone := cloneModel(head)
	modeldefinition.MaterializeLayers(clone.DefinitionV2)
	handler, err := registry.MustResolveBinding(appdefinition.AlgorithmBindingFromModel(clone))
	if err != nil {
		add("handler.missing", err.Error())
		return repairItem{}, issues
	}
	validation := handler.ValidateForPublish(ctx, clone)
	if domain.HasValidationErrors(validation) {
		for _, issue := range validation {
			if issue.Level == "warning" {
				continue
			}
			add("definition.publish_invalid."+issue.Code, issue.Field+": "+issue.Message)
		}
		return repairItem{}, issues
	}
	canonical, err := publisher.BuildSnapshot(ctx, clone)
	if err != nil {
		add("definition.runtime.invalid", err.Error())
		return repairItem{}, issues
	}
	if canonical.Version != current.Version {
		add("release_version.mismatch", fmt.Sprintf("head materializes version %s but active snapshot is %s", canonical.Version, current.Version))
	}
	if canonical.QuestionnaireCode != current.QuestionnaireCode || canonical.QuestionnaireVersion != current.QuestionnaireVersion {
		add("questionnaire.materialized_mismatch", fmt.Sprintf("materialized binding %s@%s differs from active %s@%s", canonical.QuestionnaireCode, canonical.QuestionnaireVersion, current.QuestionnaireCode, current.QuestionnaireVersion))
	}
	if headHash == "" {
		add("definition.hash.empty", "canonical DefinitionV2 hash is empty")
	}
	if current.PublishedAt == nil {
		add("published_at.missing", "active snapshot published_at is required for in-place repair")
	}
	if len(issues) > 0 {
		return repairItem{}, issues
	}
	modelcatalogport.AttachDefinitionHash(canonical, headHash)
	canonical.ReleaseStatus = domain.ReleaseStatusActive
	canonical.PublishedAt = current.PublishedAt
	canonical.ReleaseArchivedAt = nil
	return repairItem{
		Code: current.Code, Version: current.Version,
		Kind: string(canonical.Kind), SubKind: string(canonical.SubKind), Algorithm: string(canonical.Algorithm),
		AlgorithmFamily: string(canonical.AlgorithmFamily), DecisionKind: string(canonical.DecisionKind),
		QuestionnaireCode: canonical.QuestionnaireCode, QuestionnaireVersion: canonical.QuestionnaireVersion,
		DefinitionHash: headHash, headRevision: head.Revision(), snapshot: canonical, head: clone,
	}, nil
}

func cloneModel(model *domain.AssessmentModel) *domain.AssessmentModel {
	mapper := mongomodelcatalog.NewDraftMapper()
	return mapper.ToDomain(mapper.ToPO(model))
}

func listAllHeads(ctx context.Context, repo *mongomodelcatalog.DraftRepository) ([]*domain.AssessmentModel, error) {
	result := make([]*domain.AssessmentModel, 0)
	for page := 1; ; page++ {
		rows, total, err := repo.List(ctx, modelcatalogport.ListFilter{Page: page, PageSize: 100})
		if err != nil {
			return nil, fmt.Errorf("list model heads: %w", err)
		}
		result = append(result, rows...)
		if int64(page*100) >= total {
			return result, nil
		}
	}
}

func listAllActiveSnapshots(ctx context.Context, repo *mongomodelcatalog.Repository) ([]*modelcatalogport.PublishedModel, error) {
	result := make([]*modelcatalogport.PublishedModel, 0)
	for page := 1; ; page++ {
		rows, total, err := repo.ListPublishedModels(ctx, modelcatalogport.ListPublishedFilter{Page: page, PageSize: 100})
		if err != nil {
			return nil, fmt.Errorf("list active snapshots: %w", err)
		}
		result = append(result, rows...)
		if int64(page*100) >= total {
			return result, nil
		}
	}
}

func requireWritableReplicaSet(ctx context.Context, client *mongo.Client) error {
	var hello struct {
		SetName         string `bson:"setName"`
		WritablePrimary bool   `bson:"isWritablePrimary"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return fmt.Errorf("mongo hello: %w", err)
	}
	if hello.SetName == "" {
		return fmt.Errorf("MongoDB is not a replica set")
	}
	if !hello.WritablePrimary {
		return fmt.Errorf("MongoDB target is not the writable primary of replica set %s", hello.SetName)
	}
	return nil
}

func applyRepairPlan(ctx context.Context, client *mongo.Client, db *mongo.Database, plan *repairPlan) error {
	if plan == nil || plan.Blocked() {
		return fmt.Errorf("repair plan is blocked")
	}
	if client == nil || db == nil {
		return fmt.Errorf("mongo client and database are required")
	}
	runner := modelseed.NewMongoTransactionRunner(client)
	return modelseed.RunAtomically(ctx, runner, func(txCtx context.Context) error {
		return applyRepairDocuments(txCtx, db.Collection("assessment_models"), plan)
	})
}

type modelRepairCollection interface {
	DeleteMany(context.Context, interface{}, ...*options.DeleteOptions) (*mongo.DeleteResult, error)
	UpdateOne(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	UpdateMany(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

func applyRepairDocuments(ctx context.Context, models modelRepairCollection, plan *repairPlan) error {
	if models == nil {
		return fmt.Errorf("assessment_models collection is required")
	}
	if plan == nil || plan.Blocked() {
		return fmt.Errorf("repair plan is blocked")
	}
	deleted, err := models.DeleteMany(ctx, archivedSnapshotFilter())
	if err != nil {
		return fmt.Errorf("delete archived snapshots: %w", err)
	}
	if deleted.DeletedCount != plan.ArchivedSnapshotCount {
		return fmt.Errorf("delete archived snapshots: deleted=%d want=%d", deleted.DeletedCount, plan.ArchivedSnapshotCount)
	}

	for _, item := range plan.Repairs {
		headUpdate, err := canonicalHeadUpdate(item.head)
		if err != nil {
			return fmt.Errorf("build head update %s: %w", item.Code, err)
		}
		headResult, err := models.UpdateOne(ctx, bson.M{
			"deleted_at": nil, "record_role": "head", "code": item.Code, "revision": item.headRevision,
		}, headUpdate)
		if err != nil {
			return fmt.Errorf("update head %s: %w", item.Code, err)
		}
		if headResult.MatchedCount != 1 {
			return fmt.Errorf("update head %s: matched=%d want=1", item.Code, headResult.MatchedCount)
		}

		snapshotUpdate, err := canonicalSnapshotUpdate(item.snapshot)
		if err != nil {
			return fmt.Errorf("build snapshot update %s: %w", item.Code, err)
		}
		snapshotResult, err := models.UpdateOne(ctx, bson.M{
			"deleted_at": nil, "record_role": "published_snapshot", "release_status": "active",
			"code": item.Code, "release_version": item.Version,
		}, snapshotUpdate)
		if err != nil {
			return fmt.Errorf("update active snapshot %s@%s: %w", item.Code, item.Version, err)
		}
		if snapshotResult.MatchedCount != 1 {
			return fmt.Errorf("update active snapshot %s@%s: matched=%d want=1", item.Code, item.Version, snapshotResult.MatchedCount)
		}
	}

	if _, err := models.UpdateMany(ctx, bson.M{"deleted_at": nil}, bson.M{"$unset": legacyModelFields}); err != nil {
		return fmt.Errorf("remove legacy model fields: %w", err)
	}
	return nil
}

func canonicalHeadUpdate(model *domain.AssessmentModel) (bson.M, error) {
	po := mongomodelcatalog.NewDraftMapper().ToPO(model)
	data, err := po.ToBsonM()
	if err != nil {
		return nil, err
	}
	stripAuditFields(data)
	data["updated_at"] = time.Now().UTC()
	return bson.M{"$set": data, "$unset": legacyModelFields}, nil
}

func canonicalSnapshotUpdate(snapshot *modelcatalogport.AssessmentSnapshot) (bson.M, error) {
	po := mongomodelcatalog.NewMapper().ToPO(snapshot)
	data, err := po.ToBsonM()
	if err != nil {
		return nil, err
	}
	stripAuditFields(data)
	data["updated_at"] = time.Now().UTC()
	unset := cloneBSONMap(legacyModelFields)
	unset["release_archived_at"] = ""
	return bson.M{"$set": data, "$unset": unset}, nil
}

func stripAuditFields(data bson.M) {
	for _, field := range []string{"_id", "created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by"} {
		delete(data, field)
	}
}

func cloneBSONMap(source bson.M) bson.M {
	out := make(bson.M, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func verifyAppliedRepair(ctx context.Context, mysqlDB *sql.DB, db *mongo.Database, applied *repairPlan) error {
	if applied == nil {
		return fmt.Errorf("applied plan is nil")
	}
	post, err := buildRepairPlan(ctx, mysqlDB, db)
	if err != nil {
		return err
	}
	if post.Blocked() {
		return fmt.Errorf("post-apply plan remains blocked: %s", post.Text())
	}
	if post.ArchivedSnapshotCount != 0 {
		return fmt.Errorf("archived snapshots remain: %d", post.ArchivedSnapshotCount)
	}
	if post.LegacyModelDocumentCount != 0 {
		return fmt.Errorf("legacy model documents remain: %d", post.LegacyModelDocumentCount)
	}
	if post.ActiveSnapshotCount != applied.ActiveSnapshotCount {
		return fmt.Errorf("active snapshot count=%d want=%d", post.ActiveSnapshotCount, applied.ActiveSnapshotCount)
	}
	if len(post.Repairs) != len(applied.Repairs) {
		return fmt.Errorf("post-apply repair inventory=%d want=%d", len(post.Repairs), len(applied.Repairs))
	}
	for index := range post.Repairs {
		if !reflect.DeepEqual(publicRepairItem(post.Repairs[index]), publicRepairItem(applied.Repairs[index])) {
			return fmt.Errorf("post-apply repair item differs for %s", post.Repairs[index].Code)
		}
	}
	return nil
}

func publicRepairItem(item repairItem) repairItem {
	item.head = nil
	item.snapshot = nil
	item.headRevision = 0
	return item
}

func activeSnapshotFilter() bson.M {
	return bson.M{"deleted_at": nil, "record_role": "published_snapshot", "release_status": "active", "status": "published"}
}

func archivedSnapshotFilter() bson.M {
	return bson.M{"deleted_at": nil, "record_role": "published_snapshot", "release_status": "archived", "status": "published"}
}

func legacyModelFilter() bson.M {
	return bson.M{"deleted_at": nil, "$or": bson.A{
		bson.M{"payload": bson.M{"$exists": true}},
		bson.M{"payload_format": bson.M{"$exists": true}},
		bson.M{"definition_payload": bson.M{"$exists": true}},
		bson.M{"definition_payload_format": bson.M{"$exists": true}},
		bson.M{"is_active_published": bson.M{"$exists": true}},
	}}
}

var _ modelRepairCollection = (*mongo.Collection)(nil)
