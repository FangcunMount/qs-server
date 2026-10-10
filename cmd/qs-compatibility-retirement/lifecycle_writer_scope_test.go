package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	dbcensus "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementdbcensus"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// These leaf checks run only in the separately owned network-none, tmpfs
// database fixture. They grant no original-window or production authority.
func TestDatabaseNativeMySQLLockDrainRestore(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_OWNED_DB_FIXTURE") != "mysql-network-none-tmpfs" {
		t.Skip("requires the owned isolated native MySQL fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	pool, e := sql.Open("mysql", "root:owned-fixture-only@tcp(127.0.0.1:3306)/?timeout=2s&readTimeout=5s&writeTimeout=5s")
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	for pool.PingContext(ctx) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("native MySQL readiness failed")
		case <-time.After(250 * time.Millisecond):
		}
	}
	conn, e := pool.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	for _, statement := range []string{"CREATE DATABASE qs", "CREATE TABLE qs.domain_event_outbox(id BIGINT PRIMARY KEY)", "CREATE USER 'owned_app'@'%' IDENTIFIED BY 'owned-fixture-app-only'", "GRANT INSERT,SELECT ON qs.domain_event_outbox TO 'owned_app'@'%'"} {
		if _, e = conn.ExecContext(ctx, statement); e != nil {
			t.Fatal(e)
		}
	}
	var maintenanceID uint64
	if e = conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&maintenanceID); e != nil {
		t.Fatal(e)
	}
	p := lifecycleDBPrincipalExpected{User: "owned_app", HostOrDatabase: "%"}
	v := &lifecycleDBWriterLease{host: &lifecycleFixedHost{owner: &lifecyclePreparationOwner{originalConn: conn}}, connectionID: maintenanceID, input: lifecycleDBWriterInput{SQLPrincipals: []lifecycleDBPrincipalExpected{p}}}
	app, e := sql.Open("mysql", "owned_app:owned-fixture-app-only@tcp(127.0.0.1:3306)/qs?timeout=2s&readTimeout=5s&writeTimeout=5s")
	if e != nil {
		t.Fatal(e)
	}
	defer app.Close()
	idle, e := app.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer idle.Close()
	var idleID uint64
	if e = idle.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&idleID); e != nil {
		t.Fatal(e)
	}
	if e = v.setSQLLock(ctx, p, true); e != nil {
		t.Fatal(e)
	}
	blocked, e := sql.Open("mysql", "owned_app:owned-fixture-app-only@tcp(127.0.0.1:3306)/qs?timeout=2s")
	if e != nil {
		t.Fatal(e)
	}
	if blocked.PingContext(ctx) == nil {
		t.Fatal("ACCOUNT LOCK allowed a new app connection")
	}
	_ = blocked.Close()
	var oldSessions int
	if e = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID=? AND COMMAND='Sleep'", idleID).Scan(&oldSessions); e != nil || oldSessions != 1 {
		t.Fatal("original idle session was not observed", e)
	}
	if e = v.drainSQL(ctx); e != nil {
		t.Fatal(e)
	}
	if e = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE USER=?", p.User).Scan(&oldSessions); e != nil || oldSessions != 0 {
		t.Fatal("old app session survived exact drain", e)
	}
	if e = v.setSQLLock(ctx, p, false); e != nil {
		t.Fatal(e)
	}
	restored, e := sql.Open("mysql", "owned_app:owned-fixture-app-only@tcp(127.0.0.1:3306)/qs?timeout=2s&readTimeout=5s&writeTimeout=5s")
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if _, e = restored.ExecContext(ctx, "INSERT INTO domain_event_outbox(id) VALUES(1)"); e != nil {
		t.Fatal("restored app admission/write failed", e)
	}
	if e = v.setSQLLock(ctx, p, false); e != nil {
		t.Fatal("original-state idempotent restore failed", e)
	}
	t.Log("native MySQL: new admission denied, original Sleep session killed, original account restored and actual app write succeeded")
}

