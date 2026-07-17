// unify_assessment_model_records migrates modelcatalog drafts and published
// runtime snapshots into the single assessment_models collection. It is an
// operator tool for a maintenance window; application startup must never run it.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	inframodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	typologypayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

const (
	headCollection           = "assessment_models"
	questionnaireCollection  = "questionnaires"
	publishedCollection      = "published_assessment_models"
	defaultTemp              = "assessment_models_unified_staging"
	defaultQuestionnaireTemp = "questionnaires_unified_staging"
	roleHead                 = "head"
	roleSnapshot             = "published_snapshot"
)

type config struct {
	mongoURI, mongoDB, mode, temp, questionnaireTemp, legacy, questionnaireLegacy string
	timeout                                                                       time.Duration
}

type report struct {
	Heads, Snapshots, RetiredSnapshots, OrphanSnapshots                               int
	QuestionnaireHeads, QuestionnaireSnapshots, ActiveSnapshots, ActiveQuestionnaires int
	DroppedModelSnapshots, ArchivedModelActives, NormalizedModelHeads                 int
	DroppedQuestionnaireHeads, DroppedQuestionnaireSnapshots                          int
	ArchivedQuestionnaireActives, NormalizedQuestionnaireHeads                        int
	Issues                                                                            []string
	LegacyCollection                                                                  string
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fail("--mongo-uri is required (or set MONGO_URI)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fail("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fail("ping mongo: %v", err)
	}
	if err := run(ctx, client, client.Database(cfg.mongoDB), cfg, os.Stdout); err != nil {
		fail("unify assessment model records: %v", err)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.StringVar(&cfg.mode, "mode", "dry-run", "dry-run, apply, verify, cutover, or finalize")
	flag.StringVar(&cfg.temp, "temp-collection", defaultTemp, "temporary unified collection")
	flag.StringVar(&cfg.questionnaireTemp, "questionnaire-temp-collection", defaultQuestionnaireTemp, "temporary versioned questionnaire collection")
	flag.StringVar(&cfg.legacy, "legacy-collection", "", "legacy assessment_models collection name for finalize")
	flag.StringVar(&cfg.questionnaireLegacy, "legacy-questionnaire-collection", "", "legacy questionnaires collection name for finalize")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "overall timeout")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, client *mongo.Client, db *mongo.Database, cfg config, out *os.File) error {
	switch cfg.mode {
	case "dry-run":
		rep, err := audit(ctx, db, headCollection, publishedCollection)
		if err != nil {
			return err
		}
		printReport(out, rep)
		if len(rep.Issues) != 0 {
			return fmt.Errorf("audit found %d issue(s)", len(rep.Issues))
		}
		return nil
	case "apply":
		rep, err := audit(ctx, db, headCollection, publishedCollection)
		if err != nil {
			return err
		}
		if len(rep.Issues) != 0 {
			printReport(out, rep)
			return fmt.Errorf("refusing apply with %d issue(s)", len(rep.Issues))
		}
		if err := buildTemp(ctx, db, cfg.temp, cfg.questionnaireTemp); err != nil {
			return err
		}
		if err := verifyTemp(ctx, db, cfg.temp, cfg.questionnaireTemp, rep); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "prepared and verified %s\n", cfg.temp)
		return nil
	case "verify":
		rep, err := audit(ctx, db, headCollection, publishedCollection)
		if err != nil {
			return err
		}
		if err := verifyTemp(ctx, db, cfg.temp, cfg.questionnaireTemp, rep); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "verified %s\n", cfg.temp)
		return nil
	case "cutover":
		legacy, questionnaireLegacy, err := cutover(ctx, client, db, cfg.temp, cfg.questionnaireTemp)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "cutover complete; legacy collections: %s, %s\n", legacy, questionnaireLegacy)
		return nil
	case "finalize":
		if cfg.legacy == "" {
			return fmt.Errorf("--legacy-collection is required for finalize")
		}
		if cfg.questionnaireLegacy == "" {
			return fmt.Errorf("--legacy-questionnaire-collection is required for finalize")
		}
		if err := finalizeCanonicalFields(ctx, db); err != nil {
			return err
		}
		if err := dropIfExists(ctx, db, cfg.legacy); err != nil {
			return fmt.Errorf("drop %s: %w", cfg.legacy, err)
		}
		if err := dropIfExists(ctx, db, cfg.questionnaireLegacy); err != nil {
			return fmt.Errorf("drop %s: %w", cfg.questionnaireLegacy, err)
		}
		if err := dropIfExists(ctx, db, publishedCollection); err != nil {
			return fmt.Errorf("drop %s: %w", publishedCollection, err)
		}
		_, _ = fmt.Fprintln(out, "finalize complete")
		return nil
	default:
		return fmt.Errorf("unsupported --mode %q", cfg.mode)
	}
}

func finalizeCanonicalFields(ctx context.Context, db *mongo.Database) error {
	for _, collection := range []string{headCollection, questionnaireCollection} {
		if _, err := db.Collection(collection).UpdateMany(ctx, bson.M{"record_role": roleSnapshot}, bson.M{"$unset": bson.M{"is_active_published": ""}}); err != nil {
			return fmt.Errorf("remove legacy active boolean from %s: %w", collection, err)
		}
	}
	return nil
}

