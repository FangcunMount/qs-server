package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelconclusion "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modelfactor "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	mongoindexes "github.com/FangcunMount/qs-server/internal/pkg/mongodb"
	"github.com/FangcunMount/qs-server/scripts/oneoff/internal/modelseed"
)

const minimumUnifiedMongoMigration = int64(13)

const (
	brief2LegacyNormVersion = "brief2-parent-cn-legacy-gXkk9W-v1"
	brief2LegacyNormFactor  = "XTwK5RCb"
)

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
	knownZOOQuestionRefs = []string{
		"7X2uSso2", "BhPDSP3i", "C54P1JBI", "D6BGiI9R", "DYWcJzmg",
		"JdHFdzgH", "KPijIhCr", "NaYz2zGp", "UZ28oCO9", "WnWSyHbZ",
		"bATLzBFo", "dWgLjZUJ", "disdOion", "fTyRCnAT", "fXMq8eYc",
		"lwDL7JEa", "scDrv1nz", "wDKVJwVR", "yM5i6mFK", "z1GFqkYx",
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
	NormRepairs              []normRepairItem `json:"norm_repairs,omitempty"`
	Repairs                  []repairItem     `json:"repairs"`
	Issues                   []repairIssue    `json:"issues,omitempty"`
}

type normRepairItem struct {
	TableVersion     string  `json:"table_version"`
	FactorCode       string  `json:"factor_code"`
	RawScoreMin      float64 `json:"raw_score_min"`
	RawScoreMax      float64 `json:"raw_score_max"`
	MinAgeMonths     int     `json:"min_age_months"`
	MaxAgeMonths     int     `json:"max_age_months"`
	Gender           string  `json:"gender"`
	TScore           float64 `json:"t_score"`
	BeforePercentile float64 `json:"before_percentile"`
	AfterPercentile  float64 `json:"after_percentile"`
}