func TestDatabaseNativeMongoRolesSessionsRestore(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_OWNED_DB_FIXTURE") != "mongo-network-none-tmpfs" {
		t.Skip("requires the owned isolated native MongoDB fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	admin, e := mongo.Connect(ctx, options.Client().SetHosts([]string{"127.0.0.1:27017"}).SetDirect(true).SetMaxPoolSize(1).SetServerSelectionTimeout(2*time.Second).SetAuth(options.Credential{AuthSource: "admin", Username: "owned_admin", Password: "owned-fixture-only"}))
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Disconnect(context.Background())
	for admin.Ping(ctx, nil) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("native Mongo readiness failed")
		case <-time.After(250 * time.Millisecond):
		}
	}
	if e = admin.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetInitiate", Value: bson.D{{Key: "_id", Value: "qs_fixture"}, {Key: "members", Value: bson.A{bson.D{{Key: "_id", Value: 0}, {Key: "host", Value: "localhost:27017"}}}}}}}).Err(); e != nil {
		t.Fatal("native replica initialization failed", e)
	}
	for {
		var hello struct {
			Writable bool `bson:"isWritablePrimary"`
		}
		if admin.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) == nil && hello.Writable {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("native Mongo primary readiness failed")
		case <-time.After(250 * time.Millisecond):
		}
	}
	// Mongo root does not imply impersonate. The first native fixture exposed
	// this exact missing privilege; configure it explicitly in this owned fixture.
	if e = admin.Database("admin").RunCommand(ctx, bson.D{{Key: "createRole", Value: "owned_session_controller"}, {Key: "privileges", Value: bson.A{bson.D{{Key: "resource", Value: bson.D{{Key: "cluster", Value: true}}}, {Key: "actions", Value: bson.A{"impersonate"}}}}}, {Key: "roles", Value: bson.A{}}}).Err(); e != nil {
		t.Fatal(e)
	}
	if e = admin.Database("admin").RunCommand(ctx, bson.D{{Key: "grantRolesToUser", Value: "owned_admin"}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "owned_session_controller"}, {Key: "db", Value: "admin"}}}}}).Err(); e != nil {
		t.Fatal(e)
	}
	if e = admin.Database("admin").RunCommand(ctx, bson.D{{Key: "invalidateUserCache", Value: 1}}).Err(); e != nil {
		t.Fatal(e)
	}
	p := lifecycleDBPrincipalExpected{User: "owned_app", HostOrDatabase: "admin"}
	original := []bson.M{{"role": "readWrite", "db": "qs"}}
	if e = admin.Database("admin").RunCommand(ctx, bson.D{{Key: "createUser", Value: p.User}, {Key: "pwd", Value: "owned-fixture-app-only"}, {Key: "roles", Value: original}, {Key: "writeConcern", Value: bson.D{{Key: "w", Value: "majority"}}}}).Err(); e != nil {
		t.Fatal(e)
	}
	v := &lifecycleDBWriterLease{host: &lifecycleFixedHost{owner: &lifecyclePreparationOwner{originalMongo: admin, originalDB: admin.Database("qs")}}, input: lifecycleDBWriterInput{MongoPrincipals: []lifecycleDBPrincipalExpected{p}}}
	app, e := mongo.Connect(ctx, options.Client().SetHosts([]string{"127.0.0.1:27017"}).SetDirect(true).SetMaxPoolSize(1).SetAuth(options.Credential{AuthSource: "admin", Username: p.User, Password: "owned-fixture-app-only"}))
	if e != nil {
		t.Fatal(e)
	}
	defer app.Disconnect(context.Background())
	if _, e = app.Database("qs").Collection("domain_event_outbox").InsertOne(ctx, bson.M{"_id": 1}); e != nil {
		t.Fatal("original actual app write failed", e)
	}
	session, e := app.StartSession()
	if e != nil {
		t.Fatal(e)
	}
	if e = session.StartTransaction(); e != nil {
		t.Fatal(e)
	}
	sc := mongo.NewSessionContext(ctx, session)
	if _, e = app.Database("qs").Collection("domain_event_outbox").InsertOne(sc, bson.M{"_id": 10}); e != nil {
		t.Fatal(e)
	}
	if v.checkMongoTransactions(ctx) == nil {
		t.Fatal("actual original app transaction was ignored")
	}
	if e = session.AbortTransaction(sc); e != nil {
		t.Fatal(e)
	}
	session.EndSession(ctx)
	if e = v.setMongoRoles(ctx, p, original, false); e != nil {
		t.Fatal(e)
	}
	if _, e = app.Database("qs").Collection("domain_event_outbox").InsertOne(ctx, bson.M{"_id": 2}); e == nil {
		t.Fatal("revoked cached app connection retained write admission")
	} else {
		var native mongo.CommandError
		if !errors.As(e, &native) || native.Code != 13 {
			t.Fatal("expected native authorization refusal", e)
		}
	}
	if e = v.drainMongo(ctx); e != nil {
		t.Fatal(e)
	}
	if e = v.setMongoRoles(ctx, p, original, true); e != nil {
		t.Fatal(e)
	}
	if _, e = app.Database("qs").Collection("domain_event_outbox").InsertOne(ctx, bson.M{"_id": 3}); e != nil {
		t.Fatal("restored actual app write failed", e)
	}
	if e = v.setMongoRoles(ctx, p, original, true); e != nil {
		t.Fatal("original-state idempotent restore failed", e)
	}
	foreign := []bson.M{{"role": "read", "db": "qs"}}
	if e = admin.Database("admin").RunCommand(ctx, bson.D{{Key: "updateUser", Value: p.User}, {Key: "roles", Value: foreign}}).Err(); e != nil {
		t.Fatal(e)
	}
	if e = v.setMongoRoles(ctx, p, original, true); e == nil {
		t.Fatal("foreign roles were overwritten by restore")
	}
	t.Log("native Mongo: cached app write denied after actual role revoke/cache refresh, exact session drain and original roles restored; foreign-role conflict refused")
}