func audit(ctx context.Context, db *mongo.Database, headsName, snapshotsName string) (report, error) {
	allModels, err := loadAll(ctx, db.Collection(headsName), bson.M{})
	if err != nil {
		return report{}, fmt.Errorf("load assessment models: %w", err)
	}
	rawHeads := filterRows(allModels, func(row bson.M) bool {
		return stringField(row, "record_role") != roleSnapshot && row["deleted_at"] == nil
	})
	rawSnapshots := filterRows(allModels, func(row bson.M) bool { return stringField(row, "record_role") == roleSnapshot })
	if exists, existsErr := collectionExists(ctx, db, snapshotsName); existsErr != nil {
		return report{}, existsErr
	} else if exists {
		legacy, loadErr := loadAll(ctx, db.Collection(snapshotsName), bson.M{})
		if loadErr != nil {
			return report{}, fmt.Errorf("load snapshots: %w", loadErr)
		}
		rawSnapshots = append(rawSnapshots, legacy...)
	}
	models := prepareRunnableModelRecords(rawHeads, rawSnapshots)
	heads, snapshots := models.heads, models.snapshots
	rep := report{
		Heads: len(heads), Snapshots: len(snapshots), Issues: models.issues,
		DroppedModelSnapshots: models.droppedSnapshots,
		ArchivedModelActives:  models.archivedActives,
		NormalizedModelHeads:  models.normalizedHeads,
	}
	seenHeads := map[string]struct{}{}
	for _, head := range heads {
		code := stringField(head, "code")
		if code == "" {
			rep.Issues = append(rep.Issues, "head without code")
			continue
		}
		if _, ok := seenHeads[code]; ok {
			rep.Issues = append(rep.Issues, "duplicate head "+code)
		}
		seenHeads[code] = struct{}{}
	}
	modelInspection := inspectModelSnapshots(snapshots, seenHeads)
	rep.Issues = append(rep.Issues, modelInspection.issues...)
	rep.ActiveSnapshots = len(modelInspection.activeCodes)
	rep.RetiredSnapshots = modelInspection.retired
	rep.OrphanSnapshots = modelInspection.orphaned
	for _, head := range heads {
		code, status := stringField(head, "code"), stringField(head, "status")
		if code == "" {
			continue
		}
		_, active := modelInspection.activeCodes[code]
		if status == "published" && !active {
			rep.Issues = append(rep.Issues, "published model head without active snapshot "+code)
		}
		if status == "archived" && active {
			rep.Issues = append(rep.Issues, "archived model head with active snapshot "+code)
		}
	}
	questionnaires, err := loadAll(ctx, db.Collection(questionnaireCollection), bson.M{})
	if err != nil {
		return report{}, fmt.Errorf("load questionnaires: %w", err)
	}
	preparedQuestionnaires := prepareRunnableQuestionnaireRecords(questionnaires)
	questionnaireHeads, questionnaireSnapshots := preparedQuestionnaires.heads, preparedQuestionnaires.snapshots
	rep.Issues = append(rep.Issues, preparedQuestionnaires.issues...)
	rep.QuestionnaireHeads = len(questionnaireHeads)
	rep.QuestionnaireSnapshots = len(questionnaireSnapshots)
	rep.DroppedQuestionnaireHeads = preparedQuestionnaires.droppedHeads
	rep.DroppedQuestionnaireSnapshots = preparedQuestionnaires.droppedSnapshots
	rep.ArchivedQuestionnaireActives = preparedQuestionnaires.archivedActives
	rep.NormalizedQuestionnaireHeads = preparedQuestionnaires.normalizedHeads
	questionnaireInspection := inspectQuestionnaireSnapshots(questionnaireSnapshots)
	rep.Issues = append(rep.Issues, questionnaireInspection.issues...)
	rep.ActiveQuestionnaires = len(questionnaireInspection.activeCodes)
	for _, row := range snapshots {
		if !snapshotActive(row) {
			continue
		}
		questionnaireCode, questionnaireVersion := stringField(row, "questionnaire_code"), stringField(row, "questionnaire_version")
		if questionnaireCode == "" || questionnaireVersion == "" {
			continue
		}
		binding := questionnaireCode + "@" + questionnaireVersion
		if _, ok := questionnaireInspection.activeVersions[binding]; !ok {
			rep.Issues = append(rep.Issues, "active model references non-active questionnaire "+binding)
		}
	}
	rep.Issues = append(rep.Issues, inspectQuestionnaireHeads(questionnaireHeads, questionnaireInspection.activeCodes)...)
	sort.Strings(rep.Issues)
	return rep, nil
}

type modelRecordPreparation struct {
	heads, snapshots                  []bson.M
	droppedSnapshots, archivedActives int
	normalizedHeads                   int
	issues                            []string
}

func prepareRunnableModelRecords(heads, snapshots []bson.M) modelRecordPreparation {
	compatible := make([]bson.M, 0, len(snapshots))
	result := modelRecordPreparation{}
	result.heads = cloneRows(heads)
	for _, row := range snapshots {
		if !modelSnapshotCompatible(row) {
			result.droppedSnapshots++
			continue
		}
		compatible = append(compatible, row)
	}
	deduplicated, issues := deduplicateSnapshots(compatible)
	result.issues = append(result.issues, issues...)
	for _, row := range deduplicated {
		result.snapshots = append(result.snapshots, cloneBSON(row))
	}

	headByCode := make(map[string]bson.M, len(result.heads))
	for _, head := range result.heads {
		code := stringField(head, "code")
		if code != "" {
			headByCode[code] = head
		}
	}
	activeByCode := make(map[string][]bson.M)
	for _, row := range result.snapshots {
		if snapshotActive(row) {
			activeByCode[snapshotField(row, "code")] = append(activeByCode[snapshotField(row, "code")], row)
		}
	}
	for code, active := range activeByCode {
		head := headByCode[code]
		if head == nil || stringField(head, "status") == "archived" {
			for _, row := range active {
				archiveSnapshotForMigration(row, "active_without_runnable_head")
				result.archivedActives++
			}
			continue
		}
		if len(active) <= 1 {
			continue
		}
		matching := matchingModelActive(head, active)
		if len(matching) != 1 {
			result.issues = append(result.issues, "cannot determine current active snapshot for "+code)
			continue
		}
		for _, row := range active {
			if modelSnapshotIdentityKey(row) == modelSnapshotIdentityKey(matching[0]) {
				continue
			}
			archiveSnapshotForMigration(row, "superseded_active_snapshot")
			result.archivedActives++
		}
	}
	normalizePublishedModelHeads(result.heads, result.snapshots, &result.normalizedHeads)
	return result
}

