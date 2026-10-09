//go:build integration

package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func requireNative(t *testing.T) {
	t.Helper()
	if os.Getenv("QS_HISTORY_CLI_LOCAL_INTEGRATION") != "1" {
		t.Skip("owned CLI native fixtures not selected")
	}
	if os.Getenv("MYSQL_HOST") != "127.0.0.1" || os.Getenv("MYSQL_PORT") != "34306" || os.Getenv("MONGODB_HOST") != "127.0.0.1" || os.Getenv("QS_HISTORY_FIXTURE_OWNERSHIP_VERIFIED") != "1" || !hashPattern.MatchString(os.Getenv("QS_HISTORY_MONGO_FIXTURE_ID")) || os.Getenv("QS_HISTORY_MONGO_REPLICA_SET") != "qs_history_cli_native" {
		t.Fatal("independently verified owned loopback fixtures required")
	}
}
func nativeToken(t *testing.T) string {
	t.Helper()
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("owned fixture allocation failed")
	}
	return hex.EncodeToString(b[:])
}
func framedParts(parts ...string) string {
	var raw []byte
	for _, part := range parts {
		var frame [9]byte
		frame[0] = 1
		binary.BigEndian.PutUint64(frame[1:], uint64(len(part)))
		raw = append(raw, frame[:]...)
		raw = append(raw, []byte(part)...)
	}
	return rawHash(raw)
}
func nativeFixture(t *testing.T, nonempty bool) (*sql.DB, *mongo.Client, *mongo.Database, string) {
	t.Helper()
	requireNative(t)
	token := nativeToken(t)
	namespace := "qs_history_cli_test_" + token
	c := drivermysql.NewConfig()
	c.User = os.Getenv("MYSQL_USERNAME")
	c.Passwd = os.Getenv("MYSQL_PASSWORD")
	c.Net = "tcp"
	c.Addr = "127.0.0.1:34306"
	c.ParseTime = true
	c.Loc = time.UTC
	c.MultiStatements = true
	admin, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		t.Fatal("owned SQL connect failed")
	}
	if _, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+namespace+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatal("owned SQL namespace create failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, e := admin.ExecContext(ctx, "DROP DATABASE `"+namespace+"`"); e != nil {
			t.Error("owned SQL namespace cleanup failed")
		}
		var n int
		queryErr := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?", namespace).Scan(&n)
		if queryErr != nil || n != 0 {
			t.Error("owned SQL namespace remaining")
		}
		if admin.Close() != nil {
			t.Error("owned SQL admin close failed")
		}
		if n == 0 && queryErr == nil {
			t.Log("owned_sql_namespace_cleanup_zero")
		}
	})
	c.DBName = namespace
	pool, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		t.Fatal("owned SQL pool create failed")
	}
	t.Cleanup(func() {
		if pool.Close() != nil {
			t.Error("owned SQL pool close failed")
		}
	})
	v, _, err := migration.NewMigrator(pool, &migration.Config{Enabled: true, Database: namespace}).Run()
	if err != nil || v != 99 {
		t.Fatal("actual additive99 fixture migration failed")
	}
	mongoPort, e := strconv.Atoi(os.Getenv("MONGODB_PORT"))
	if e != nil || mongoPort < 1024 || mongoPort > 65535 || mongoPort == 33317 {
		t.Fatal("independently owned Mongo port required")
	}
	adminOptions := options.Client().SetHosts([]string{"127.0.0.1:" + strconv.Itoa(mongoPort)}).SetReplicaSet("qs_history_cli_native").SetAuth(options.Credential{Username: os.Getenv("QS_HISTORY_MONGO_ADMIN_USERNAME"), Password: os.Getenv("QS_HISTORY_MONGO_ADMIN_PASSWORD"), AuthSource: "admin"}).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second)
	adminMongo, err := mongo.Connect(t.Context(), adminOptions)
	if err != nil || adminMongo.Ping(t.Context(), readpref.Primary()) != nil {
		t.Fatal("owned Mongo connect failed")
	}
	db := adminMongo.Database(namespace)
	role, user, password := "qs_history_cli_role_"+token, "qs_history_cli_user_"+token, nativeToken(t)+nativeToken(t)
	roleMade, userMade := false, false
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cleanupVerified := true
		if userMade && adminMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "dropUser", Value: user}}).Err() != nil {
			cleanupVerified = false
			t.Error("owned Mongo user cleanup failed")
		}
		if roleMade && adminMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "dropRole", Value: role}}).Err() != nil {
			cleanupVerified = false
			t.Error("owned Mongo role cleanup failed")
		}
		var users struct {
			Users []bson.Raw `bson:"users"`
		}
		if e := adminMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: user}, {Key: "db", Value: "admin"}}}}).Decode(&users); e != nil || len(users.Users) != 0 {
			cleanupVerified = false
			t.Error("owned Mongo user remaining")
		}
		var roles struct {
			Roles []bson.Raw `bson:"roles"`
		}
		if e := adminMongo.Database("admin").RunCommand(ctx, bson.D{{Key: "rolesInfo", Value: bson.D{{Key: "role", Value: role}, {Key: "db", Value: "admin"}}}}).Decode(&roles); e != nil || len(roles.Roles) != 0 {
			cleanupVerified = false
			t.Error("owned Mongo role remaining")
		}
		if db.Drop(ctx) != nil {
			cleanupVerified = false
			t.Error("owned Mongo namespace cleanup failed")
		}
		names, e := adminMongo.ListDatabaseNames(ctx, bson.D{{Key: "name", Value: namespace}})
		if e != nil || len(names) != 0 {
			cleanupVerified = false
			t.Error("owned Mongo namespace remaining")
		}
		if adminMongo.Disconnect(ctx) != nil {
			cleanupVerified = false
			t.Error("owned Mongo admin close failed")
		}
		if cleanupVerified && len(users.Users) == 0 && len(roles.Roles) == 0 && len(names) == 0 && e == nil {
			t.Log("owned_mongo_namespace_user_role_cleanup_zero")
		}
	})
	// The business qualifier requires the real complete unique indexes; a
	// hand-created _id-only catalog cannot prove business identity uniqueness.
	mongoVersion, _, migrationErr := migration.NewMongoMigrator(adminMongo, &migration.Config{Enabled: true, Database: namespace}).Run()
	if migrationErr != nil || mongoVersion != 38 {
		t.Fatal("actual additive38 Mongo fixture migration failed")
	}
	if adminMongo.Database("admin").RunCommand(t.Context(), bson.D{{Key: "createRole", Value: role}, {Key: "privileges", Value: bson.A{bson.D{{Key: "resource", Value: bson.D{{Key: "cluster", Value: true}}}, {Key: "actions", Value: bson.A{"replSetGetConfig"}}}}}, {Key: "roles", Value: bson.A{}}}).Err() != nil {
		t.Fatal("owned Mongo identity read role create failed")
	}
	roleMade = true
	if adminMongo.Database("admin").RunCommand(t.Context(), bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: password}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: namespace}}, bson.D{{Key: "role", Value: role}, {Key: "db", Value: "admin"}}}}}).Err() != nil {
		t.Fatal("owned Mongo test user create failed")
	}
	userMade = true
	t.Setenv("MYSQL_DATABASE", namespace)
	t.Setenv("MONGODB_DBNAME", namespace)
	t.Setenv("MONGODB_USERNAME", user)
	t.Setenv("MONGODB_PASSWORD", password)
	if nonempty {
		nativeOriginalSubmission(t, db)
	}
	return pool, adminMongo, db, namespace
}
func nativeOriginalSubmission(t *testing.T, db *mongo.Database) {
	t.Helper()
	at := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	sheet := sheetmongo.AnswerSheetPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(10042)}, OrgID: 7, TesteeID: 21, FillerID: 21, FillerType: "testee", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", FilledAt: at, Admission: &sheetmongo.AdmissionPO{Purpose: "independent_questionnaire", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0"}}
	if _, e := db.Collection("answersheets").InsertOne(t.Context(), sheet); e != nil {
		t.Fatal("owned business record insert failed")
	}
	p := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: "10042", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", OrgID: 7, TesteeID: 21, FillerID: 21, FillerType: "testee", SubmittedAt: at, Admission: &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurposeIndependentQuestionnaire, QuestionnaireCode: "Q", QuestionnaireVersion: "1.0"}}
	evt := event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: event.BaseEvent{ID: "owned-original-submission", EventTypeValue: "answersheet.submitted", AggregateTypeValue: "AnswerSheet", AggregateIDValue: "10042", OccurredAtValue: at.Add(time.Second)}, Data: p}
	body, e := domainwire.EncodeEvent(evt)
	if e != nil {
		t.Fatal("real original event encode failed")
	}
	row := bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "event_id", Value: evt.ID}, {Key: "event_type", Value: evt.EventTypeValue}, {Key: "aggregate_type", Value: evt.AggregateTypeValue}, {Key: "aggregate_id", Value: evt.AggregateIDValue}, {Key: "org_id", Value: int64(7)}, {Key: "topic_name", Value: "qs-survey"}, {Key: "payload_json", Value: string(body)}, {Key: "status", Value: "published"}, {Key: "attempt_count", Value: int32(0)}, {Key: "next_attempt_at", Value: at}, {Key: "created_at", Value: at}, {Key: "updated_at", Value: at.Add(time.Second)}, {Key: "published_at", Value: at.Add(time.Second)}}
	if _, e = db.Collection("domain_event_outbox").InsertOne(t.Context(), row); e != nil {
		t.Fatal("owned source event insert failed")
	}
}
func nativeInventoryBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(privateTestDir(t), "inventory")
	build := exec.CommandContext(t.Context(), os.Getenv("QS_HISTORY_GO"), "build", "-buildvcs=false", "-ldflags=-X main.sourceSHA="+sourceSHA, "-o", binary, "../qs-compatibility-retirement")
	if raw, err := build.CombinedOutput(); err != nil {
		_ = raw
		t.Fatal("frozen inventory executable build failed")
	}
	return binary
}
func nativeRunInventory(t *testing.T, binary, path, hash, mode, run, out string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, "--request", path, "--request-hash", hash, "--operation-id", "123-1", "--run-id", run, "--output-directory", out, "--mode", mode)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		var safe struct {
			ErrorCategory    string `json:"error_category"`
			DatabaseBindings map[string]struct {
				ErrorCategory string `json:"error_category"`
			} `json:"database_bindings"`
			Targets []struct {
				ErrorCategory string `json:"error_category"`
			} `json:"targets"`
		}
		_ = json.Unmarshal(raw, &safe)
		for _, db := range []string{"mysql", "mongodb"} {
			t.Log("inventory fixed database category " + db + " " + nativeSafeCategory(safe.DatabaseBindings[db].ErrorCategory))
		}
		for i, target := range safe.Targets {
			t.Log("inventory fixed target category " + strconv.Itoa(i) + " " + nativeSafeCategory(target.ErrorCategory))
		}
		t.Fatal("owned actual inventory failed: " + nativeSafeCategory(safe.ErrorCategory))
	}
}
func nativeSafeCategory(s string) string {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			if r != '_' {
				return "unknown"
			}
		}
	}
	if len(s) == 0 || len(s) > 100 {
		return "unknown"
	}
	return s
}
func nativeInputs(t *testing.T, pool *sql.DB, client *mongo.Client, db *mongo.Database) (string, string, *approvedInputs) {
	t.Helper()
	opdir := privateTestDir(t)
	boundsDir := filepath.Join(opdir, "bounds-122-1")
	invDir := filepath.Join(opdir, "inventory-124-1")
	if os.Mkdir(boundsDir, 0700) != nil || os.Mkdir(invDir, 0700) != nil {
		t.Fatal("owned private run directory")
	}
	var uuid, namespace string
	if pool.QueryRowContext(t.Context(), "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &namespace) != nil {
		t.Fatal("actual SQL identity read failed")
	}
	var hello bson.M
	if client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		t.Fatal("actual Mongo topology read failed")
	}
	stable := bson.D{}
	for _, k := range []string{"setName", "hosts", "me"} {
		if v, ok := hello[k]; ok {
			stable = append(stable, bson.E{Key: k, Value: v})
		}
	}
	encoded, e := json.Marshal(stable)
	if e != nil {
		t.Fatal("actual Mongo identity encode")
	}
	cur, e := db.ListCollections(t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
	if e != nil {
		t.Fatal("actual migration identity read")
	}
	var metadata []bson.Raw
	if cur.All(t.Context(), &metadata) != nil || len(metadata) != 1 {
		t.Fatal("actual migration metadata")
	}
	_, uuidBytes := metadata[0].Lookup("info", "uuid").Binary()
	identities := map[string]string{"mysql": framedParts("mysql_database_identity_v1", uuid, namespace), "mongodb": framedParts("mongodb_database_identity_v1", string(encoded), db.Name(), hex.EncodeToString(uuidBytes))}
	inv := inventoryRequest{FormatVersion: 2, Kind: "readonly_inventory_boundary_request", OperationID: "123-1", SourceSHA: sourceSHA, TargetHash: jsonHash(historyTargets), DatabaseScope: "mysql-and-mongodb", Identities: identities, Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}, Limits: inventoryLimits{30, 1500, 1_000_000, 2 << 30, 1000, 1001}}
	boundsPath := filepath.Join(opdir, "boundary-request.json")
	boundsSHA := writeFixtureJSON(t, boundsPath, inv)
	binary := nativeInventoryBinary(t)
	nativeRunInventory(t, binary, boundsPath, boundsSHA, "bounds", "122-1", boundsDir)
	boundaryPath := filepath.Join(boundsDir, "boundary.private.json")
	raw, e := os.ReadFile(boundaryPath)
	if e != nil {
		t.Fatal("private boundary output read failed")
	}
	var bounds inventoryReport
	if strictDecode(raw, &bounds) != nil || !bounds.Complete {
		t.Fatal("actual boundary report incomplete")
	}
	inv.Kind = "readonly_inventory_request"
	inv.BoundaryRunID = "122-1"
	inv.BoundaryReportHash = rawHash(raw)
	for _, s := range bounds.Targets {
		if s.Boundary == nil {
			t.Fatal("actual boundary missing")
		}
		inv.Boundaries = append(inv.Boundaries, *s.Boundary)
	}
	invPath := filepath.Join(opdir, "inventory-request.json")
	invSHA := writeFixtureJSON(t, invPath, inv)
	nativeRunInventory(t, binary, invPath, invSHA, "inventory", "124-1", invDir)
	reportPath := filepath.Join(invDir, "inventory.private.json")
	raw, e = os.ReadFile(reportPath)
	if e != nil {
		t.Fatal("actual source private report read")
	}
	var report inventoryReport
	if strictDecode(raw, &report) != nil || !report.Complete {
		t.Fatal("actual source report incomplete")
	}
	req := historyRequest{FormatVersion: 1, Kind: "readonly_compatibility_history_request", SourceSHA: sourceSHA, OperationID: "123-1", RunID: "125-1", InventoryRequest: fileBinding{invPath, invSHA}, InventoryReport: fileBinding{reportPath, rawHash(raw)}}
	for _, s := range report.Targets {
		path := filepath.Join(invDir, s.SourceFile)
		content, e := os.ReadFile(path)
		if e != nil {
			t.Fatal("actual source EOF read")
		}
		req.Assets = append(req.Assets, assetBinding{s.Database, s.Name, path, rawHash(content), uint64(len(content))})
	}
	path := filepath.Join(opdir, "history-request.json")
	hash := writeFixtureJSON(t, path, req)
	a, e := loadInputs(t.Context(), path, hash, "123-1", "125-1")
	if e != nil {
		t.Fatal("actual inventory to history input rejected: " + safeCategory(e))
	}
	t.Cleanup(func() {
		if a.close() != nil {
			t.Error("owned input close")
		}
	})
	return path, hash, a
}
func TestHistoryCLINativeActualInventoryEmptyAndOriginalBusiness(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(strconv.FormatBool(nonempty), func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, nonempty)
			path, hash, a := nativeInputs(t, pool, client, db)
			host, e := openDatabases(t.Context(), a)
			if e != nil {
				t.Fatal("actual authenticated CLI host open: " + safeCategory(e))
			}
			defer func() {
				if host.close() != nil {
					t.Error("owned host close")
				}
			}()
			r, e := executePipeline(t.Context(), a, host)
			if e != nil {
				t.Fatal("actual two-epoch chain failed: " + safeCategory(e))
			}
			if !r.CompletedReadOnlyPipeline || r.IndependentEpochs != 2 || !r.SourceFilesAndActualOriginsMatched || !r.BusinessAndResponsibilityFactsUnchanged || r.SQLLedgerCount != 8 || r.MongoCollectionCount != 11 || r.DropReady || r.CASComplete || r.WriterFenceProven || r.IndependentProductionApprovalVerified || r.FullExternalAIClosureVerified || r.OrderedMongoSourceMetadataApproved {
				t.Fatal("actual read-only qualification misstates boundaries")
			}
			if nonempty && (r.LocalCandidates != 1 || r.JointEventPages != 1 || r.Sources[3].Records != 1) {
				t.Fatal("actual original business source page not consumed")
			}
			if !nonempty && r.LocalCandidates != 0 {
				t.Fatal("present-empty source counted as a candidate")
			}
			if nonempty {
				// Exercise the actual new executable, including compile-time source
				// binding, host-owned pools/transactions, stdout and private O_EXCL
				// output. The SHA is a fixture binding, never a release approval.
				executable := filepath.Join(privateTestDir(t), "history")
				build := exec.CommandContext(t.Context(), os.Getenv("QS_HISTORY_GO"), "build", "-buildvcs=false", "-ldflags=-X main.sourceSHA="+sourceSHA, "-o", executable, ".")
				if raw, err := build.CombinedOutput(); err != nil {
					_ = raw
					t.Fatal("owned history executable build failed")
				}
				output := privateTestDir(t)
				cmd := exec.CommandContext(t.Context(), executable, "--request", path, "--request-sha256", hash, "--operation", "123-1", "--run", "125-1", "--output", output)
				raw, err := cmd.CombinedOutput()
				var actual readiness
				if err != nil || strictDecode(raw, &actual) != nil || !actual.CompletedReadOnlyPipeline || actual.IndependentEpochs != 2 || actual.LocalCandidates != 1 || actual.CandidateSHA256 != r.CandidateSHA256 || actual.SourceSHA != sourceSHA || actual.DropReady || actual.CASComplete || actual.MutationBackendEnabled {
					t.Fatal("actual compiled history host chain failed")
				}
				private, err := os.ReadFile(filepath.Join(output, "history.readiness.json"))
				if err != nil || string(private) != string(raw) {
					t.Fatal("actual compiled history receipt output diverged")
				}
			}
			output := privateTestDir(t)
			result, e := runCLI(t.Context(), []string{"--request", path, "--request-sha256", hash, "--operation", "123-1", "--run", "125-1", "--output", output})
			if e != nil || !result.CompletedReadOnlyPipeline {
				t.Fatal("real CLI lifecycle/output failed: " + safeCategory(e))
			}
			raw, e := os.ReadFile(filepath.Join(output, "history.readiness.json"))
			if e != nil || strings.Contains(string(raw), "owned-original-submission") || strings.Contains(string(raw), os.Getenv("MONGODB_PASSWORD")) || strings.Contains(string(raw), `"10042"`) {
				t.Fatal("public layered readiness leaked private references")
			}
		})
	}
}
func TestHistoryCLINativeStaleTxnAndChangedFactsRefuse(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, true)
	_, _, a := nativeInputs(t, pool, client, db)
	host, e := openDatabases(t.Context(), a)
	if e != nil {
		t.Fatal(safeCategory(e))
	}
	defer func() {
		if host.close() != nil {
			t.Error("owned host close")
		}
	}()
	var first *epochResult
	if e = host.epoch(t.Context(), func(ctx context.Context) error {
		var err error
		first, err = buildEpoch(ctx, a, host)
		if err != nil {
			return err
		}
		if a.rewind() != nil {
			t.Fatal("rewind")
		}
		if proof, err := first.origin.RecheckSnapshots(ctx, first.sql, first.mongo, a.readers()); err == nil || proof != nil {
			t.Fatal("old actual transaction fabricated fresh proof")
		}
		return nil
	}); e != nil {
		t.Fatal("native first chain: " + safeCategory(e))
	}
	if _, e = db.Collection("answersheets").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(10042)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "questionnaire_title", Value: "changed private business title"}}}}); e != nil {
		t.Fatal("owned business mutation")
	}
	if e = host.epoch(t.Context(), func(ctx context.Context) error {
		second, err := buildEpoch(ctx, a, host)
		if err != nil {
			return err
		}
		return compareEpochs(first, second)
	}); e == nil {
		t.Fatal("changed actual business baseline accepted")
	}
	if e = host.epoch(t.Context(), func(ctx context.Context) error {
		if first.sql.ValidateBorrowedSnapshot(ctx) == nil || first.mongo.ValidateBorrowedSnapshot(ctx) == nil {
			t.Fatal("old snapshot reused under fresh actual host transaction")
		}
		return nil
	}); e != nil {
		t.Fatal("host fresh cleanup failed")
	}
}
func TestHistoryCLINativeSourceTamperAndOriginDriftRefuse(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, true)
	_, _, a := nativeInputs(t, pool, client, db)
	host, e := openDatabases(t.Context(), a)
	if e != nil {
		t.Fatal(safeCategory(e))
	}
	defer func() {
		if host.close() != nil {
			t.Error("owned host close")
		}
	}()
	if _, e = db.Collection("domain_event_outbox").UpdateOne(t.Context(), bson.D{{Key: "event_id", Value: "owned-original-submission"}}, bson.D{{Key: "$set", Value: bson.D{{Key: "attempt_count", Value: int32(9)}}}}); e != nil {
		t.Fatal("owned source mutation")
	}
	if r, e := executePipeline(t.Context(), a, host); e == nil || r.CompletedReadOnlyPipeline || r.DropReady {
		t.Fatal("changed actual original source accepted")
	}
	if os.WriteFile(a.request.Assets[3].Path, []byte("private tampered BSON frame"), 0600) != nil {
		t.Fatal("owned file mutation")
	}
	if r, e := executePipeline(t.Context(), a, host); e == nil || r.CompletedReadOnlyPipeline || r.DropReady {
		t.Fatal("changed immutable source file accepted")
	}
	if pool.PingContext(t.Context()) != nil || client.Ping(t.Context(), readpref.Primary()) != nil {
		t.Fatal("host/helpers closed unrelated fixture pools")
	}
}