func TestWriterScopeCannotImportAuthorityOrEnterPreparation(t *testing.T) {
	for _, name := range []string{"drop_ready", "whole_writer_fence_proven", "platform_observation", "SourceSHA", "workflow_scope_sha256_extra"} {
		raw := []byte(`{"writer_control":{"workflow_scope_sha256":"` + strings.Repeat("a", 64) + `","` + name + `":true}}`)
		if lifecycleExactJSONNames(raw, reflect.TypeOf(lifecycleRequest{})) == nil {
			t.Fatal("imported field accepted")
		}
	}
	for _, value := range []string{"null", `{"workflow_scope_sha256":"` + strings.Repeat("a", 64) + `"}`} {
		if _, e := decodeLifecycleStagingRequest([]byte(`{"writer_control":` + value + `}`)); e == nil {
			t.Fatal("prepare accepted writer field")
		}
	}
	for _, v := range []*lifecycleWriterControl{nil, {}, {WorkflowScopeSHA256: "main"}} {
		if v.valid() {
			t.Fatal("unbound scope accepted")
		}
	}
	v := &lifecycleWriterControl{WorkflowScopeSHA256: strings.Repeat("a", 64)}
	raw, e := json.Marshal(v)
	if e != nil || string(raw) != `{"workflow_scope_sha256":"`+strings.Repeat("a", 64)+`"}` {
		t.Fatal("unexpected expected-input schema")
	}
}