func modelSnapshotIdentityKey(row bson.M) string {
	return strings.Join([]string{
		snapshotField(row, "kind"), snapshotField(row, "sub_kind"), snapshotField(row, "algorithm"),
		snapshotField(row, "code"), snapshotField(row, "version"),
	}, "|")
}

func modelSnapshotCompatible(row bson.M) bool {
	kind := domain.Kind(snapshotField(row, "kind"))
	switch kind {
	case domain.KindScale, domain.KindTypology, domain.KindCognitive, domain.KindBehavioralRating:
	default:
		return false
	}
	return snapshotField(row, "code") != "" &&
		snapshotField(row, "version") != "" &&
		len(bytesField(row, "payload")) != 0 &&
		stringField(row, "payload_format") != "" &&
		stringField(row, "decision_kind") != "" &&
		row["definition_v2"] != nil &&
		stringField(row, "questionnaire_code") != "" &&
		stringField(row, "questionnaire_version") != ""
}

func matchingModelActive(head bson.M, candidates []bson.M) []bson.M {
	result := make([]bson.M, 0, 1)
	for _, row := range candidates {
		if snapshotField(row, "kind") != stringField(head, "kind") ||
			snapshotField(row, "sub_kind") != stringField(head, "sub_kind") ||
			snapshotField(row, "algorithm") != stringField(head, "algorithm") ||
			stringField(row, "questionnaire_code") != stringField(head, "questionnaire_code") ||
			stringField(row, "questionnaire_version") != stringField(head, "questionnaire_version") {
			continue
		}
		result = append(result, row)
	}
	if len(result) != 0 {
		return result
	}
	// Some legacy heads predate the canonical identity fields. The exact
	// questionnaire binding still identifies the online release pair.
	for _, row := range candidates {
		if stringField(row, "questionnaire_code") == stringField(head, "questionnaire_code") &&
			stringField(row, "questionnaire_version") == stringField(head, "questionnaire_version") {
			result = append(result, row)
		}
	}
	return result
}

func normalizePublishedModelHeads(heads, snapshots []bson.M, normalized *int) {
	activeCodes := make(map[string]struct{})
	for _, row := range snapshots {
		if snapshotActive(row) {
			activeCodes[snapshotField(row, "code")] = struct{}{}
		}
	}
	for _, head := range heads {
		if stringField(head, "status") != "published" {
			continue
		}
		if _, ok := activeCodes[stringField(head, "code")]; ok {
			continue
		}
		head["status"] = "draft"
		head["migration_status_normalization"] = "published_without_runnable_snapshot"
		(*normalized)++
	}
}

type questionnaireRecordPreparation struct {
	heads, snapshots                 []bson.M
	droppedHeads, droppedSnapshots   int
	archivedActives, normalizedHeads int
	issues                           []string
}

func prepareRunnableQuestionnaireRecords(rows []bson.M) questionnaireRecordPreparation {
	result := questionnaireRecordPreparation{}
	for _, row := range rows {
		if stringField(row, "record_role") == roleSnapshot || row["deleted_at"] != nil {
			continue
		}
		if stringField(row, "code") == "" || stringField(row, "version") == "" {
			result.droppedHeads++
			continue
		}
		result.heads = append(result.heads, cloneBSON(row))
	}
	deduplicated, issues := deduplicateQuestionnaireSnapshots(questionnaireSnapshotSources(rows))
	result.issues = append(result.issues, issues...)
	for _, row := range deduplicated {
		if stringField(row, "code") == "" || stringField(row, "version") == "" {
			result.droppedSnapshots++
			continue
		}
		result.snapshots = append(result.snapshots, cloneBSON(row))
	}

	headByCode := make(map[string]bson.M, len(result.heads))
	for _, head := range result.heads {
		headByCode[stringField(head, "code")] = head
	}
	activeByCode := make(map[string][]bson.M)
	for _, row := range result.snapshots {
		if snapshotActive(row) {
			activeByCode[stringField(row, "code")] = append(activeByCode[stringField(row, "code")], row)
		}
	}
	for code, active := range activeByCode {
		head := headByCode[code]
		if head == nil || stringField(head, "status") == "archived" {
			for _, row := range active {
				archiveSnapshotForMigration(row, "active_without_runnable_head")
				result.archivedActives++
			}
			continue
		}
		if len(active) <= 1 {
			continue
		}
		matching := make([]bson.M, 0, 1)
		for _, row := range active {
			if stringField(row, "version") == stringField(head, "version") {
				matching = append(matching, row)
			}
		}
		if len(matching) != 1 {
			result.issues = append(result.issues, "cannot determine current active questionnaire snapshot for "+code)
			continue
		}
		for _, row := range active {
			if stringField(row, "version") == stringField(matching[0], "version") {
				continue
			}
			archiveSnapshotForMigration(row, "superseded_active_snapshot")
			result.archivedActives++
		}
	}
	normalizePublishedQuestionnaireHeads(result.heads, result.snapshots, &result.normalizedHeads)
	return result
}

func normalizePublishedQuestionnaireHeads(heads, snapshots []bson.M, normalized *int) {
	activeCodes := make(map[string]struct{})
	for _, row := range snapshots {
		if snapshotActive(row) {
			activeCodes[stringField(row, "code")] = struct{}{}
		}
	}
	for _, head := range heads {
		if stringField(head, "status") != "published" {
			continue
		}
		if _, ok := activeCodes[stringField(head, "code")]; ok {
			continue
		}
		head["status"] = "draft"
		head["migration_status_normalization"] = "published_without_runnable_snapshot"
		(*normalized)++
	}
}

func archiveSnapshotForMigration(row bson.M, reason string) {
	row["release_status"] = "archived"
	row["is_active_published"] = false
	if row["release_archived_at"] == nil {
		row["release_archived_at"] = time.Now().UTC()
	}
	row["migration_release_normalization"] = reason
}

func cloneRows(rows []bson.M) []bson.M {
	result := make([]bson.M, 0, len(rows))
	for _, row := range rows {
		result = append(result, cloneBSON(row))
	}
	return result
}