type repairItem struct {
	Code                 string   `json:"code"`
	Version              string   `json:"version"`
	Kind                 string   `json:"kind"`
	SubKind              string   `json:"sub_kind,omitempty"`
	Algorithm            string   `json:"algorithm"`
	AlgorithmFamily      string   `json:"algorithm_family"`
	DecisionKind         string   `json:"decision_kind"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	DefinitionHash       string   `json:"definition_hash"`
	Normalizations       []string `json:"normalizations,omitempty"`

	snapshot *modelcatalogport.AssessmentSnapshot
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
	fmt.Fprintf(&out, "ModelCatalog cutover repair: mode=%s active=%d archived=%d heads=%d legacy=%d norms=%d issue_groups=%d findings=%d generated_at=%s\n",
		p.Mode, p.ActiveSnapshotCount, p.ArchivedSnapshotCount, p.HeadCount, p.LegacyModelDocumentCount, p.NormCount, len(p.Issues), p.findingCount(), p.GeneratedAt.Format(time.RFC3339))
	for _, item := range p.Repairs {
		fmt.Fprintf(&out, "- repair %s@%s identity=%s/%s/%s runtime=%s/%s questionnaire=%s@%s hash=%s\n",
			item.Code, item.Version, item.Kind, item.SubKind, item.Algorithm, item.AlgorithmFamily, item.DecisionKind,
			item.QuestionnaireCode, item.QuestionnaireVersion, item.DefinitionHash)
		if len(item.Normalizations) > 0 {
			fmt.Fprintf(&out, "  normalizations=%s\n", strings.Join(item.Normalizations, ","))
		}
	}
	for _, item := range p.NormRepairs {
		fmt.Fprintf(&out, "- repair norm %s factor=%s raw=%g age=%d-%d gender=%s t=%g percentile=%g->%g\n",
			item.TableVersion, item.FactorCode, item.RawScoreMin, item.MinAgeMonths, item.MaxAgeMonths,
			item.Gender, item.TScore, item.BeforePercentile, item.AfterPercentile)
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

func (p *repairPlan) findingCount() int64 {
	var total int64
	for _, issue := range p.Issues {
		if issue.Count > 0 {
			total += issue.Count
		} else {
			total++
		}
	}
	return total
}

func (p *repairPlan) coalesceIssues() {
	type issueKey struct {
		scope   string
		code    string
		rule    string
		message string
	}
	grouped := make(map[issueKey]repairIssue, len(p.Issues))
	for _, issue := range p.Issues {
		key := issueKey{scope: issue.Scope, code: issue.Code, rule: issue.Rule, message: issue.Message}
		count := issue.Count
		if count <= 0 {
			count = 1
		}
		if existing, ok := grouped[key]; ok {
			existing.Count += count
			grouped[key] = existing
			continue
		}
		issue.Count = count
		grouped[key] = issue
	}
	p.Issues = p.Issues[:0]
	for _, issue := range grouped {
		p.Issues = append(p.Issues, issue)
	}
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
	plan.coalesceIssues()
	sort.Slice(plan.Repairs, func(i, j int) bool { return plan.Repairs[i].Code < plan.Repairs[j].Code })
	sort.Slice(plan.Issues, func(i, j int) bool {
		left, right := plan.Issues[i], plan.Issues[j]
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Rule != right.Rule {
			return left.Rule < right.Rule
		}
		return left.Message < right.Message
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
	wanted := make(map[string]struct{}, len(mysqlHistoryTables))
	for _, table := range mysqlHistoryTables {
		wanted[table] = struct{}{}
	}
	rows, err := db.QueryContext(ctx, `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_type = 'BASE TABLE'`)
	if err != nil {
		return nil, fmt.Errorf("list MySQL tables for history guard: %w", err)
	}
	defer func() { _ = rows.Close() }()
	seen := make(map[string]struct{}, len(mysqlHistoryTables)+16)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan MySQL table for history guard: %w", err)
		}
		if strings.Contains(table, "`") {
			return nil, fmt.Errorf("unsafe MySQL table name %q", table)
		}
		_, fixedHistoryTable := wanted[table]
		if fixedHistoryTable || strings.HasPrefix(table, "statistics_") || strings.HasPrefix(table, "analytics_") {
			seen[table] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list MySQL tables for history guard: %w", err)
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
			repairs, normalizeErr := normalizeKnownNormCorruption(table)
			if normalizeErr != nil {
				plan.addIssue("assessment_norms", table.TableVersion, "known_repair.ambiguous", normalizeErr.Error(), 1)
			} else {
				plan.NormRepairs = append(plan.NormRepairs, repairs...)
			}
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

// normalizeKnownNormCorruption repairs one source-proven transcription error.
// Every identifying field and the corrupt value must match exactly; all other
// invalid norm data remains a blocking finding.
func normalizeKnownNormCorruption(table *modelnorm.Norm) ([]normRepairItem, error) {
	if table == nil || table.TableVersion != brief2LegacyNormVersion {
		return nil, nil
	}
	type location struct {
		factor int
		row    int
	}
	matches := make([]location, 0, 1)
	for factorIndex := range table.Factors {
		factor := &table.Factors[factorIndex]
		if factor.FactorCode != brief2LegacyNormFactor {
			continue
		}
		for rowIndex := range factor.Lookup {
			row := factor.Lookup[rowIndex]
			if row.RawScoreMin == 125 && row.RawScoreMax == 125 &&
				row.MinAgeMonths == 60 && row.MaxAgeMonths == 95 && row.Gender == "male" &&
				row.TScore == 65 && row.Percentile == 952 {
				matches = append(matches, location{factor: factorIndex, row: rowIndex})
			}
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("known BRIEF-2 percentile corruption matched %d rows, want exactly 1", len(matches))
	}
	match := matches[0]
	row := &table.Factors[match.factor].Lookup[match.row]
	repair := normRepairItem{
		TableVersion: table.TableVersion, FactorCode: brief2LegacyNormFactor,
		RawScoreMin: row.RawScoreMin, RawScoreMax: row.RawScoreMax,
		MinAgeMonths: row.MinAgeMonths, MaxAgeMonths: row.MaxAgeMonths, Gender: row.Gender,
		TScore: row.TScore, BeforePercentile: row.Percentile, AfterPercentile: 92,
	}
	table.Factors[match.factor].Lookup[match.row].Percentile = repair.AfterPercentile
	return []normRepairItem{repair}, nil
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

	normRepo := plannedNormRepository{base: mongomodelcatalog.NewNormRepository(db)}
	questionnaireRepo := mongoquestionnaire.NewRepository(db)
	questionnaireQuery := questionnaireapp.NewQueryService(
		questionnaireRepo,
		nil,
		nil,
		mongoquestionnaire.NewQuestionnaireReadModel(questionnaireRepo),
	)
	registry := appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{QuestionnaireQuery: questionnaireQuery},
		appdefinition.TypologyDefinitionHandler{QuestionnaireQuery: questionnaireQuery},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo, QuestionnaireQuery: questionnaireQuery},
		appdefinition.CognitiveDefinitionHandler{NormRepo: normRepo, QuestionnaireQuery: questionnaireQuery},
	)
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
		item, issues := canonicalRepairItem(ctx, db, registry, current)
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

// plannedNormRepository presents the exact post-repair Norm view to the
// Definition publish guards during dry-run. It never writes and never hides an
// unknown Norm error; apply persists the same known repair in the transaction.
type plannedNormRepository struct {
	base modelcatalogport.NormRepository
}

func (r plannedNormRepository) UpsertNorm(context.Context, *modelnorm.Norm) error {
	return fmt.Errorf("planned norm repository is read-only")
}

func (r plannedNormRepository) FindNorm(ctx context.Context, tableVersion string) (*modelnorm.Norm, error) {
	if r.base == nil {
		return nil, fmt.Errorf("base norm repository is required")
	}
	table, err := r.base.FindNorm(ctx, tableVersion)
	if err != nil || table == nil {
		return table, err
	}
	planned := cloneNorm(table)
	if _, err := normalizeKnownNormCorruption(planned); err != nil {
		return nil, err
	}
	return planned, nil
}

func (r plannedNormRepository) ListNorms(ctx context.Context, filter modelcatalogport.NormListFilter) ([]*modelnorm.Norm, int64, error) {
	if r.base == nil {
		return nil, 0, fmt.Errorf("base norm repository is required")
	}
	return r.base.ListNorms(ctx, filter)
}

func cloneNorm(table *modelnorm.Norm) *modelnorm.Norm {
	if table == nil {
		return nil
	}
	out := *table
	if table.Factors == nil {
		return &out
	}
	out.Factors = make([]modelnorm.FactorTable, len(table.Factors))
	for index := range table.Factors {
		out.Factors[index] = table.Factors[index]
		out.Factors[index].Bands = append([]modelnorm.Band(nil), table.Factors[index].Bands...)
		out.Factors[index].Lookup = append([]modelnorm.LookupEntry(nil), table.Factors[index].Lookup...)
	}
	return &out
}

func canonicalRepairItem(
	ctx context.Context,
	db *mongo.Database,
	registry appdefinition.Registry,
	current *modelcatalogport.PublishedModel,
) (repairItem, []repairIssue) {
	issues := make([]repairIssue, 0)
	add := func(rule, message string) {
		issues = append(issues, repairIssue{Scope: "published_runtime", Code: current.Code + "@" + current.Version, Rule: rule, Message: message, Count: 1})
	}
	if current.DefinitionV2 == nil {
		add("definition_v2.required", "active snapshot must contain DefinitionV2")
		return repairItem{}, issues
	}
	canonical := clonePublishedSnapshot(current)
	bindingNormalizations := normalizeKnownQuestionnaireBinding(canonical)
	questionnaireCount, err := db.Collection("questionnaires").CountDocuments(ctx, bson.M{
		"deleted_at": nil, "record_role": "published_snapshot", "release_status": bson.M{"$in": bson.A{"active", "archived"}}, "status": "published",
		"code": canonical.QuestionnaireCode, "version": canonical.QuestionnaireVersion,
	})
	if err != nil {
		add("questionnaire.lookup_failed", err.Error())
	} else if questionnaireCount != 1 {
		add("questionnaire.published_not_exact", fmt.Sprintf("exact retained published questionnaire snapshot count is %d, want 1", questionnaireCount))
	}

	normalizations := append(bindingNormalizations, normalizeLegacyDefinition(canonical.DefinitionV2)...)
	modeldefinition.MaterializeLayers(canonical.DefinitionV2)
	model := publication.ModelFromPublishedSnapshot(canonical)
	handler, err := registry.MustResolveBinding(appdefinition.AlgorithmBindingFromModel(model))
	if err != nil {
		add("handler.missing", err.Error())
		return repairItem{}, issues
	}
	validation := handler.ValidateForPublish(ctx, model)
	if domain.HasValidationErrors(validation) {
		for _, issue := range validation {
			if issue.Level == "warning" {
				continue
			}
			add("definition.publish_invalid."+issue.Code, issue.Field+": "+issue.Message)
		}
		return repairItem{}, issues
	}
	materialized, err := handler.MaterializeSnapshot(ctx, model)
	if err != nil {
		add("definition.runtime.invalid", err.Error())
		return repairItem{}, issues
	}
	definitionHash, err := modeldefinition.CanonicalContentHash(model.DefinitionV2)
	if err != nil {
		add("definition.active_hash_failed", err.Error())
	}
	if definitionHash == "" {
		add("definition.hash.empty", "canonical DefinitionV2 hash is empty")
	}
	if current.PublishedAt == nil {
		add("published_at.missing", "active snapshot published_at is required for in-place repair")
	}
	if len(issues) > 0 {
		return repairItem{}, issues
	}
	canonical.SchemaVersion = domain.SchemaVersionV2
	canonical.ProductChannel = domain.ResolveProductChannel(materialized.Kind, canonical.ProductChannel)
	canonical.Kind = materialized.Kind
	canonical.SubKind = materialized.SubKind
	canonical.Algorithm = materialized.Algorithm
	canonical.AlgorithmFamily = materialized.AlgorithmFamily
	canonical.DecisionKind = materialized.DecisionKind
	canonical.Status = string(domain.ModelStatusPublished)
	canonical.DefinitionV2 = model.DefinitionV2
	modelcatalogport.AttachDefinitionHash(canonical, definitionHash)
	canonical.ReleaseStatus = domain.ReleaseStatusActive
	canonical.PublishedAt = current.PublishedAt
	canonical.ReleaseArchivedAt = nil
	return repairItem{
		Code: current.Code, Version: current.Version,
		Kind: string(canonical.Kind), SubKind: string(canonical.SubKind), Algorithm: string(canonical.Algorithm),
		AlgorithmFamily: string(canonical.AlgorithmFamily), DecisionKind: string(canonical.DecisionKind),
		QuestionnaireCode: canonical.QuestionnaireCode, QuestionnaireVersion: canonical.QuestionnaireVersion,
		DefinitionHash: definitionHash, Normalizations: normalizations, snapshot: canonical,
	}, nil
}

// normalizeKnownQuestionnaireBinding restores the frozen questionnaire
// identity for one source-proven catalog mismatch. The 20 Definition question
// refs match zOO4eG@5.0.1 exactly and none exists in the incorrectly bound
// zOO4eG@6.0.1 snapshot. Any identity or ref drift leaves the item blocked.
func normalizeKnownQuestionnaireBinding(snapshot *modelcatalogport.PublishedModel) []string {
	if snapshot == nil || snapshot.DefinitionV2 == nil ||
		snapshot.Code != "zOO4eG" || snapshot.Version != "v9" ||
		snapshot.QuestionnaireCode != "zOO4eG" || snapshot.QuestionnaireVersion != "6.0.1" {
		return nil
	}
	actual := make(map[string]struct{}, len(knownZOOQuestionRefs))
	for _, scoring := range snapshot.DefinitionV2.Measure.Scoring {
		for _, source := range scoring.Sources {
			if source.Kind == modelfactor.ScoringSourceQuestion && source.Code != "" {
				actual[source.Code] = struct{}{}
			}
		}
	}
	if len(actual) != len(knownZOOQuestionRefs) {
		return nil
	}
	for _, code := range knownZOOQuestionRefs {
		if _, exists := actual[code]; !exists {
			return nil
		}
	}
	snapshot.QuestionnaireVersion = "5.0.1"
	return []string{"questionnaire_binding:1"}
}

// normalizeLegacyDefinition makes historical implicit semantics explicit only
// where the current runtime already has a unique compatibility interpretation.
// It must not guess questionnaire refs, outcome codes without a legacy level,
// arbitrary score gaps, or unknown norm values.
func normalizeLegacyDefinition(def *modeldefinition.Definition) []string {
	if def == nil {
		return nil
	}
	var scoringModes, contributionDefaults int
	for ruleIndex := range def.Measure.Scoring {
		for sourceIndex := range def.Measure.Scoring[ruleIndex].Sources {
			source := &def.Measure.Scoring[ruleIndex].Sources[sourceIndex]
			if source.Kind != modelfactor.ScoringSourceQuestion || source.ScoringMode != "" {
				continue
			}
			if source.OptionScores != nil && len(source.OptionScores) == 0 {
				continue
			}
			if len(source.OptionScores) > 0 {
				source.ScoringMode = modelfactor.QuestionScoringModeOptionOverride
			} else {
				source.ScoringMode = modelfactor.QuestionScoringModeQuestionScore
			}
			scoringModes++
			if source.Sign == 0 {
				source.Sign = 1
				contributionDefaults++
			}
			if source.Weight == 0 {
				source.Weight = 1
				contributionDefaults++
			}
		}
	}

	outcomeRegistry := synthesizeLegacyOutcomeRegistry(def)
	var outcomeCodes, rangeAdjacency, rangeEndpoints int
	knownOutcomeCodes := make(map[string]struct{}, len(def.Outcomes))
	for _, outcome := range def.Outcomes {
		if outcome.Code != "" {
			knownOutcomeCodes[outcome.Code] = struct{}{}
		}
	}
	for index, item := range def.Conclusions {
		switch typed := item.(type) {
		case modelconclusion.RiskConclusion:
			normalizeLegacyScoreRanges(typed.Rules, knownOutcomeCodes, &outcomeCodes, &rangeAdjacency, &rangeEndpoints)
			def.Conclusions[index] = typed
		case modelconclusion.NormConclusion:
			normalizeLegacyScoreRanges(typed.Rules, knownOutcomeCodes, &outcomeCodes, &rangeAdjacency, &rangeEndpoints)
			def.Conclusions[index] = typed
		case modelconclusion.AbilityConclusion:
			normalizeLegacyScoreRanges(typed.Rules, knownOutcomeCodes, &outcomeCodes, &rangeAdjacency, &rangeEndpoints)
			def.Conclusions[index] = typed
		}
	}

	result := make([]string, 0, 6)
	appendCount := func(name string, count int) {
		if count > 0 {
			result = append(result, fmt.Sprintf("%s:%d", name, count))
		}
	}
	appendCount("scoring_mode", scoringModes)
	appendCount("contribution_defaults", contributionDefaults)
	appendCount("outcome_registry", outcomeRegistry)
	appendCount("outcome_code", outcomeCodes)
	appendCount("range_adjacency", rangeAdjacency)
	appendCount("range_endpoint", rangeEndpoints)
	return result
}

type legacyOutcomeCandidate struct {
	code     string
	title    string
	invalid  bool
	observed bool
}

func synthesizeLegacyOutcomeRegistry(def *modeldefinition.Definition) int {
	if def == nil {
		return 0
	}
	known := make(map[string]struct{}, len(def.Outcomes))
	for _, outcome := range def.Outcomes {
		if outcome.Code != "" {
			known[outcome.Code] = struct{}{}
		}
	}
	order := make([]string, 0)
	candidates := make(map[string]*legacyOutcomeCandidate)
	visit := func(rules []modelconclusion.ScoreRangeOutcome) {
		for _, rule := range rules {
			if rule.OutcomeCode != "" || rule.Level == "" {
				continue
			}
			if _, exists := known[rule.Level]; exists {
				continue
			}
			candidate := candidates[rule.Level]
			if candidate == nil {
				candidate = &legacyOutcomeCandidate{code: rule.Level}
				candidates[rule.Level] = candidate
				order = append(order, rule.Level)
			}
			if !isSafeLegacyOutcomeCode(rule.Level) || rule.Title == "" {
				candidate.invalid = true
				continue
			}
			if candidate.observed && candidate.title != rule.Title {
				candidate.invalid = true
				continue
			}
			candidate.title = rule.Title
			candidate.observed = true
		}
	}
	for _, item := range def.Conclusions {
		switch typed := item.(type) {
		case modelconclusion.RiskConclusion:
			visit(typed.Rules)
		case modelconclusion.NormConclusion:
			visit(typed.Rules)
		case modelconclusion.AbilityConclusion:
			visit(typed.Rules)
		}
	}
	added := 0
	for _, code := range order {
		candidate := candidates[code]
		if candidate.invalid || !candidate.observed {
			continue
		}
		def.Outcomes = append(def.Outcomes, modelconclusion.Outcome{Code: candidate.code, Title: candidate.title})
		known[code] = struct{}{}
		added++
	}
	return added
}

func isSafeLegacyOutcomeCode(code string) bool {
	if len(code) == 0 || len(code) > 64 || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for index := 1; index < len(code); index++ {
		value := code[index]
		if (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') || value == '_' {
			continue
		}
		return false
	}
	return true
}

func normalizeLegacyScoreRanges(
	rules []modelconclusion.ScoreRangeOutcome,
	knownOutcomeCodes map[string]struct{},
	outcomeCodes, rangeAdjacency, rangeEndpoints *int,
) {
	if len(rules) == 0 {
		return
	}
	for index := range rules {
		_, levelIsCanonicalCode := knownOutcomeCodes[rules[index].Level]
		if rules[index].OutcomeCode == "" && rules[index].Level != "" && levelIsCanonicalCode {
			rules[index].OutcomeCode = rules[index].Level
			(*outcomeCodes)++
		}
	}

	ordered := make([]int, len(rules))
	for index := range rules {
		ordered[index] = index
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := rules[ordered[i]], rules[ordered[j]]
		if left.MinScore == right.MinScore {
			return left.MaxScore < right.MaxScore
		}
		return left.MinScore < right.MinScore
	})
	for index := 0; index+1 < len(ordered); index++ {
		left := &rules[ordered[index]]
		right := rules[ordered[index+1]]
		if left.UnboundedMax || left.MaxInclusive || !legacyUnitGap(left.MaxScore, right.MinScore) {
			continue
		}
		left.MaxScore = right.MinScore
		(*rangeAdjacency)++
	}
	last := &rules[ordered[len(ordered)-1]]
	if !last.UnboundedMax && !last.MaxInclusive {
		last.MaxInclusive = true
		(*rangeEndpoints)++
	}
}

