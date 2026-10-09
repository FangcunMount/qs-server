package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const compatibilitySQLVersion uint = 100
const compatibilityMongoVersion uint = 39

var retirementSQLNames = []string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands"}
var retirementHashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var retirementUUIDRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var retirementOpRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)

type PairConfig struct {
	MySQLDatabase                string
	MongoDatabase                string
	BootstrapAuthorizationFile   string
	BootstrapAuthorizationSHA256 string
	// This comes from the host's compiled build identity, never a runtime flag.
	ExpectedSourceSHA string
}
type PairPreflight struct {
	mu                  sync.Mutex
	sqlDB               *sql.DB
	sqlConn             *sql.Conn
	controlledRun       bool
	migrationFailed     bool
	initialSQLVersion   uint
	initialMongoVersion uint
	mongo               *mongo.Client
	config              PairConfig
	pristine            bool
	sqlHash             string
	mongoHash           string
	sqlDone             bool
	authorization       *bootstrapAuthorization
}

// Both the normal pool and the dedicated host connection expose reads. The
// controlled host path never acquires another pool connection or closes this one.
type retirementSQLReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func retirementSQLMissing(reader retirementSQLReader) bool {
	switch db := reader.(type) {
	case *sql.DB:
		return db == nil
	case *sql.Conn:
		return db == nil
	default:
		return true
	}
}
func (p *PairPreflight) sqlReader() retirementSQLReader {
	if p.sqlConn != nil {
		return p.sqlConn
	}
	return p.sqlDB
}

type bootstrapAuthorization struct {
	FormatVersion            int    `json:"format_version"`
	Kind                     string `json:"kind"`
	OperationID              string `json:"operation_id"`
	ExpiresAt                string `json:"expires_at"`
	ApprovedSourceSHA        string `json:"approved_source_sha"`
	ApprovalSummarySHA256    string `json:"approval_summary_sha256"`
	MySQLDatabase            string `json:"mysql_database"`
	MongoDatabase            string `json:"mongo_database"`
	MySQLIdentitySHA256      string `json:"mysql_identity_sha256"`
	MongoClusterSHA256       string `json:"mongo_cluster_sha256"`
	MySQLVersion             uint   `json:"mysql_version"`
	MongoVersion             uint   `json:"mongo_version"`
	MigrationResourcesSHA256 string `json:"migration_resources_sha256"`
}

func retirementError(s string) error   { return fmt.Errorf("compatibility retirement: %s", s) }
func retirementHash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func marshalSQLIdentity(uuid, name string) ([]byte, error) {
	return json.Marshal([]any{"mysql_server_selected_v1", uuid, name})
}
func CompatibilityMigrationResourcesSHA256() string {
	a, _ := migrations.ReadFile("migrations/mysql/000100_retire_compatibility_message_storage.up.sql")
	b, _ := migrations.ReadFile("migrations/mongodb/000039_retire_compatibility_message_storage.up.json")
	raw, _ := json.Marshal([]string{retirementHash(a), retirementHash(b)})
	return retirementHash(raw)
}
func retirementContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 15*time.Second)
}
func pairIdentity(ctx context.Context, db retirementSQLReader, client *mongo.Client, sqlName, mongoName string) (string, string, error) {
	if ctx == nil || ctx.Err() != nil || retirementSQLMissing(db) || client == nil || sqlName == "" || mongoName == "" {
		return "", "", retirementError("pair connections or selected names missing")
	}
	q, cancel := retirementContext(ctx)
	defer cancel()
	var uuid, selected string
	if e := db.QueryRowContext(q, "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &selected); e != nil {
		return "", "", retirementError("mysql identity read failed")
	}
	if selected != sqlName || !retirementUUIDRE.MatchString(uuid) {
		return "", "", retirementError("mysql selected identity mismatch")
	}
	raw, e := marshalSQLIdentity(uuid, sqlName)
	if e != nil {
		return "", "", retirementError("identity encoding failed")
	}
	sqlHash := retirementHash(raw)
	mongoHash, e := retirementMongoIdentity(q, client, mongoName)
	if e != nil {
		return "", "", e
	}
	return sqlHash, mongoHash, nil
}

func retirementMongoIdentity(ctx context.Context, client *mongo.Client, name string) (string, error) {
	var hello struct {
		SetName string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
	}
	if client == nil || name == "" || client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return "", retirementError("mongo cluster read failed")
	}
	if hello.SetName == "" || len(hello.Hosts) == 0 {
		return "", retirementError("mongo stable cluster identity insufficient")
	}
	sort.Strings(hello.Hosts)
	for i, host := range hello.Hosts {
		if host == "" || (i > 0 && host == hello.Hosts[i-1]) {
			return "", retirementError("mongo stable cluster identity malformed")
		}
	}
	raw, e := json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.SetName}, {Key: "hosts", Value: hello.Hosts}}, name})
	if e != nil {
		return "", retirementError("identity encoding failed")
	}
	return retirementHash(raw), nil
}