type modelSnapshotInspection struct {
	issues      []string
	activeCodes map[string]struct{}
	retired     int
	orphaned    int
}

func inspectModelSnapshots(snapshots []bson.M, headCodes map[string]struct{}) modelSnapshotInspection {
	result := modelSnapshotInspection{activeCodes: map[string]struct{}{}}
	seenRelease := map[string]struct{}{}
	seenActiveQuestionnaire := map[string]struct{}{}
	for _, row := range snapshots {
		code, kind, version := snapshotField(row, "code"), snapshotField(row, "kind"), snapshotField(row, "version")
		active := snapshotActive(row)
		if active {
			if code != "" {
				if _, ok := result.activeCodes[code]; ok {
					result.issues = append(result.issues, "multiple active snapshots for "+code)
				}
				result.activeCodes[code] = struct{}{}
			}
			questionnaireCode, questionnaireVersion := stringField(row, "questionnaire_code"), stringField(row, "questionnaire_version")
			if questionnaireCode == "" || questionnaireVersion == "" {
				result.issues = append(result.issues, fmt.Sprintf("active snapshot without questionnaire binding %s code=%q version=%q", code, questionnaireCode, questionnaireVersion))
			} else {
				bindingKey := questionnaireCode + "@" + questionnaireVersion
				if _, ok := seenActiveQuestionnaire[bindingKey]; ok {
					result.issues = append(result.issues, "multiple active snapshots for questionnaire "+bindingKey)
				} else {
					seenActiveQuestionnaire[bindingKey] = struct{}{}
				}
			}
		} else {
			result.retired++
		}
		if code != "" {
			if _, ok := headCodes[code]; !ok {
				result.orphaned++
				if active {
					result.issues = append(result.issues, fmt.Sprintf("active orphan snapshot %s@%s", code, version))
				}
			}
		}

		payload := bytesField(row, "payload")
		if code == "" || kind == "" || version == "" || len(payload) == 0 {
			result.issues = append(result.issues, fmt.Sprintf("invalid published snapshot code=%q kind=%q version=%q payload_type=%T", code, kind, version, row["payload"]))
			continue
		}
		missing := make([]string, 0, 3)
		if stringField(row, "payload_format") == "" {
			missing = append(missing, "payload_format")
		}
		if stringField(row, "decision_kind") == "" {
			missing = append(missing, "decision_kind")
		}
		if row["definition_v2"] == nil {
			missing = append(missing, "definition_v2")
		}
		if len(missing) != 0 {
			result.issues = append(result.issues, fmt.Sprintf("incomplete published snapshot %s@%s missing=%s", code, version, strings.Join(missing, ",")))
		}
		key := strings.Join([]string{kind, snapshotField(row, "sub_kind"), snapshotField(row, "algorithm"), code, version}, "|")
		if _, ok := seenRelease[key]; ok {
			result.issues = append(result.issues, "duplicate published release "+key)
		}
		seenRelease[key] = struct{}{}
	}
	return result
}

type questionnaireSnapshotInspection struct {
	issues         []string
	activeCodes    map[string]struct{}
	activeVersions map[string]struct{}
}

func inspectQuestionnaireSnapshots(snapshots []bson.M) questionnaireSnapshotInspection {
	result := questionnaireSnapshotInspection{
		activeCodes:    map[string]struct{}{},
		activeVersions: map[string]struct{}{},
	}
	for _, row := range snapshots {
		code, version := stringField(row, "code"), stringField(row, "version")
		if code == "" || version == "" {
			result.issues = append(result.issues, fmt.Sprintf("questionnaire snapshot without identity code=%q version=%q", code, version))
		}
		if !snapshotActive(row) || code == "" {
			continue
		}
		if _, ok := result.activeCodes[code]; ok {
			result.issues = append(result.issues, "multiple active questionnaire snapshots for "+code)
		}
		result.activeCodes[code] = struct{}{}
		if version != "" {
			result.activeVersions[code+"@"+version] = struct{}{}
		}
	}
	return result
}

func inspectQuestionnaireHeads(heads []bson.M, activeCodes map[string]struct{}) []string {
	issues := make([]string, 0)
	headCodes := make(map[string]struct{}, len(heads))
	for _, head := range heads {
		code, version, status := stringField(head, "code"), stringField(head, "version"), stringField(head, "status")
		if code == "" || version == "" {
			issues = append(issues, fmt.Sprintf("questionnaire head without identity code=%q version=%q", code, version))
		}
		if code == "" {
			continue
		}
		headCodes[code] = struct{}{}
		_, active := activeCodes[code]
		if status == "published" && !active {
			issues = append(issues, "published questionnaire head without active snapshot "+code)
		}
		if status == "archived" && active {
			issues = append(issues, "archived questionnaire head with active snapshot "+code)
		}
	}
	for code := range activeCodes {
		if _, ok := headCodes[code]; !ok {
			issues = append(issues, "active questionnaire snapshot without head "+code)
		}
	}
	return issues
}