func legacyUnitGap(leftMax, rightMin float64) bool {
	return math.Trunc(leftMax) == leftMax &&
		math.Trunc(rightMin) == rightMin &&
		rightMin-leftMax == 1
}

func clonePublishedSnapshot(snapshot *modelcatalogport.PublishedModel) *modelcatalogport.PublishedModel {
	mapper := mongomodelcatalog.NewMapper()
	return mapper.ToPublished(mapper.ToPO(snapshot))
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
		if err := applyNormRepairDocuments(txCtx, db.Collection("assessment_norms"), plan.NormRepairs); err != nil {
			return err
		}
		return applyRepairDocuments(txCtx, db.Collection("assessment_models"), plan)
	})
}

type normRepairCollection interface {
	UpdateOne(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

func applyNormRepairDocuments(ctx context.Context, norms normRepairCollection, repairs []normRepairItem) error {
	if norms == nil {
		return fmt.Errorf("assessment_norms collection is required")
	}
	for _, item := range repairs {
		rowFilter := bson.M{
			"row.raw_score_min": item.RawScoreMin, "row.raw_score_max": item.RawScoreMax,
			"row.min_age_months": item.MinAgeMonths, "row.max_age_months": item.MaxAgeMonths,
			"row.gender": item.Gender, "row.t_score": item.TScore, "row.percentile": item.BeforePercentile,
		}
		result, err := norms.UpdateOne(ctx,
			bson.M{
				"table_version": item.TableVersion, "deleted_at": nil,
				"factors": bson.M{"$elemMatch": bson.M{
					"factor_code": item.FactorCode,
					"lookup": bson.M{"$elemMatch": bson.M{
						"raw_score_min": item.RawScoreMin, "raw_score_max": item.RawScoreMax,
						"min_age_months": item.MinAgeMonths, "max_age_months": item.MaxAgeMonths,
						"gender": item.Gender, "t_score": item.TScore, "percentile": item.BeforePercentile,
					}},
				}},
			},
			bson.M{"$set": bson.M{
				"factors.$[factor].lookup.$[row].percentile": item.AfterPercentile,
				"updated_at": time.Now().UTC(),
			}},
			options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
				bson.M{"factor.factor_code": item.FactorCode}, rowFilter,
			}}),
		)
		if err != nil {
			return fmt.Errorf("repair norm %s factor %s: %w", item.TableVersion, item.FactorCode, err)
		}
		if result.MatchedCount != 1 || result.ModifiedCount != 1 {
			return fmt.Errorf("repair norm %s factor %s: matched=%d modified=%d want=1/1",
				item.TableVersion, item.FactorCode, result.MatchedCount, result.ModifiedCount)
		}
	}
	return nil
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
	if len(post.NormRepairs) != 0 {
		return fmt.Errorf("known norm repairs remain after apply: %d", len(post.NormRepairs))
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
	item.snapshot = nil
	item.Normalizations = nil
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
var _ normRepairCollection = (*mongo.Collection)(nil)