func TestDatabaseWriterGrantsCoverColumnsDelegationAndScope(t *testing.T) {
	for _, tc := range []struct {
		grant           string
		writes, unknown bool
	}{
		{"GRANT SELECT ON `qs`.* TO 'reader'@'%'", false, false},
		{"GRANT UPDATE (`status`, `content`) ON `qs`.`domain_event_outbox` TO 'u'@'%'", true, false},
		{"GRANT INSERT ON `qs`.`rm_outbox` TO 'u'@'%'", false, false},
		{"GRANT ALL PRIVILEGES ON *.* TO 'u'@'%'", true, false},
		{"GRANT EXECUTE ON `qs`.`indirect_writer` TO 'u'@'%'", true, false},
		{"GRANT UPDATE ON `other`.* TO 'u'@'%'", false, false},
		{"GRANT UPDATE ON `qs%`.* TO 'u'@'%'", false, true},
		{"GRANT UPDATE ON invalid TO 'u'@'%'", false, true},
		{"unrecognized grant", false, true},
	} {
		t.Run(tc.grant, func(t *testing.T) {
			got, e := lifecycleSQLGrantMayWrite(tc.grant, "qs")
			if got != tc.writes || (e != nil) != tc.unknown {
				t.Fatalf("scope changed: %v %v", got, e)
			}
		})
	}
	for _, tc := range []struct {
		db, collection, action string
		writes                 bool
	}{{"qs", "domain_event_outbox", "insert", true}, {"qs", "rm_outbox", "insert", false}, {"other", "domain_event_outbox", "update", false}, {"", "", "dropDatabase", true}, {"admin", "", "grantRole", true}, {"qs", "domain_event_outbox", "find", false}} {
		u := bson.M{"inheritedPrivileges": bson.A{bson.M{"resource": bson.M{"db": tc.db, "collection": tc.collection}, "actions": bson.A{tc.action}}}}
		if got, e := lifecycleMongoMayWrite(u, "qs"); e != nil || got != tc.writes {
			t.Fatal("Mongo writer scope changed", tc, got, e)
		}
	}
}

func TestDatabaseCensusDigestSurvivesPrivateBSONIdentityEncoding(t *testing.T) {
	v := bson.M{"userId": primitive.Binary{Subtype: 4, Data: []byte{1, 2, 3}}, "roles": bson.A{bson.M{"role": "readWrite", "db": "qs"}}}
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var fromPrivate bson.M
	if json.Unmarshal(raw, &fromPrivate) != nil || lifecycleDBDigest(v) != lifecycleDBDigest(fromPrivate) {
		t.Fatal("BSON identity changed only by native private JSON encoding")
	}
	fromPrivate["userId"] = bson.M{"Subtype": 4, "Data": "different"}
	if lifecycleDBDigest(v) == lifecycleDBDigest(fromPrivate) {
		t.Fatal("different original principal identity was ignored")
	}
}

func databaseWriterPolicyFixture(t *testing.T) (*lifecycleDBWriterLease, dbcensus.Catalog) {
	t.Helper()
	row := func(vs ...string) []*string {
		out := []*string{}
		for _, s := range vs {
			copy := s
			out = append(out, &copy)
		}
		return out
	}
	user := func(name string) bson.M {
		return bson.M{"user": name, "db": "admin", "userId": name, "roles": bson.A{bson.M{"role": "readWrite", "db": "qs"}}, "inheritedRoles": bson.A{}, "inheritedPrivileges": bson.A{bson.M{"resource": bson.M{"db": "qs", "collection": ""}, "actions": bson.A{"insert"}}}}
	}
	c := dbcensus.Catalog{SQL: map[string][][]*string{"accounts": {row("maint", "%", "caching_sha2_password", "N"), row("app", "%", "caching_sha2_password", "N")}, "account_grants": {row("maint", "%", "GRANT ALL PRIVILEGES ON *.* TO 'maint'@'%'"), row("app", "%", "GRANT INSERT ON `qs`.* TO 'app'@'%'")}, "connections": {row("5", "maint", "host:1", "qs", "Sleep", ""), row("8", "app", "host:2", "qs", "Sleep", "")}}, Mongo: map[string][]bson.M{"users": {user("maint"), user("app")}, "connections_and_idle_operations": {{"connectionId": int64(11), "effectiveUsers": bson.A{bson.M{"user": "maint", "db": "admin"}}}}}}
	client := new(mongo.Client)
	h := &lifecycleFixedHost{owner: &lifecyclePreparationOwner{originalMongo: client, originalDB: client.Database("qs")}}
	v := &lifecycleDBWriterLease{host: h, sqlDatabase: "qs", connectionID: 5, mongoConnectionID: 11, input: lifecycleDBWriterInput{SQLObserver: lifecycleDBPrincipalExpected{User: "maint", HostOrDatabase: "%"}, MongoObserver: lifecycleDBPrincipalExpected{User: "maint", HostOrDatabase: "admin"}, SQLPrincipals: []lifecycleDBPrincipalExpected{{User: "app", HostOrDatabase: "%"}}, MongoPrincipals: []lifecycleDBPrincipalExpected{{User: "app", HostOrDatabase: "admin"}}}, baseline: c}
	v.staticSHA = lifecycleDBStaticHash(c)
	return v, c
}