func buildTemp(ctx context.Context, db *mongo.Database, temp, questionnaireTemp string) error {
	if exists, err := collectionExists(ctx, db, temp); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("temporary collection %s already exists", temp)
	}
	if exists, err := collectionExists(ctx, db, questionnaireTemp); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("temporary collection %s already exists", questionnaireTemp)
	}
	target := db.Collection(temp)
	allModels, err := loadAll(ctx, db.Collection(headCollection), bson.M{})
	if err != nil {
		return err
	}
	rawHeads := filterRows(allModels, func(row bson.M) bool {
		return stringField(row, "record_role") != roleSnapshot && row["deleted_at"] == nil
	})
	rawSnapshots := filterRows(allModels, func(row bson.M) bool { return stringField(row, "record_role") == roleSnapshot })
	if exists, existsErr := collectionExists(ctx, db, publishedCollection); existsErr != nil {
		return existsErr
	} else if exists {
		legacy, loadErr := loadAll(ctx, db.Collection(publishedCollection), bson.M{})
		if loadErr != nil {
			return loadErr
		}
		rawSnapshots = append(rawSnapshots, legacy...)
	}
	models := prepareRunnableModelRecords(rawHeads, rawSnapshots)
	if len(models.issues) != 0 {
		return fmt.Errorf("model source conflict: %s", strings.Join(models.issues, "; "))
	}
	for _, row := range models.heads {
		row["record_role"] = roleHead
		row["is_active_published"] = false
		delete(row, "release_status")
		delete(row, "release_archived_at")
		if _, ok := row["revision"]; !ok {
			row["revision"] = row["version"]
			delete(row, "version")
		}
		if _, err := target.InsertOne(ctx, row); err != nil {
			return fmt.Errorf("insert head %s: %w", stringField(row, "code"), err)
		}
	}
	for _, row := range models.snapshots {
		converted := convertSnapshot(row)
		if _, err := target.InsertOne(ctx, converted); err != nil {
			return fmt.Errorf("insert snapshot %s@%s: %w", stringField(converted, "code"), stringField(converted, "release_version"), err)
		}
	}
	if err := createIndexes(ctx, target); err != nil {
		return err
	}
	questionnaireTarget := db.Collection(questionnaireTemp)
	questionnaires, err := loadAll(ctx, db.Collection(questionnaireCollection), bson.M{})
	if err != nil {
		return err
	}
	preparedQuestionnaires := prepareRunnableQuestionnaireRecords(questionnaires)
	if len(preparedQuestionnaires.issues) != 0 {
		return fmt.Errorf("questionnaire source conflict: %s", strings.Join(preparedQuestionnaires.issues, "; "))
	}
	for _, row := range preparedQuestionnaires.heads {
		converted := convertQuestionnaireRecord(row)
		if _, err := questionnaireTarget.InsertOne(ctx, converted); err != nil {
			return fmt.Errorf("insert questionnaire %s@%s: %w", stringField(converted, "code"), stringField(converted, "version"), err)
		}
	}
	for _, row := range preparedQuestionnaires.snapshots {
		source := cloneBSON(row)
		source["record_role"] = roleSnapshot
		converted := convertQuestionnaireRecord(source)
		converted["legacy_source_id"] = legacySourceID(row["_id"])
		if stringField(row, "record_role") != roleSnapshot {
			converted["_id"] = primitive.NewObjectID()
		}
		if _, err := questionnaireTarget.InsertOne(ctx, converted); err != nil {
			return fmt.Errorf("insert questionnaire snapshot %s@%s: %w", stringField(converted, "code"), stringField(converted, "version"), err)
		}
	}
	return createQuestionnaireIndexes(ctx, questionnaireTarget)
}

func convertSnapshot(row bson.M) bson.M {
	converted := bson.M{}
	for key, value := range row {
		converted[key] = value
	}
	converted["legacy_source_id"] = legacySourceID(row["_id"])
	converted["legacy_source_collection"] = headCollection
	if stringField(row, "model_code") != "" {
		converted["legacy_source_collection"] = publishedCollection
	}
	converted["_id"] = primitive.NewObjectID()
	converted["record_role"] = roleSnapshot
	converted["status"] = "published"
	if converted["published_at"] == nil {
		converted["published_at"] = firstTime(row["updated_at"], row["created_at"], time.Now().UTC())
	}
	active := snapshotActive(row)
	converted["is_active_published"] = active
	converted["release_status"] = "archived"
	if active {
		converted["release_status"] = "active"
		delete(converted, "release_archived_at")
	} else if converted["release_archived_at"] == nil {
		converted["release_archived_at"] = time.Now().UTC()
	}
	converted["product_channel"] = snapshotField(row, "product_channel")
	converted["kind"] = snapshotField(row, "kind")
	converted["sub_kind"] = snapshotField(row, "sub_kind")
	converted["algorithm"] = snapshotField(row, "algorithm")
	converted["code"] = snapshotField(row, "code")
	converted["release_version"] = snapshotField(row, "version")
	if row["deleted_at"] != nil {
		// Historical soft-deleted rows remain queryable by their exact release
		// after cutover, but cannot be selected by any active runtime query.
		converted["legacy_deleted_at"] = row["deleted_at"]
		converted["retention_state"] = "legacy_soft_deleted"
		converted["deleted_at"] = nil
	}
	for _, key := range []string{"model_product_channel", "model_kind", "model_sub_kind", "model_algorithm", "model_code", "model_version"} {
		delete(converted, key)
	}
	return converted
}