type pairStoreState struct {
	pristine bool
	version  uint
	dirty    bool
}

func schemaVisible(ctx context.Context, db retirementSQLReader, name string, pristine bool) error {
	q, cancel := retirementContext(ctx)
	defer cancel()
	rows, e := db.QueryContext(q, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil {
		return retirementError("mysql visibility unknown")
	}
	defer func() { _ = rows.Close() }()
	visible := false
	granted := map[string]bool{}
	for rows.Next() {
		var grant string
		if rows.Scan(&grant) != nil {
			return retirementError("mysql visibility unknown")
		}
		if strings.HasPrefix(grant, "REVOKE ") {
			return retirementError("mysql partial visibility rejected")
		}
		parts := strings.SplitN(grant, " ON ", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "GRANT ") {
			continue
		}
		scope := strings.SplitN(parts[1], " TO ", 2)[0]
		if scope != "*.*" && scope != "`"+strings.ReplaceAll(name, "`", "``")+"`.*" {
			continue
		}
		privileges := strings.Split(strings.TrimPrefix(parts[0], "GRANT "), ", ")
		for _, p := range privileges {
			granted[p] = true
			if p == "ALL PRIVILEGES" || (p == "SELECT" && !pristine) {
				visible = true
			}
		}
	}
	// MySQL may expand ALL PRIVILEGES into its explicit static privilege list.
	// Schema-wide routine grants make every routine visible regardless of definer.
	if pristine && !visible {
		visible = true
		for _, privilege := range []string{"SELECT", "SHOW VIEW", "TRIGGER", "EVENT", "CREATE ROUTINE", "ALTER ROUTINE", "EXECUTE"} {
			if !granted[privilege] {
				visible = false
			}
		}
	}
	if rows.Err() != nil || !visible {
		return retirementError("mysql schema-wide visibility unproven")
	}
	return nil
}
func observeSQLPair(ctx context.Context, db retirementSQLReader, name string) (pairStoreState, error) {
	var s pairStoreState
	if e := schemaVisible(ctx, db, name, false); e != nil {
		return s, e
	}
	q, cancel := retirementContext(ctx)
	defer cancel()
	rows, e := db.QueryContext(q, "SELECT TABLE_NAME,TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE()")
	if e != nil {
		return s, retirementError("mysql catalog read failed")
	}
	catalog := map[string]string{}
	for rows.Next() {
		var n, k string
		if rows.Scan(&n, &k) != nil {
			_ = rows.Close()
			return s, retirementError("mysql catalog read failed")
		}
		catalog[n] = k
	}
	e = rows.Err()
	closeErr := rows.Close()
	if e != nil || closeErr != nil {
		return s, retirementError("mysql catalog read failed")
	}
	if len(catalog) == 0 {
		if e := schemaVisible(ctx, db, name, true); e != nil {
			return s, retirementError("mysql pristine complete metadata visibility unproven")
		}
		var count int
		if db.QueryRowContext(q, "SELECT (SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema=DATABASE())+(SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE())+(SELECT COUNT(*) FROM information_schema.events WHERE event_schema=DATABASE())").Scan(&count) != nil || count != 0 {
			return s, retirementError("mysql pristine catalog unproven")
		}
		s.pristine = true
		return s, nil
	}
	if catalog["schema_migrations"] != "BASE TABLE" {
		return s, retirementError("mysql missing or wrong migration namespace")
	}
	columns, e := db.QueryContext(q, "SELECT COLUMN_NAME,DATA_TYPE,COLUMN_TYPE,IS_NULLABLE,COLUMN_KEY FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='schema_migrations' ORDER BY ORDINAL_POSITION")
	if e != nil {
		return s, retirementError("mysql head column shape unknown")
	}
	columnCount := 0
	shape := true
	for columns.Next() {
		var n, kind, columnType, nullable, key string
		if columns.Scan(&n, &kind, &columnType, &nullable, &key) != nil {
			shape = false
			break
		}
		switch columnCount {
		case 0:
			shape = shape && n == "version" && kind == "bigint" && nullable == "NO" && key == "PRI"
		case 1:
			shape = shape && n == "dirty" && kind == "tinyint" && columnType == "tinyint(1)" && nullable == "NO"
		default:
			shape = false
		}
		columnCount++
	}
	e = columns.Err()
	closeErr = columns.Close()
	if e != nil || closeErr != nil || !shape || columnCount != 2 {
		return s, retirementError("mysql head column shape rejected")
	}
	rows, e = db.QueryContext(q, "SELECT version,dirty FROM schema_migrations LIMIT 2")
	if e != nil {
		return s, retirementError("mysql head read failed")
	}
	count := 0
	for rows.Next() {
		var version uint
		var dirty int64
		if rows.Scan(&version, &dirty) != nil {
			_ = rows.Close()
			return s, retirementError("mysql head malformed")
		}
		s.version = version
		if dirty != 0 && dirty != 1 {
			_ = rows.Close()
			return s, retirementError("mysql head malformed")
		}
		s.dirty = dirty == 1
		count++
	}
	e = rows.Err()
	closeErr = rows.Close()
	if e != nil || closeErr != nil || count != 1 || s.dirty || (s.version != compatibilitySQLVersion-1 && s.version != compatibilitySQLVersion) {
		return s, retirementError("mysql head rejected")
	}
	for _, n := range retirementSQLNames {
		if _, ok := catalog[n]; ok {
			return s, retirementError("installed mysql retirement target present")
		}
	}
	return s, nil
}
func observeMongoPair(ctx context.Context, client *mongo.Client, name string) (pairStoreState, error) {
	var s pairStoreState
	q, cancel := retirementContext(ctx)
	defer cancel()
	db := client.Database(name)
	cursor, e := db.ListCollections(q, bson.D{}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return s, retirementError("mongo catalog visibility unknown")
	}
	var collections []bson.M
	e = cursor.All(q, &collections)
	closeErr := cursor.Close(q)
	if e != nil || closeErr != nil {
		return s, retirementError("mongo catalog read failed")
	}
	if len(collections) == 0 {
		var profile bson.Raw
		if db.RunCommand(q, bson.D{{Key: "profile", Value: -1}}).Decode(&profile) != nil {
			return s, retirementError("mongo pristine profiling state unknown")
		}
		was := profile.Lookup("was")
		levelKnown := false
		if value, ok := was.Int32OK(); ok {
			levelKnown = value == 0
		} else if value, ok := was.Int64OK(); ok {
			levelKnown = value == 0
		}
		if !levelKnown || profile.Lookup("filter").Type != 0 {
			return s, retirementError("mongo pristine profiling state rejected")
		}
		s.pristine = true
		return s, nil
	}
	head := false
	for _, c := range collections {
		n, ok := c["name"].(string)
		if !ok {
			return s, retirementError("mongo namespace malformed")
		}
		if n == "domain_event_outbox" {
			return s, retirementError("installed mongo retirement target present")
		}
		if n == "schema_migrations" {
			if c["type"] != "collection" {
				return s, retirementError("mongo head namespace rejected")
			}
			head = true
		}
	}
	if !head {
		return s, retirementError("mongo headless catalog rejected")
	}
	cursor, e = db.Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		return s, retirementError("mongo head read failed")
	}
	var heads []bson.M
	e = cursor.All(q, &heads)
	closeErr = cursor.Close(q)
	if e != nil || closeErr != nil || len(heads) != 1 {
		return s, retirementError("mongo head malformed")
	}
	switch v := heads[0]["version"].(type) {
	case int32:
		if v < 0 {
			return s, retirementError("mongo head malformed")
		}
		s.version = uint(v)
	case int64:
		if v < 0 {
			return s, retirementError("mongo head malformed")
		}
		s.version = uint(v)
	default:
		return s, retirementError("mongo head malformed")
	}
	dirty, ok := heads[0]["dirty"].(bool)
	s.dirty = dirty
	if !ok || dirty || (s.version != compatibilityMongoVersion-1 && s.version != compatibilityMongoVersion) {
		return s, retirementError("mongo head rejected")
	}
	return s, nil
}
func PreflightCompatibilityPair(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig) (*PairPreflight, error) {
	sqlHash, mongoHash, e := pairIdentity(ctx, db, client, cfg.MySQLDatabase, cfg.MongoDatabase)
	if e != nil {
		return nil, e
	}
	sqlState, e := observeSQLPair(ctx, db, cfg.MySQLDatabase)
	if e != nil {
		return nil, e
	}
	mongoState, e := observeMongoPair(ctx, client, cfg.MongoDatabase)
	if e != nil {
		return nil, e
	}
	if sqlState.pristine != mongoState.pristine {
		return nil, retirementError("mixed pristine pair rejected")
	}
	if !sqlState.pristine && !installedPairComplete(sqlState.version, mongoState.version) {
		return nil, retirementError("partial installed pair conflict")
	}
	p := &PairPreflight{sqlDB: db, mongo: client, config: cfg, pristine: sqlState.pristine, sqlHash: sqlHash, mongoHash: mongoHash, initialSQLVersion: sqlState.version, initialMongoVersion: mongoState.version}
	if p.pristine {
		a, e := readBootstrapAuthorization(cfg)
		if e != nil {
			return nil, e
		}
		if a.MySQLDatabase != cfg.MySQLDatabase || a.MongoDatabase != cfg.MongoDatabase || a.MySQLIdentitySHA256 != sqlHash || a.MongoClusterSHA256 != mongoHash {
			return nil, retirementError("bootstrap selected identity mismatch")
		}
		q, cancel := retirementContext(ctx)
		defer cancel()
		var hello struct {
			SetName string   `bson:"setName"`
			Hosts   []string `bson:"hosts"`
		}
		if client.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || hello.SetName == "" || len(hello.Hosts) == 0 {
			return nil, retirementError("bootstrap stable cluster identity insufficient")
		}
		p.authorization = a
	}
	return p, nil
}
func (p *PairPreflight) MySQLConfig(autoSeed bool) *Config {
	return &Config{Enabled: true, AutoSeed: autoSeed, Database: p.config.MySQLDatabase, retirementPair: p}
}
func (p *PairPreflight) MongoConfig(autoSeed bool) *Config {
	return &Config{Enabled: true, AutoSeed: autoSeed, Database: p.config.MongoDatabase, retirementPair: p}
}
func (p *PairPreflight) validateStart(ctx context.Context, backend Backend) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.migrationFailed {
		return retirementError("prior paired migration result failed or unknown")
	}
	a, b, e := pairIdentity(ctx, p.sqlReader(), p.mongo, p.config.MySQLDatabase, p.config.MongoDatabase)
	if e != nil || a != p.sqlHash || b != p.mongoHash {
		return retirementError("pair identity changed")
	}
	sqlState, e := observeSQLPair(ctx, p.sqlReader(), p.config.MySQLDatabase)
	if e != nil {
		return e
	}
	mongoState, e := observeMongoPair(ctx, p.mongo, p.config.MongoDatabase)
	if e != nil {
		return e
	}
	if !p.pristine {
		if sqlState.pristine || mongoState.pristine {
			return retirementError("installed pair state changed")
		}
		if !installedPairStepAllowed(p.initialSQLVersion, p.initialMongoVersion, sqlState.version, mongoState.version, backend, p.sqlDone) {
			return retirementError("installed pair phase changed or partial result unbound")
		}
		return nil
	}
	if backend == BackendMySQL {
		if !sqlState.pristine || !mongoState.pristine {
			return retirementError("pristine pair changed")
		}
		return consumeBootstrapAuthorization(p.config, p.authorization)
	}
	if !p.sqlDone || sqlState.pristine || sqlState.version != 100 || !mongoState.pristine {
		return retirementError("pristine complete sql up unproven")
	}
	return nil
}
func (p *PairPreflight) sqlFinished() { p.mu.Lock(); p.sqlDone = true; p.mu.Unlock() }
func readBootstrapAuthorization(cfg PairConfig) (*bootstrapAuthorization, error) {
	if !retirementHashRE.MatchString(cfg.BootstrapAuthorizationSHA256) || !filepath.IsAbs(cfg.BootstrapAuthorizationFile) || filepath.Clean(cfg.BootstrapAuthorizationFile) != cfg.BootstrapAuthorizationFile {
		return nil, retirementError("pristine bootstrap explicit authorization required")
	}
	path := cfg.BootstrapAuthorizationFile
	for x := path; ; x = filepath.Dir(x) {
		info, e := os.Lstat(x)
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, retirementError("bootstrap private path rejected")
		}
		if x != path && !info.IsDir() {
			return nil, retirementError("bootstrap private path rejected")
		}
		if x != path && info.Mode().Perm()&022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return nil, retirementError("bootstrap ancestor writable")
		}
		if x == filepath.Dir(x) {
			break
		}
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Sys().(*syscall.Stat_t).Nlink != 1 || (info.Sys().(*syscall.Stat_t).Uid != 0 && info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid())) || info.Size() < 1 || info.Size() > 32768 {
		return nil, retirementError("bootstrap private file rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, retirementError("bootstrap private file open rejected")
	}
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 || opened.Sys().(*syscall.Stat_t).Nlink != 1 {
		_ = f.Close()
		return nil, retirementError("bootstrap private file changed")
	}
	raw, e := io.ReadAll(io.LimitReader(f, 32769))
	after, statErr := f.Stat()
	closeErr := f.Close()
	current, pathErr := os.Lstat(path)
	if e != nil || statErr != nil || closeErr != nil || pathErr != nil || !os.SameFile(opened, after) || !os.SameFile(after, current) || after.Mode().Perm() != 0600 || after.Sys().(*syscall.Stat_t).Nlink != 1 || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || len(raw) > 32768 || retirementHash(raw) != cfg.BootstrapAuthorizationSHA256 {
		return nil, retirementError("bootstrap raw authorization hash mismatch")
	}
	if e = rejectRetirementDuplicateJSON(raw); e != nil {
		return nil, e
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var a bootstrapAuthorization
	if decoder.Decode(&a) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, retirementError("bootstrap authorization schema rejected")
	}
	expires, e := time.Parse(time.RFC3339, a.ExpiresAt)
	if e != nil || !strings.HasSuffix(a.ExpiresAt, "Z") || !time.Now().Before(expires) || expires.After(time.Now().Add(24*time.Hour)) || a.FormatVersion != 1 || a.Kind != "qs_compatibility_retirement_b_pristine_bootstrap" || !retirementOpRE.MatchString(a.OperationID) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(a.ApprovedSourceSHA) || a.ApprovedSourceSHA != cfg.ExpectedSourceSHA || !retirementHashRE.MatchString(a.ApprovalSummarySHA256) || a.MySQLVersion != 100 || a.MongoVersion != 39 || a.MigrationResourcesSHA256 != CompatibilityMigrationResourcesSHA256() || !retirementHashRE.MatchString(a.MySQLIdentitySHA256) || !retirementHashRE.MatchString(a.MongoClusterSHA256) {
		return nil, retirementError("bootstrap authorization binding rejected")
	}
	if _, e = os.Lstat(path + ".consumed"); !errors.Is(e, os.ErrNotExist) {
		return nil, retirementError("bootstrap authorization already consumed or unknown")
	}
	return &a, nil
}
func consumeBootstrapAuthorization(cfg PairConfig, a *bootstrapAuthorization) error {
	if a == nil {
		return retirementError("bootstrap authorization missing")
	}
	observed, e := readBootstrapAuthorization(cfg)
	if e != nil {
		return e
	}
	if *observed != *a {
		return retirementError("bootstrap authorization changed")
	}
	f, e := os.OpenFile(cfg.BootstrapAuthorizationFile+".consumed", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return retirementError("bootstrap authorization reuse rejected")
	}
	raw, _ := json.Marshal([]string{a.OperationID, cfg.BootstrapAuthorizationSHA256})
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return retirementError("bootstrap consumption receipt incomplete")
	}
	d, e := os.Open(filepath.Dir(cfg.BootstrapAuthorizationFile))
	if e != nil {
		return retirementError("bootstrap consumption receipt incomplete")
	}
	e = d.Sync()
	closeErr = d.Close()
	if e != nil || closeErr != nil {
		return retirementError("bootstrap consumption receipt incomplete")
	}
	return nil
}
func rejectRetirementDuplicateJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		v, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch v {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || seen[s] {
					return retirementError("bootstrap duplicate json key")
				}
				seen[s] = true
				if e = walk(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = walk(); e != nil {
					return e
				}
			}
		}
		_, e = d.Token()
		return e
	}
	if walk() != nil {
		return retirementError("bootstrap json rejected")
	}
	if _, e := d.Token(); e != io.EOF {
		return retirementError("bootstrap json rejected")
	}
	return nil
}

func installedPairComplete(sqlVersion, mongoVersion uint) bool {
	return (sqlVersion == 99 && mongoVersion == 38) || (sqlVersion == 100 && mongoVersion == 39)
}

// A 100/38 phase exists only within this one live pair after its own actual
// SQL success. A new ordinary preflight never adopts either partial pair.
func installedPairStepAllowed(initialSQL, initialMongo, actualSQL, actualMongo uint, backend Backend, sqlDone bool) bool {
	if initialSQL == 100 && initialMongo == 39 {
		return actualSQL == 100 && actualMongo == 39
	}
	if initialSQL != 99 || initialMongo != 38 {
		return false
	}
	if backend == BackendMySQL {
		return !sqlDone && actualSQL == 99 && actualMongo == 38
	}
	return backend == BackendMongo && sqlDone && actualSQL == 100 && actualMongo == 38
}
func (p *PairPreflight) migrationResultFailed() { p.mu.Lock(); p.migrationFailed = true; p.mu.Unlock() }