func TestDatabaseWriterCoverageRejectsUnknownSharedAndIdleWriters(t *testing.T) {
	v, c := databaseWriterPolicyFixture(t)
	if e := v.validateWriterCoverage(c, false); e != nil {
		t.Fatal(e)
	}
	clone := func() dbcensus.Catalog {
		raw, _ := json.Marshal(c)
		var next dbcensus.Catalog
		if json.Unmarshal(raw, &next) != nil {
			t.Fatal("fixture")
		}
		return next
	}
	for _, tc := range []struct {
		name   string
		mutate func(dbcensus.Catalog)
	}{
		{"unknown-target-owner", func(c dbcensus.Catalog) {
			for _, g := range c.SQL["account_grants"] {
				if *g[0] == "app" {
					*g[0] = "other"
				}
			}
		}},
		{"shared-maintenance-idle", func(c dbcensus.Catalog) {
			copy := append([]*string(nil), c.SQL["connections"][0]...)
			id := "6"
			copy[0] = &id
			c.SQL["connections"] = append(c.SQL["connections"], copy)
		}},
		{"shared-mongo-maintenance", func(c dbcensus.Catalog) { c.Mongo["connections_and_idle_operations"][0]["connectionId"] = int64(12) }},
		{"changed-role-binding", func(c dbcensus.Catalog) { c.Mongo["users"][1]["roles"] = bson.A{bson.M{"role": "other", "db": "qs"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := clone()
			tc.mutate(next)
			if v.validateWriterCoverage(next, false) == nil {
				t.Fatal("unknown/shared/changed writer accepted")
			}
		})
	}
	locked := clone()
	yes := "Y"
	locked.SQL["accounts"][1][3] = &yes
	locked.Mongo["users"][1]["roles"] = bson.A{}
	locked.Mongo["users"][1]["inheritedPrivileges"] = bson.A{}
	if v.validateWriterCoverage(locked, true) == nil {
		t.Fatal("ACCOUNT LOCK with old Sleep session became isolation")
	}
	locked.SQL["connections"] = locked.SQL["connections"][:1]
	if e := v.validateWriterCoverage(locked, true); e != nil {
		t.Fatal("bounded account/roles/session policy rejected", e)
	}
}

func databaseWriterSQLFixture(t *testing.T) (*lifecycleDBWriterLease, sqlmock.Sqlmock) {
	t.Helper()
	pool, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	conn, e := pool.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = conn.Close(); _ = pool.Close() })
	return &lifecycleDBWriterLease{host: &lifecycleFixedHost{owner: &lifecyclePreparationOwner{originalConn: conn}}, connectionID: 5, input: lifecycleDBWriterInput{SQLPrincipals: []lifecycleDBPrincipalExpected{{User: "app'o", HostOrDatabase: "%"}}}}, m
}
func TestDatabaseAccountLockCompareApplyReadbackAndConflict(t *testing.T) {
	q := regexp.QuoteMeta("SELECT account_locked FROM mysql.user WHERE User=? AND Host=?")
	for _, tc := range []struct {
		name, current        string
		restore, effectError bool
	}{{"lock", "N", false, false}, {"restore", "Y", true, false}, {"already-original", "N", true, false}, {"foreign-lock", "Y", false, false}, {"unknown-command", "N", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			v, m := databaseWriterSQLFixture(t)
			p := v.input.SQLPrincipals[0]
			m.ExpectQuery(q).WithArgs(p.User, p.HostOrDatabase).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(tc.current))
			conflict := !tc.restore && tc.current == "Y"
			if !conflict && !(tc.restore && tc.current == "N") {
				verb, next := "LOCK", "Y"
				if tc.restore {
					verb, next = "UNLOCK", "N"
				}
				ex := m.ExpectExec(regexp.QuoteMeta("ALTER USER 'app''o'@'%' ACCOUNT " + verb))
				if tc.effectError {
					ex.WillReturnError(errors.New("unknown result"))
				} else {
					ex.WillReturnResult(sqlmock.NewResult(0, 0))
					m.ExpectQuery(q).WithArgs(p.User, p.HostOrDatabase).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(next))
				}
			}
			e := v.setSQLLock(t.Context(), p, !tc.restore)
			if (e != nil) != (conflict || tc.effectError) {
				t.Fatal("account transition conflict/unknown hidden", e)
			}
			if m.ExpectationsWereMet() != nil {
				t.Fatal("wrong native SQL sequence")
			}
		})
	}
}
func TestDatabaseDrainTargetsExactOriginalIDsAndRechecksOwner(t *testing.T) {
	for _, tc := range []struct {
		name, owner string
		gone, self  bool
	}{{"exact-id", "app'o", false, false}, {"owner-changed", "other", false, false}, {"already-ended", "", true, false}, {"original-maintenance-id", "app'o", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			v, m := databaseWriterSQLFixture(t)
			id := uint64(8)
			if tc.self {
				id = 5
			}
			m.ExpectQuery(regexp.QuoteMeta("SELECT ID FROM information_schema.PROCESSLIST WHERE USER=? ORDER BY ID")).WithArgs("app'o").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
			if !tc.self {
				ex := m.ExpectQuery(regexp.QuoteMeta("SELECT USER FROM information_schema.PROCESSLIST WHERE ID=?")).WithArgs(id)
				if tc.gone {
					ex.WillReturnError(sql.ErrNoRows)
				} else {
					ex.WillReturnRows(sqlmock.NewRows([]string{"user"}).AddRow(tc.owner))
					if tc.owner == "app'o" {
						m.ExpectExec("KILL CONNECTION 8").WillReturnResult(sqlmock.NewResult(0, 0))
					}
				}
			}
			e := v.drainSQL(t.Context())
			if (e != nil) != (tc.self || !tc.gone && tc.owner != "app'o") {
				t.Fatal("idle session ownership lost", e)
			}
			if e = m.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestDatabaseLeaseCannotMintFenceOrRestoreImportedOwner(t *testing.T) {
	h := new(lifecycleFixedHost)
	if h.installDatabaseWriterLease(t.Context(), lifecycleRequest{}) == nil || h.checkDatabaseWriterLease(t.Context(), lifecycleRequest{}) == nil || h.restoreDatabaseWriterLease(t.Context(), lifecycleRequest{}, true) == nil {
		t.Fatal("missing actual input/native owners became success")
	}
	h.dbWriters = &lifecycleDBWriterLease{installed: true, restored: true}
	if h.checkDatabaseWriterLease(t.Context(), lifecycleRequest{}) == nil || h.restoreDatabaseWriterLease(t.Context(), lifecycleRequest{}, false) == nil || h.Close() == nil {
		t.Fatal("imported installed/restored flag granted native effect or restoration")
	}
	if lifecycleEffectsPreflight(t.Context()) == nil {
		t.Fatal("database leaf activated full production effects")
	}
}

func TestWriterTokenStaysInLiveOwnerAndIsCleared(t *testing.T) {
	for _, raw := range []string{"", "invalid\ncredential", strings.Repeat("a", 8193)} {
		t.Setenv("GITHUB_READ_TOKEN", raw)
		if b, e := lifecyclePlatformToken(); e == nil || b != nil || os.Getenv("GITHUB_READ_TOKEN") != "" {
			t.Fatal("invalid token retained")
		}
	}
	t.Setenv("GITHUB_READ_TOKEN", "offline-private-input")
	b, e := lifecyclePlatformToken()
	if e != nil || os.Getenv("GITHUB_READ_TOKEN") != "" {
		t.Fatal("token environment retained")
	}
	owner := &lifecycleWriterObservation{token: b, platform: new(fence.PlatformObservation)}
	owner.close()
	owner.close()
	if owner.token != nil || owner.platform != nil {
		t.Fatal("owner retains credential/observation")
	}
	for _, c := range b {
		if c != 0 {
			t.Fatal("borrowed credential bytes not cleared")
		}
	}
}

func TestWriterScopeRequiresOriginalLiveNativeManagement(t *testing.T) {
	r := lifecycleRequest{WriterControl: &lifecycleWriterControl{WorkflowScopeSHA256: strings.Repeat("a", 64)}}
	for _, h := range []*lifecycleFixedHost{nil, {}, {services: &lifecycleServiceController{window: new(fence.MaintenanceWindow), managementReady: true}}} {
		if e := h.CheckWriterPreconditions(context.Background(), r); e == nil {
			t.Fatal("expected identities became actual pre-stop platform quarantine")
		}
		if e := h.CheckWholeWriterFence(context.Background(), r); e == nil {
			t.Fatal("expected identities became isolation")
		}
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("partial platform leaf activated DDL")
	}
}

func TestWriterPreconditionsCannotReuseStoppedOrInstalledOwner(t *testing.T) {
	for _, h := range []*lifecycleFixedHost{
		{services: &lifecycleServiceController{stopAttempted: true}},
		{services: &lifecycleServiceController{remoteStopAttempted: true}},
		{services: &lifecycleServiceController{}, dbWriters: &lifecycleDBWriterLease{installed: true}},
	} {
		if e := h.CheckWriterPreconditions(t.Context(), lifecycleRequest{}); e != lifecycleError("lifecycle_writer_precondition_phase_rejected") {
			t.Fatal("post-stop state reused narrower precondition", e)
		}
	}
}

func TestDatabaseWriterNativeProducerBindsCensusAndOriginalActors(t *testing.T) {
	v, c := databaseWriterPolicyFixture(t)
	r := lifecycleRequest{ToolSourceSHA: strings.Repeat("b", 40), OriginalSourceSHA: strings.Repeat("a", 40), OperationID: "12-1", ActualRunID: "22-1", ManifestSHA256: strings.Repeat("c", 64), Recovery: backup.TargetRecoveryRequest{OriginalRunID: "16-1"}}
	original := dbCensusPrivate{SourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID, RunID: "18-1", RequestSHA256: strings.Repeat("d", 64), IdentityProducer: prepareFactsProducer{OperationID: r.OperationID, RunID: "17-1", SourceSHA: r.OriginalSourceSHA, ReportSHA256: strings.Repeat("e", 64), RequestSHA256: strings.Repeat("f", 64)}}
	file := lifecycleFinalFileBinding{Path: filepath.Join(lifecycleRootBatch(r.OperationID, original.RunID), "db-writer-census.private.json"), SHA256: strings.Repeat("d", 64)}
	if run, ok := lifecycleDBInputCensusRun(file.Path, r.OperationID); !ok || run != original.RunID || !lifecycleDBCensusProducerValid(original, r, run) {
		t.Fatal("actual original census source rejected")
	}
	for _, path := range []string{"/tmp/db-writer-census.private.json", strings.Replace(file.Path, "12-1", "13-1", 1), strings.Replace(file.Path, "18-1", "future", 1), file.Path + "/../db-writer-census.private.json"} {
		if _, ok := lifecycleDBInputCensusRun(path, r.OperationID); ok {
			t.Fatal("non-exact original source path accepted", path)
		}
	}
	bad := original
	bad.SourceSHA = r.ToolSourceSHA
	if lifecycleDBCensusProducerValid(bad, r, original.RunID) {
		t.Fatal("different census source accepted")
	}
	bad = original
	bad.WriterScopeComplete = true
	if lifecycleDBCensusProducerValid(bad, r, original.RunID) {
		t.Fatal("census isolation token accepted")
	}
	actors := []stop.DatabasePrincipal{
		{Component: "qs-apiserver", ContainerID: strings.Repeat("1", 64), EnvironmentSHA256: strings.Repeat("2", 64), SQLUser: "app", SQLDatabase: "qs", MongoUser: "app", MongoDatabase: "qs"},
		{Component: "qs-worker", ContainerID: strings.Repeat("3", 64), EnvironmentSHA256: strings.Repeat("4", 64), SQLUser: "app", SQLDatabase: "qs", MongoUser: "app", MongoDatabase: "qs"},
	}
	row := func(parts ...string) []*string {
		out := []*string{}
		for _, part := range parts {
			copy := part
			out = append(out, &copy)
		}
		return out
	}
	// The real AI account exists but its own database grant cannot write the
	// four qs targets. Its history responsibility is checked by the AI protocol.
	c.SQL["accounts"] = append(c.SQL["accounts"], row("ai", "%", "caching_sha2_password", "N"))
	c.SQL["account_grants"] = append(c.SQL["account_grants"], row("ai", "%", "GRANT INSERT ON `ai`.* TO 'ai'@'%'"))
	sqlObserver := lifecycleDBPrincipalExpected{User: "maint", HostOrDatabase: "%", Owners: []string{"original_maintenance_run"}}
	mongoObserver := lifecycleDBPrincipalExpected{User: "maint", HostOrDatabase: "admin", Owners: []string{"original_maintenance_run"}}
	produce := func(actors []stop.DatabasePrincipal, ai string) (lifecycleDBWriterInput, error) {
		return lifecycleProduceDBWriterInput(r, file, original, c, actors, ai, sqlObserver, mongoObserver, "qs", "qs")
	}
	in, e := produce(actors, "ai")
	if e != nil || len(in.SQLPrincipals) != 1 || len(in.MongoPrincipals) != 1 || !reflect.DeepEqual(in.SQLPrincipals[0].Owners, []string{"qs-apiserver", "qs-worker"}) || !reflect.DeepEqual(in.OriginalActors, actors) {
		t.Fatal("true original actor ownership not composed", e, in)
	}
	for _, tc := range []struct {
		name   string
		change func([]stop.DatabasePrincipal)
	}{
		{"wrong-component", func(a []stop.DatabasePrincipal) { a[1].Component = "external" }},
		{"duplicate-component", func(a []stop.DatabasePrincipal) { a[1].Component = a[0].Component }},
		{"different-database", func(a []stop.DatabasePrincipal) { a[0].SQLDatabase = "other" }},
		{"missing-config", func(a []stop.DatabasePrincipal) { a[0].EnvironmentSHA256 = "" }},
		{"shared-maintenance", func(a []stop.DatabasePrincipal) { a[0].SQLUser = "maint" }},
		{"unknown-account", func(a []stop.DatabasePrincipal) { a[0].SQLUser = "unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := append([]stop.DatabasePrincipal(nil), actors...)
			tc.change(a)
			if _, err := produce(a, "ai"); err == nil {
				t.Fatal("unproven source accepted")
			}
		})
	}
	// Reuse the same effective role propagation as the actual isolation policy.
	c.SQL["account_grants"][1] = row("app", "%", "GRANT SELECT ON `qs`.* TO 'app'@'%'")
	c.SQL["accounts"] = append(c.SQL["accounts"], row("writer_role", "%", "caching_sha2_password", "Y"))
	c.SQL["account_grants"] = append(c.SQL["account_grants"], row("writer_role", "%", "GRANT INSERT ON `qs`.* TO 'writer_role'@'%'"))
	c.SQL["role_edges"] = [][]*string{row("%", "writer_role", "%", "app", "N")}
	in, e = produce(actors, "ai")
	if e != nil || len(in.SQLPrincipals) != 1 {
		t.Fatal("actual inherited target grant omitted", e)
	}
	// Native source generation never conceals an unowned writer from coverage.
	c.SQL["accounts"] = append(c.SQL["accounts"], row("outside", "%", "caching_sha2_password", "N"))
	c.SQL["account_grants"] = append(c.SQL["account_grants"], row("outside", "%", "GRANT INSERT ON `qs`.* TO 'outside'@'%'"))
	in, e = produce(actors, "ai")
	if e != nil {
		t.Fatal(e)
	}
	v.input, v.baseline, v.staticSHA = in, c, lifecycleDBStaticHash(c)
	if e = v.validateWriterCoverage(c, false); e != lifecycleError("lifecycle_database_unowned_target_writer") {
		t.Fatal("producer turned unknown catalog ownership into isolation", e)
	}
	if lifecycleEffectsPreflight(t.Context()) == nil {
		t.Fatal("expected input activated production effects")
	}
}