func verifyTemp(ctx context.Context, db *mongo.Database, temp, questionnaireTemp string, source report) error {
	rows, err := loadAll(ctx, db.Collection(temp), bson.M{})
	if err != nil {
		return err
	}
	var heads, snapshots, retired, orphaned int
	seenActive, seenQuestionnaire := map[string]struct{}{}, map[string]struct{}{}
	for _, row := range rows {
		switch stringField(row, "record_role") {
		case roleHead:
			heads++
		case roleSnapshot:
			snapshots++
			if len(bytesField(row, "payload")) == 0 || stringField(row, "release_version") == "" || row["definition_v2"] == nil {
				return fmt.Errorf("invalid converted snapshot %s", stringField(row, "code"))
			}
			if err := verifyRuntimeDecodable(ctx, db, row); err != nil {
				return fmt.Errorf("runtime decode %s@%s: %w", stringField(row, "code"), stringField(row, "release_version"), err)
			}
			if err := verifySnapshotSource(ctx, db, row); err != nil {
				return err
			}
			if snapshotActive(row) {
				code := stringField(row, "code")
				if _, ok := seenActive[code]; ok {
					return fmt.Errorf("multiple active converted snapshots for %s", code)
				}
				seenActive[code] = struct{}{}
				binding := stringField(row, "questionnaire_code") + "@" + stringField(row, "questionnaire_version")
				if _, ok := seenQuestionnaire[binding]; ok {
					return fmt.Errorf("multiple active converted snapshots for questionnaire %s", binding)
				}
				seenQuestionnaire[binding] = struct{}{}
			} else {
				retired++
			}
			if stringField(row, "legacy_source_id") != "" {
				// Orphan count is informational: retained snapshots remain runnable,
				// while missing heads are repaired only by this one-off tool.
				var head bson.M
				err := db.Collection(temp).FindOne(ctx, bson.M{"record_role": roleHead, "code": stringField(row, "code")}).Decode(&head)
				if err == mongo.ErrNoDocuments {
					orphaned++
				} else if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown record_role %q", stringField(row, "record_role"))
		}
	}
	if heads != source.Heads || snapshots != source.Snapshots {
		return fmt.Errorf("temporary count mismatch heads=%d/%d snapshots=%d/%d", heads, source.Heads, snapshots, source.Snapshots)
	}
	if len(seenActive) != source.ActiveSnapshots {
		return fmt.Errorf("temporary active model mismatch active=%d/%d", len(seenActive), source.ActiveSnapshots)
	}
	if retired != source.RetiredSnapshots || orphaned != source.OrphanSnapshots {
		return fmt.Errorf("temporary retained/orphan mismatch retained=%d/%d orphaned=%d/%d", retired, source.RetiredSnapshots, orphaned, source.OrphanSnapshots)
	}
	questionnaires, err := loadAll(ctx, db.Collection(questionnaireTemp), bson.M{})
	if err != nil {
		return err
	}
	var questionnaireHeads, questionnaireSnapshots int
	activeQuestionnaires := map[string]struct{}{}
	for _, row := range questionnaires {
		if stringField(row, "record_role") == roleSnapshot {
			questionnaireSnapshots++
			if snapshotActive(row) {
				key := stringField(row, "code") + "@" + stringField(row, "version")
				activeQuestionnaires[key] = struct{}{}
			}
		} else if stringField(row, "record_role") == roleHead {
			questionnaireHeads++
		} else {
			return fmt.Errorf("unknown questionnaire record_role %q", stringField(row, "record_role"))
		}
	}
	if questionnaireHeads != source.QuestionnaireHeads || questionnaireSnapshots != source.QuestionnaireSnapshots {
		return fmt.Errorf("temporary questionnaire count mismatch heads=%d/%d snapshots=%d/%d", questionnaireHeads, source.QuestionnaireHeads, questionnaireSnapshots, source.QuestionnaireSnapshots)
	}
	if len(activeQuestionnaires) != source.ActiveQuestionnaires {
		return fmt.Errorf("temporary active questionnaire mismatch active=%d/%d", len(activeQuestionnaires), source.ActiveQuestionnaires)
	}
	for _, row := range rows {
		if stringField(row, "record_role") != roleSnapshot || !snapshotActive(row) {
			continue
		}
		binding := stringField(row, "questionnaire_code") + "@" + stringField(row, "questionnaire_version")
		if _, ok := activeQuestionnaires[binding]; !ok {
			return fmt.Errorf("active model references non-active questionnaire %s", binding)
		}
	}
	return nil
}

func verifyRuntimeDecodable(ctx context.Context, db *mongo.Database, row bson.M) error {
	data, err := bson.Marshal(row)
	if err != nil {
		return err
	}
	var po mongomodelcatalog.PublishedAssessmentModelPO
	if err := bson.Unmarshal(data, &po); err != nil {
		return err
	}
	model := mongomodelcatalog.NewMapper().ToPublished(&po)
	if model == nil || model.DefinitionV2 == nil {
		return fmt.Errorf("definition_v2 is required")
	}
	switch model.Kind {
	case domain.KindScale:
		_, err = inframodelcatalog.DecodeScaleFromPublished(model)
	case domain.KindTypology:
		var payload *typologypayload.Payload
		payload, err = typologypayload.PayloadFromDefinition(typologypayload.DefinitionEnvelope{
			Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status, Algorithm: model.Algorithm,
		}, model.DefinitionV2)
		if err == nil && (payload == nil || !payload.IsPublished()) {
			err = fmt.Errorf("typology definition did not produce a published payload")
		}
	case domain.KindCognitive:
		var snapshot *cognitivepayload.Snapshot
		snapshot, err = cognitivepayload.SnapshotFromDefinition(cognitivepayload.DefinitionEnvelope{
			Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status,
		}, model.DefinitionV2)
		if err == nil && (snapshot == nil || !snapshot.IsPublished()) {
			err = fmt.Errorf("cognitive definition did not produce a published snapshot")
		}
	case domain.KindBehavioralRating:
		tables := make(map[string]*domain.Norm, len(model.DefinitionV2.Calibration.NormRefs))
		norms := mongomodelcatalog.NewNormRepository(db)
		for _, ref := range model.DefinitionV2.Calibration.NormRefs {
			if ref.NormTableVersion == "" {
				continue
			}
			table, loadErr := norms.FindNorm(ctx, ref.NormTableVersion)
			if loadErr != nil {
				return fmt.Errorf("load norm %s: %w", ref.NormTableVersion, loadErr)
			}
			tables[ref.NormTableVersion] = table
		}
		_, err = behavioralpayload.SnapshotFromDefinition(behavioralpayload.DefinitionEnvelope{
			Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status,
		}, model.DefinitionV2, tables)
	default:
		err = fmt.Errorf("unsupported model kind %q", model.Kind)
	}
	return err
}

func verifySnapshotSource(ctx context.Context, db *mongo.Database, row bson.M) error {
	id, ok := row["legacy_source_id"].(string)
	if !ok || id == "" {
		return fmt.Errorf("converted snapshot %s is missing legacy_source_id", stringField(row, "code"))
	}
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("converted snapshot %s has invalid legacy_source_id: %w", stringField(row, "code"), err)
	}
	var source bson.M
	sourceCollection := stringField(row, "legacy_source_collection")
	if sourceCollection == "" {
		sourceCollection = publishedCollection
	}
	if err := db.Collection(sourceCollection).FindOne(ctx, bson.M{"_id": objectID}).Decode(&source); err != nil {
		return fmt.Errorf("load legacy source %s: %w", id, err)
	}
	if payloadHash(bytesField(source, "payload")) != payloadHash(bytesField(row, "payload")) {
		return fmt.Errorf("payload hash mismatch for %s@%s", stringField(row, "code"), stringField(row, "release_version"))
	}
	if snapshotField(source, "code") != stringField(row, "code") || snapshotField(source, "version") != stringField(row, "release_version") || stringField(source, "questionnaire_code") != stringField(row, "questionnaire_code") || stringField(source, "questionnaire_version") != stringField(row, "questionnaire_version") {
		return fmt.Errorf("identity or questionnaire binding mismatch for %s", id)
	}
	return nil
}