func TestHistoryCLINativeEmptySourcesDoNotHideGlobalUnknown(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, false)
	// This existing mutating maintenance store is deliberately unsupported by
	// the legacy-event closure. Its unknown responsibility must remain visible
	// even when all four actual legacy source copies have zero rows.
	if _, err := db.Collection("interpretation_catalog_repair_plans").InsertOne(t.Context(), bson.D{{Key: "_id", Value: primitive.NewObjectID()}}); err != nil {
		t.Fatal("owned global unknown insert failed")
	}
	_, _, a := nativeInputs(t, pool, client, db)
	host, err := openDatabases(t.Context(), a)
	if err != nil {
		t.Fatal("owned global unknown host open failed")
	}
	defer func() {
		if host.close() != nil {
			t.Error("owned global unknown host close failed")
		}
	}()
	r, err := executePipeline(t.Context(), a, host)
	if err != nil {
		t.Fatal("actual global unknown pipeline failed: " + safeCategory(err))
	}
	if !r.CompletedReadOnlyPipeline || r.LocalCandidates != 0 || r.MongoGlobal.Rows != 1 || r.MongoGlobal.ClassCounts["maintenance_repair_authorization_unknown"] != 1 || len(r.MongoGlobal.CoverageGaps) == 0 || r.BlockingReasons["mongo_global_coverage_gap_catalog_repair_plan_execution_coverage_required"] == 0 || r.DropReady || r.CASComplete {
		t.Fatal("empty legacy source hid actual global unknown responsibility")
	}
}