func createIndexes(ctx context.Context, c *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_head_code").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleHead, "deleted_at": nil})},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "sub_kind", Value: 1}, {Key: "algorithm", Value: 1}, {Key: "code", Value: 1}, {Key: "release_version", Value: 1}, {Key: "record_role", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_snapshot_identity_version").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "deleted_at": nil})},
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_active_code").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "release_status": "active", "deleted_at": nil})},
		{Keys: bson.D{{Key: "questionnaire_code", Value: 1}, {Key: "questionnaire_version", Value: 1}, {Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_active_questionnaire").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "release_status": "active", "deleted_at": nil})},
		{Keys: bson.D{{Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}, {Key: "status", Value: 1}, {Key: "kind", Value: 1}, {Key: "category", Value: 1}, {Key: "algorithm", Value: 1}, {Key: "code", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_active_catalog")},
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "published_at", Value: -1}}, Options: options.Index().SetName("idx_assessment_models_release_history")},
	}
	_, err := c.Indexes().CreateMany(ctx, indexes)
	return err
}

func createQuestionnaireIndexes(ctx context.Context, c *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}}, Options: options.Index().SetName("idx_questionnaires_head_code").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleHead, "deleted_at": nil})},
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "version", Value: 1}, {Key: "record_role", Value: 1}}, Options: options.Index().SetName("idx_questionnaires_snapshot_version").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "deleted_at": nil})},
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}}, Options: options.Index().SetName("idx_questionnaires_active_code").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "release_status": "active", "deleted_at": nil})},
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "published_at", Value: -1}}, Options: options.Index().SetName("idx_questionnaires_release_history")},
	}
	_, err := c.Indexes().CreateMany(ctx, indexes)
	return err
}

func cutover(ctx context.Context, client *mongo.Client, db *mongo.Database, temp, questionnaireTemp string) (string, string, error) {
	if ok, err := collectionExists(ctx, db, temp); err != nil || !ok {
		if err != nil {
			return "", "", err
		}
		return "", "", fmt.Errorf("temporary collection %s does not exist", temp)
	}
	if ok, err := collectionExists(ctx, db, questionnaireTemp); err != nil || !ok {
		if err != nil {
			return "", "", err
		}
		return "", "", fmt.Errorf("temporary collection %s does not exist", questionnaireTemp)
	}
	suffix := time.Now().UTC().Format("20060102_150405")
	legacy := headCollection + "_legacy_" + suffix
	questionnaireLegacy := questionnaireCollection + "_legacy_" + suffix
	if err := renameCollection(ctx, client, db.Name(), headCollection, legacy); err != nil {
		return "", "", err
	}
	if err := renameCollection(ctx, client, db.Name(), temp, headCollection); err != nil {
		_ = renameCollection(ctx, client, db.Name(), legacy, headCollection)
		return "", "", err
	}
	if err := renameCollection(ctx, client, db.Name(), questionnaireCollection, questionnaireLegacy); err != nil {
		_ = renameCollection(ctx, client, db.Name(), headCollection, temp)
		_ = renameCollection(ctx, client, db.Name(), legacy, headCollection)
		return "", "", err
	}
	if err := renameCollection(ctx, client, db.Name(), questionnaireTemp, questionnaireCollection); err != nil {
		_ = renameCollection(ctx, client, db.Name(), questionnaireLegacy, questionnaireCollection)
		_ = renameCollection(ctx, client, db.Name(), headCollection, temp)
		_ = renameCollection(ctx, client, db.Name(), legacy, headCollection)
		return "", "", err
	}
	return legacy, questionnaireLegacy, nil
}

func renameCollection(ctx context.Context, client *mongo.Client, dbName, from, to string) error {
	result := client.Database("admin").RunCommand(ctx, bson.D{{Key: "renameCollection", Value: dbName + "." + from}, {Key: "to", Value: dbName + "." + to}})
	return result.Err()
}

func collectionExists(ctx context.Context, db *mongo.Database, name string) (bool, error) {
	items, err := db.ListCollectionNames(ctx, bson.M{"name": name})
	return len(items) != 0, err
}

func dropIfExists(ctx context.Context, db *mongo.Database, name string) error {
	exists, err := collectionExists(ctx, db, name)
	if err != nil || !exists {
		return err
	}
	return db.Collection(name).Drop(ctx)
}

func convertQuestionnaireRecord(row bson.M) bson.M {
	converted := bson.M{}
	for key, value := range row {
		converted[key] = value
	}
	role := stringField(row, "record_role")
	if role != roleSnapshot {
		converted["record_role"] = roleHead
		converted["is_active_published"] = false
		delete(converted, "release_status")
		delete(converted, "release_archived_at")
		delete(converted, "published_at")
		return converted
	}
	active := snapshotActive(row)
	converted["record_role"] = roleSnapshot
	converted["status"] = "published"
	if converted["published_at"] == nil {
		converted["published_at"] = firstTime(row["updated_at"], row["created_at"], time.Now().UTC())
	}
	converted["is_active_published"] = active
	converted["release_status"] = "archived"
	if active {
		converted["release_status"] = "active"
		delete(converted, "release_archived_at")
	} else if converted["release_archived_at"] == nil {
		converted["release_archived_at"] = time.Now().UTC()
	}
	if row["deleted_at"] != nil {
		converted["legacy_deleted_at"] = row["deleted_at"]
		converted["deleted_at"] = nil
	}
	return converted
}

func snapshotActive(row bson.M) bool {
	if status := stringField(row, "release_status"); status != "" {
		return status == "active"
	}
	if active, ok := row["is_active_published"].(bool); ok {
		return active
	}
	return row["deleted_at"] == nil && stringField(row, "status") == "published"
}

func snapshotField(row bson.M, field string) string {
	legacyKey := map[string]string{
		"code": "model_code", "kind": "model_kind", "sub_kind": "model_sub_kind",
		"algorithm": "model_algorithm", "product_channel": "model_product_channel", "version": "model_version",
	}[field]
	if legacyKey != "" {
		if value := stringField(row, legacyKey); value != "" {
			return value
		}
	}
	if field == "version" {
		return stringField(row, "release_version")
	}
	return stringField(row, field)
}

func filterRows(rows []bson.M, keep func(bson.M) bool) []bson.M {
	result := make([]bson.M, 0, len(rows))
	for _, row := range rows {
		if keep(row) {
			result = append(result, row)
		}
	}
	return result
}

func deduplicateSnapshots(rows []bson.M) ([]bson.M, []string) {
	result := make([]bson.M, 0, len(rows))
	indexes := map[string]int{}
	issues := make([]string, 0)
	for _, row := range rows {
		kind, code, version := snapshotField(row, "kind"), snapshotField(row, "code"), snapshotField(row, "version")
		if kind == "" || code == "" || version == "" {
			result = append(result, row)
			continue
		}
		key := strings.Join([]string{kind, snapshotField(row, "sub_kind"), snapshotField(row, "algorithm"), code, version}, "|")
		if index, ok := indexes[key]; ok {
			existing := result[index]
			same := payloadHash(bytesField(existing, "payload")) == payloadHash(bytesField(row, "payload")) &&
				stringField(existing, "questionnaire_code") == stringField(row, "questionnaire_code") &&
				stringField(existing, "questionnaire_version") == stringField(row, "questionnaire_version") &&
				reflect.DeepEqual(existing["definition_v2"], row["definition_v2"])
			if !same {
				issues = append(issues, "conflicting duplicate published release "+key)
				continue
			}
			if snapshotActive(row) && !snapshotActive(existing) {
				result[index] = row
			}
			continue
		}
		indexes[key] = len(result)
		result = append(result, row)
	}
	return result, issues
}

func questionnaireSnapshotSources(rows []bson.M) []bson.M {
	return filterRows(rows, func(row bson.M) bool {
		return stringField(row, "record_role") == roleSnapshot ||
			(stringField(row, "record_role") != roleHead && stringField(row, "status") == "published")
	})
}

func deduplicateQuestionnaireSnapshots(rows []bson.M) ([]bson.M, []string) {
	result := make([]bson.M, 0, len(rows))
	indexes := map[string]int{}
	issues := make([]string, 0)
	for _, row := range rows {
		code, version := stringField(row, "code"), stringField(row, "version")
		if code == "" || version == "" {
			result = append(result, row)
			continue
		}
		key := code + "@" + version
		if index, ok := indexes[key]; ok {
			existing := result[index]
			if !sameQuestionnaireReleaseContent(existing, row) {
				issues = append(issues, "conflicting duplicate questionnaire release "+key)
				continue
			}
			if snapshotActive(row) && !snapshotActive(existing) {
				result[index] = row
			}
			continue
		}
		indexes[key] = len(result)
		result = append(result, row)
	}
	return result, issues
}

func sameQuestionnaireReleaseContent(a, b bson.M) bool {
	for _, field := range []string{"code", "version", "title", "description", "img_url", "type", "questions"} {
		if !reflect.DeepEqual(a[field], b[field]) {
			return false
		}
	}
	return true
}

func cloneBSON(row bson.M) bson.M {
	copy := make(bson.M, len(row))
	for key, value := range row {
		copy[key] = value
	}
	return copy
}

func firstTime(values ...any) time.Time {
	for _, value := range values {
		if at, ok := value.(time.Time); ok && !at.IsZero() {
			return at.UTC()
		}
	}
	return time.Now().UTC()
}

func loadAll(ctx context.Context, c *mongo.Collection, filter bson.M) ([]bson.M, error) {
	cursor, err := c.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]bson.M, 0)
	for cursor.Next(ctx) {
		var row bson.M
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, cursor.Err()
}

func stringField(row bson.M, key string) string { value, _ := row[key].(string); return value }
func bytesField(row bson.M, key string) []byte {
	switch value := row[key].(type) {
	case []byte:
		return value
	case primitive.Binary:
		return value.Data
	default:
		return nil
	}
}
func legacySourceID(value any) string {
	if id, ok := value.(primitive.ObjectID); ok {
		return id.Hex()
	}
	return fmt.Sprint(value)
}
func payloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func printReport(out *os.File, rep report) {
	_, _ = fmt.Fprintf(out, "model_heads=%d model_snapshots=%d active_models=%d retired_snapshots=%d orphan_snapshots=%d questionnaire_heads=%d questionnaire_snapshots=%d active_questionnaires=%d issues=%d\n", rep.Heads, rep.Snapshots, rep.ActiveSnapshots, rep.RetiredSnapshots, rep.OrphanSnapshots, rep.QuestionnaireHeads, rep.QuestionnaireSnapshots, rep.ActiveQuestionnaires, len(rep.Issues))
	_, _ = fmt.Fprintf(out, "skipped_model_snapshots=%d archived_model_actives=%d normalized_model_heads=%d skipped_questionnaire_heads=%d skipped_questionnaire_snapshots=%d archived_questionnaire_actives=%d normalized_questionnaire_heads=%d\n", rep.DroppedModelSnapshots, rep.ArchivedModelActives, rep.NormalizedModelHeads, rep.DroppedQuestionnaireHeads, rep.DroppedQuestionnaireSnapshots, rep.ArchivedQuestionnaireActives, rep.NormalizedQuestionnaireHeads)
	for _, issue := range rep.Issues {
		_, _ = fmt.Fprintln(out, "-", issue)
	}
}
func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