func TestHistoryCLINativeFirstEpochReleasesGraphsBeforeSecond(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, true)
	_, _, a := nativeInputs(t, pool, client, db)
	host, err := openDatabases(t.Context(), a)
	if err != nil {
		t.Fatal(safeCategory(err))
	}
	defer func() {
		if host.close() != nil {
			t.Error("owned host close")
		}
	}()
	var first *epochResult
	if err = host.epoch(t.Context(), func(ctx context.Context) error {
		var err error
		first, err = buildEpoch(ctx, a, host)
		if err != nil {
			return err
		}
		before := jsonHash(first.coordinator)
		if err = first.compactOrigin(ctx); err != nil {
			return err
		}
		if first.sql != nil || first.mongo != nil || first.origin != nil || first.anchor != nil || first.aiReverse != nil || first.aiReverseCoordinator != nil || first.reverseAnchor == nil || jsonHash(first.coordinator) != before {
			t.Fatal("compaction kept old graphs or lost stable source facts")
		}
		if err = first.compactOrigin(ctx); err == nil {
			t.Fatal("already compacted first epoch accepted twice")
		}
		return nil
	}); err != nil {
		t.Fatal(safeCategory(err))
	}
	if err = host.epoch(t.Context(), func(ctx context.Context) error {
		second, err := buildEpoch(ctx, a, host)
		if err != nil {
			return err
		}
		if err = compareEpochs(first, second); err != nil {
			return err
		}
		aiProof, err := first.recheckAIReverse(ctx, second, a)
		if err != nil {
			return err
		}
		if a.rewind() != nil {
			return fixedError("history_asset_read_failed")
		}
		proof, err := first.reverseAnchor.RecheckOrigin(ctx, aiProof, second.sql, second.mongo, second.aiReverseCoordinator, a.readers())
		if err != nil {
			return err
		}
		if r := proof.Report(); !r.ActualOriginMatched || !r.SourceFilesMatched || !r.IndependentEpochRechecked || !r.IndependentApprovalRequired || !r.FirstAuthMetadataContinuityUnproven || r.DropReady || r.CASAuthorized {
			t.Fatal("compaction changed fresh proof authority")
		}
		return nil
	}); err != nil {
		t.Fatal(safeCategory(err))
	}
}
