package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mysql "github.com/go-sql-driver/mysql"
)

func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
func testErrorCategory(t *testing.T, err error, want string) {
	t.Helper()
	got, _ := errorCategory(err)
	if err == nil || got != want {
		t.Fatalf("error category = %q, want %q", got, want)
	}
}
func testLedger() ledger {
	l := ledger{FormatVersion: 1}
	for _, name := range targetTables() {
		l.Entries = append(l.Entries, dropEntry{Name: name, State: "pending"})
	}
	return l
}
func testConn(t *testing.T) (*sql.Conn, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, mock
}

func TestTargetAllowlistAndLocks(t *testing.T) {
	wantSources := []string{"assessment", "evaluation_outcome", "interpretation_admission_failure", "interpretation_attention_projection", "assessment_score", "statistics_assessment_fact", "statistics_assessment_daily", "statistics_org_snapshot", "domain_event_outbox", "runtime_checkpoint", "retry_event_hold"}
	wantSuffixes := []string{"20260827114947", "20260827131756"}
	names := targetTables()
	if len(names) != 22 {
		t.Fatal("target allowlist size changed")
	}
	seen := map[string]bool{}
	for i, name := range names {
		want := "cbpt_" + wantSources[i%11] + "_" + wantSuffixes[i/11]
		if name != want || seen[name] || !identifierRE.MatchString(name) {
			t.Fatal("unexpected target allowlist")
		}
		seen[name] = true
	}
	for _, mode := range []string{"READ", "WRITE"} {
		query := lockSQL("synthetic", mode)
		if strings.Contains(query, "*") || strings.Contains(query, "schema_migrations") {
			t.Fatal("target locks expanded outside the allowlist")
		}
		if strings.Count(query, " "+mode) != 22 {
			t.Fatal("wrong lock count")
		}
		for _, name := range names {
			if !strings.Contains(query, qi("synthetic")+"."+qi(name)+" "+mode) {
				t.Fatal("target missing from lock")
			}
		}
	}
}

func TestCanonicalDDLIgnoresMutableTableCounterOnly(t *testing.T) {
	a := "CREATE TABLE `t` (\n `id` bigint NOT NULL AUTO_INCREMENT,\n `v` varchar(99) DEFAULT 'AUTO_INCREMENT=456  a  b',\n PRIMARY KEY (`id`)\n) ENGINE=InnoDB AUTO_INCREMENT=12 DEFAULT CHARSET=utf8mb4"
	b := strings.Replace(a, "AUTO_INCREMENT=12 ", "AUTO_INCREMENT=99999 ", 1)
	if canonicalDDL(a) != canonicalDDL(b) {
		t.Fatal("mutable table counter changed fingerprint")
	}
	for _, change := range []string{
		strings.Replace(a, "bigint", "int", 1),
		strings.Replace(a, "AUTO_INCREMENT,", ",", 1),
		strings.Replace(a, "a  b", "a b", 1),
		strings.Replace(a, "utf8mb4", "utf8mb3", 1),
		strings.Replace(a, "456", "457", 1),
	} {
		if canonicalDDL(a) == canonicalDDL(change) {
			t.Fatal("semantic schema change was normalized away")
		}
	}
	if canonicalDDL("a\t\nb") != "a b" {
		t.Fatal("external whitespace was not normalized")
	}
	if canonicalDDL("COMMENT='it\\'s AUTO_INCREMENT=123'") != "COMMENT='it\\'s AUTO_INCREMENT=123'" {
		t.Fatal("quoted escape or literal was changed")
	}
}

func TestCellEncodingDistinguishesNullEmptyAndBoundaries(t *testing.T) {
	encode := func(cells ...[]byte) string {
		h := sha256.New()
		for _, cell := range cells {
			writeCell(h, cell)
		}
		return hex.EncodeToString(h.Sum(nil))
	}
	if encode(nil) == encode([]byte{}) || encode([]byte("ab"), []byte("c")) == encode([]byte("a"), []byte("bc")) {
		t.Fatal("content encoding conflated distinct values")
	}
	if hashText("ab", "c") == hashText("a", "bc") {
		t.Fatal("identity hash conflated distinct components")
	}
}

func TestDropExactDurableIntentBeforeEachDDL(t *testing.T) {
	l := testLedger()
	var events []string
	var durable ledger
	save := func(value ledger) error {
		durable = value
		durable.Entries = append([]dropEntry(nil), value.Entries...)
		if value.Complete {
			events = append(events, "complete")
		} else {
			for _, entry := range value.Entries {
				if entry.State == "drop_started" {
					events = append(events, "intent:"+entry.Name)
					return nil
				}
			}
			events = append(events, "ack")
		}
		return nil
	}
	drop := func(name string) error {
		found := false
		for _, entry := range durable.Entries {
			if entry.Name == name && entry.State == "drop_started" {
				found = true
			}
		}
		if !found {
			t.Fatal("DDL preceded durable intent")
		}
		events = append(events, "drop:"+name)
		return nil
	}
	if err := dropExact(&l, save, drop); err != nil {
		t.Fatal(err)
	}
	if len(events) != 67 || events[len(events)-1] != "complete" {
		t.Fatal("unexpected durable execution sequence")
	}
	for i, name := range targetTables() {
		if events[3*i] != "intent:"+name || events[3*i+1] != "drop:"+name || events[3*i+2] != "ack" {
			t.Fatal("DDL or acknowledgement reordered")
		}
	}
	r := receipt{}
	receiptLedger(&r, l)
	if !r.LedgerComplete || r.DroppedCount != 22 || r.PendingCount+r.UnknownCount+r.FailedCount != 0 {
		t.Fatal("successful ledger receipt is incomplete")
	}
}

func TestDropExactStopsAfterAmbiguousExecution(t *testing.T) {
	l := testLedger()
	calls := 0
	err := dropExact(&l, func(ledger) error { return nil }, func(string) error {
		calls++
		if calls == 3 {
			return fail("drop_execution_unknown", errors.New("private driver body"))
		}
		return nil
	})
	testErrorCategory(t, err, "drop_execution_unknown")
	r := receipt{}
	receiptLedger(&r, l)
	if calls != 3 || r.DroppedCount != 2 || r.UnknownCount != 1 || r.PendingCount != 19 || r.LedgerComplete {
		t.Fatal("ambiguous DDL did not stop later targets")
	}
}

func TestDropExactFsyncFailureStates(t *testing.T) {
	for _, tc := range []struct {
		name                                                      string
		failSave, wantDrop, wantDropped, wantUnknown, wantPending int
		category                                                  string
	}{
		{"initial_intent", 1, 0, 0, 1, 21, "drop_intent_fsync_failed"},
		{"second_intent", 3, 1, 1, 1, 20, "drop_intent_fsync_failed"},
		{"initial_ack", 2, 1, 0, 1, 21, "drop_ack_fsync_unknown"},
		{"third_ack", 6, 3, 2, 1, 19, "drop_ack_fsync_unknown"},
		{"complete", 45, 22, 22, 0, 0, "drop_complete_fsync_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := testLedger()
			saves, drops := 0, 0
			err := dropExact(&l, func(ledger) error {
				saves++
				if saves == tc.failSave {
					return errors.New("synthetic disk failure")
				}
				return nil
			}, func(string) error { drops++; return nil })
			testErrorCategory(t, err, tc.category)
			r := receipt{}
			receiptLedger(&r, l)
			if drops != tc.wantDrop || r.DroppedCount != tc.wantDropped || r.UnknownCount != tc.wantUnknown || r.PendingCount != tc.wantPending || r.LedgerComplete {
				t.Fatal("fsync failure claimed durable execution or continued DDL")
			}
		})
	}
}

func TestDropExactRefusesResume(t *testing.T) {
	for _, state := range []string{"dropped", "unknown", "drop_started"} {
		l := testLedger()
		l.Entries[0].State = state
		called := false
		err := dropExact(&l, func(ledger) error { called = true; return nil }, func(string) error { called = true; return nil })
		testErrorCategory(t, err, "drop_ledger_state_invalid")
		if called {
			t.Fatal("resumed existing ledger")
		}
	}
}

func TestSessionIdentityLossBlocksLock(t *testing.T) {
	c, mock := testConn(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT CONNECTION_ID()")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	err := takeLock(context.Background(), c, 41, "synthetic", "WRITE")
	testErrorCategory(t, err, "lock_session_lost")
}

func TestMigrationHeadMustBeOneClean95(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values [][2]any
		ok     bool
	}{
		{"clean", [][2]any{{95, false}}, true},
		{"empty", nil, false},
		{"duplicate", [][2]any{{95, false}, {95, false}}, false},
		{"old", [][2]any{{94, false}}, false},
		{"dirty", [][2]any{{95, true}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, mock := testConn(t)
			rows := sqlmock.NewRows([]string{"version", "dirty"})
			for _, value := range tc.values {
				rows.AddRow(value[0], value[1])
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT version, dirty FROM schema_migrations LIMIT 2")).WillReturnRows(rows)
			err := migrationHead(context.Background(), c)
			if tc.ok {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				testErrorCategory(t, err, "migration_head_mismatch")
			}
		})
	}
}

func TestAuditFingerprintDoesNotReadContent(t *testing.T) {
	c, mock := testConn(t)
	name := targetTables()[0]
	ddl := "CREATE TABLE " + qi(name) + " (`id` bigint NOT NULL, PRIMARY KEY (`id`)) ENGINE=InnoDB"
	mock.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE " + qi("synthetic") + "." + qi(name))).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(name, ddl))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name, data_type, extra, COALESCE(character_set_name,''), COALESCE(collation_name,'') FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position LIMIT 257")).WithArgs("synthetic", name).WillReturnRows(sqlmock.NewRows([]string{"name", "type", "extra", "charset", "collation"}).AddRow("id", "bigint", "", "", ""))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name='PRIMARY' ORDER BY seq_in_index LIMIT 257")).WithArgs("synthetic", name).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("id"))
	fp, err := fingerprint(context.Background(), c, "synthetic", name, false)
	if err != nil {
		t.Fatal(err)
	}
	if fp.ContentMeasured || fp.ContentSHA256 != "" || fp.Rows != 0 {
		t.Fatal("audit claimed content measurement")
	}
}

func TestFingerprintStreamsOrderedNullAwareRows(t *testing.T) {
	c, mock := testConn(t)
	name := targetTables()[0]
	ddl := "CREATE TABLE " + qi(name) + " (`id` bigint NOT NULL, `value` blob, PRIMARY KEY (`id`)) ENGINE=InnoDB"
	mock.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE " + qi("synthetic") + "." + qi(name))).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(name, ddl))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name, data_type, extra, COALESCE(character_set_name,''), COALESCE(collation_name,'') FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position LIMIT 257")).WithArgs("synthetic", name).WillReturnRows(sqlmock.NewRows([]string{"name", "type", "extra", "charset", "collation"}).AddRow("id", "bigint", "", "", "").AddRow("value", "blob", "", "", ""))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name='PRIMARY' ORDER BY seq_in_index LIMIT 257")).WithArgs("synthetic", name).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("id"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`value` FROM `synthetic`." + qi(name) + " ORDER BY `id`")).WillReturnRows(sqlmock.NewRows([]string{"id", "value"}).AddRow("1", nil).AddRow("2", []byte{}).AddRow("3", []byte{0, 1, 255}))
	fp, err := fingerprint(context.Background(), c, "synthetic", name, true)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	writeUint(h, 2)
	for _, pair := range [][2][]byte{{[]byte("1"), nil}, {[]byte("2"), {}}, {[]byte("3"), {0, 1, 255}}} {
		writeCell(h, pair[0])
		writeCell(h, pair[1])
	}
	writeUint(h, 3)
	if !fp.ContentMeasured || fp.Rows != 3 || fp.ContentSHA256 != hex.EncodeToString(h.Sum(nil)) {
		t.Fatal("streamed content fingerprint mismatch")
	}
}

func TestDumpArgumentsAreExactAndCredentialsAreOnlyPrivateFile(t *testing.T) {
	args := dumpArgs(options{defaultsFile: "/connection/mysql.cnf"}, "synthetic")
	if args[0] != "--defaults-file=/connection/mysql.cnf" {
		t.Fatal("defaults-file must be the first client option")
	}
	required := []string{"--single-transaction", "--quick", "--set-gtid-purged=OFF", "--no-tablespaces", "--skip-triggers", "--skip-routines", "--skip-events", "--hex-blob", "--skip-add-locks", "--skip-add-drop-table", "--skip-lock-tables"}
	for _, flag := range required {
		found := false
		for _, arg := range args {
			found = found || arg == flag
		}
		if !found {
			t.Fatal("missing restrictive dump option")
		}
	}
	if args[len(args)-23] != "synthetic" {
		t.Fatal("unexpected dump database scope")
	}
	for i, name := range targetTables() {
		if args[len(args)-22+i] != name {
			t.Fatal("dump target list differs from allowlist")
		}
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--password") || strings.HasPrefix(arg, "--user") || arg == "--all-databases" || arg == "--databases" {
			t.Fatal("dump credentials or scope appeared in arguments")
		}
	}
}

func TestCompressedDumpWriterHardLimit(t *testing.T) {
	var b bytes.Buffer
	w := &cappedWriter{w: &b, limit: 3}
	if n, err := w.Write([]byte("ab")); n != 2 || err != nil {
		t.Fatal("unexpected first write")
	}
	if n, err := w.Write([]byte("cd")); n != 0 || err == nil {
		t.Fatal("cap was exceeded")
	}
	if b.String() != "ab" {
		t.Fatal("overflow bytes were persisted")
	}
}

func TestPrivateJSONRejectsAmbiguityAndUnsafePermissions(t *testing.T) {
	dir := testPrivateDir(t)
	valid := `{"host":"localhost","port":3306,"user":"synthetic","password":"synthetic","database":"synthetic"}`
	for _, input := range []string{
		strings.Replace(valid, `"host":`, `"Host":`, 1),
		strings.Replace(valid, `"host":"localhost",`, `"host":"localhost","host":"other",`, 1),
		strings.Replace(valid, `"database":"synthetic"`, `"database":"synthetic","unexpected":"value"`, 1),
		strings.Replace(valid, `,"database":"synthetic"`, "", 1),
		valid + " {}",
		strings.Replace(valid, `"port":3306`, `"port":"3306"`, 1),
	} {
		path := filepath.Join(dir, "connection.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		var cfg connectionConfig
		_, err := readPrivateJSON(path, &cfg)
		testErrorCategory(t, err, "private_json_invalid")
	}
	path := filepath.Join(dir, "connection.json")
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	var cfg connectionConfig
	if _, err := readPrivateJSON(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateJSON(path, &cfg); err == nil {
		t.Fatal("public credentials accepted")
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := privateFile(link); err == nil {
		t.Fatal("credential symlink accepted")
	}
}

func TestAtomicArtifactsPrivateAndCannotOverwriteProof(t *testing.T) {
	dir := testPrivateDir(t)
	path := filepath.Join(dir, "proof.json")
	value := connectionConfig{Host: "localhost", Port: 3306, User: "synthetic", Password: "synthetic", Database: "synthetic"}
	if err := writeAtomicJSON(path, value, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("artifact permissions are not private")
	}
	testErrorCategory(t, writeAtomicJSON(path, value, false), "artifact_already_exists")
	if err := writeAtomicJSON(path, value, true); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary artifacts remained")
	}
}

func TestMySQLDefaultsSpecialCharacterBinding(t *testing.T) {
	dir := testPrivateDir(t)
	cfg := connectionConfig{Host: "localhost", Port: 3306, User: "synthetic", Password: "a\\b\"c'd\n\r\t\b #= z", Database: "synthetic"}
	text := "[client]\nhost=\"localhost\"\nport=\"3306\"\nuser=\"synthetic\"\npassword=\"a\\\\b\\\"c\\'d\\n\\r\\t\\b #= z\"\n"
	path := filepath.Join(dir, "mysql.cnf")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkDefaultsFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		text + "password=\"other\"\n",
		strings.Replace(text, "port=\"3306\"", "port=3306", 1),
		text + "database=\"synthetic\"\n",
		strings.Replace(text, "a\\\\b", "a\\xb", 1),
	} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if err := checkDefaultsFile(path, cfg); err == nil {
			t.Fatal("unsafe or mismatched defaults accepted")
		}
	}
}

func testManifest(t *testing.T) (options, manifest) {
	t.Helper()
	dir := testPrivateDir(t)
	o := options{archiveDir: dir, sourceSHA: strings.Repeat("a", 40)}
	m := manifest{FormatVersion: 1, OperationID: filepath.Base(dir), SourceSHA: o.sourceSHA, SourceServerUUID: "11111111-1111-1111-1111-111111111111", SourceTargetHash: strings.Repeat("b", 64), MigrationVersion: 95, ArchiveDropEligible: true, DumpFile: "dump.sql.gz", DumpSHA256: strings.Repeat("c", 64)}
	for _, name := range targetTables() {
		ddl := "CREATE TABLE " + qi(name) + " (`id` bigint NOT NULL, PRIMARY KEY (`id`)) ENGINE=InnoDB"
		m.Tables = append(m.Tables, tableFingerprint{ContentMeasured: true, Name: name, DDL: ddl, SchemaSHA256: hashText(canonicalDDL(ddl)), ContentSHA256: hashText("synthetic"), PK: []string{"id"}, Columns: []column{{Name: "id", DataType: "bigint"}}})
	}
	names := append(append([]string(nil), sourceTables...), "schema_migrations")
	for i := 0; len(names) < 66; i++ {
		names = append(names, fmt.Sprintf("synthetic_%02d", i))
	}
	for _, name := range names {
		ddl := "CREATE TABLE " + qi(name) + " (`id` bigint NOT NULL, PRIMARY KEY (`id`)) ENGINE=InnoDB"
		m.NonTargets = append(m.NonTargets, nonTarget{Name: name, Kind: "BASE TABLE", DDL: ddl, SchemaSHA256: hashText(canonicalDDL(ddl))})
	}
	return o, m
}

func TestManifestRejectsScopeSchemaAndMeasuredContentTampering(t *testing.T) {
	o, valid := testManifest(t)
	if err := validateManifest(valid, o); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*manifest){
		func(m *manifest) { m.Tables = m.Tables[:21] },
		func(m *manifest) { m.Tables[0].Name = "assessment" },
		func(m *manifest) { m.Tables[1].Name = m.Tables[0].Name },
		func(m *manifest) { m.Tables[0].ContentMeasured = false },
		func(m *manifest) { m.Tables[0].DDL += " COMMENT='changed'" },
		func(m *manifest) { m.Tables[0].PK = []string{"missing"} },
		func(m *manifest) { m.NonTargets = m.NonTargets[:65] },
		func(m *manifest) { m.MigrationDirty = true },
		func(m *manifest) { m.SourceSHA = strings.Repeat("d", 40) },
		func(m *manifest) { m.DumpFile = "../other.sql.gz" },
	} {
		b, _ := json.Marshal(valid)
		var mutated manifest
		if err := json.Unmarshal(b, &mutated); err != nil {
			t.Fatal(err)
		}
		change(&mutated)
		if err := validateManifest(mutated, o); err == nil {
			t.Fatal("tampered manifest accepted")
		}
	}
}

func testWriteManifest(t *testing.T, o options, m manifest) (manifest, string) {
	t.Helper()
	dump := filepath.Join(o.archiveDir, "dump.sql.gz")
	if err := os.WriteFile(dump, []byte("synthetic archive bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	var err error
	m.DumpSHA256, err = fileHash(dump, maxDumpBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeAtomicJSON(filepath.Join(o.archiveDir, "manifest.json"), m, false); err != nil {
		t.Fatal(err)
	}
	_, hash, err := loadManifest(o)
	if err != nil {
		t.Fatal(err)
	}
	return m, hash
}

func TestManifestArchiveBytesMustMatch(t *testing.T) {
	o, m := testManifest(t)
	testWriteManifest(t, o, m)
	if err := os.WriteFile(filepath.Join(o.archiveDir, "dump.sql.gz"), []byte("tampered archive"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadManifest(o)
	testErrorCategory(t, err, "archive_dump_mismatch")
}

func TestRestoreMarkerRefusesSourceAndWrongUUIDBeforeMetadataRead(t *testing.T) {
	for _, sourceUUID := range []bool{true, false} {
		o, m := testManifest(t)
		m, _ = testWriteManifest(t, o, m)
		nonce := strings.Repeat("d", 32)
		uuid := "22222222-2222-2222-2222-222222222222"
		markerUUID := uuid
		if sourceUUID {
			uuid = m.SourceServerUUID
			markerUUID = uuid
		} else {
			markerUUID = "33333333-3333-3333-3333-333333333333"
		}
		marker := restoreMarker{FormatVersion: 1, OperationID: m.OperationID, Nonce: nonce, ContainerOwner: strings.Repeat("e", 32), SourceServerUUID: m.SourceServerUUID, RestoreServerUUID: markerUUID, RestoreDatabase: "cbpt_restore_" + nonce, SourceTargetHash: m.SourceTargetHash}
		o.marker = filepath.Join(o.archiveDir, "marker.json")
		if err := writeAtomicJSON(o.marker, marker, false); err != nil {
			t.Fatal(err)
		}
		err := verifyRestored(context.Background(), nil, connectionConfig{Database: marker.RestoreDatabase}, uuid, o, &receipt{})
		testErrorCategory(t, err, "restore_target_marker_invalid")
	}
}

func TestRestoredProofBindsAllArchiveIdentity(t *testing.T) {
	o, m := testManifest(t)
	m, mh := testWriteManifest(t, o, m)
	valid := restoreProof{FormatVersion: 1, OperationID: m.OperationID, SourceSHA: m.SourceSHA, SourceTargetHash: m.SourceTargetHash, ManifestSHA256: mh, DumpSHA256: m.DumpSHA256, MarkerSHA256: strings.Repeat("d", 64), RestoreServerUUID: "22222222-2222-2222-2222-222222222222", Verified: true, TargetTableCount: 22}
	path := filepath.Join(o.archiveDir, "restore-proof.json")
	if err := writeAtomicJSON(path, valid, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadProof(o, m, mh); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*restoreProof){
		func(p *restoreProof) { p.Verified = false },
		func(p *restoreProof) { p.ManifestSHA256 = strings.Repeat("e", 64) },
		func(p *restoreProof) { p.DumpSHA256 = strings.Repeat("e", 64) },
		func(p *restoreProof) { p.SourceSHA = strings.Repeat("e", 40) },
		func(p *restoreProof) { p.RestoreServerUUID = m.SourceServerUUID },
		func(p *restoreProof) { p.TargetTableCount = 21 },
	} {
		p := valid
		mutate(&p)
		if err := writeAtomicJSON(path, p, true); err != nil {
			t.Fatal(err)
		}
		_, _, err := loadProof(o, m, mh)
		testErrorCategory(t, err, "restore_proof_invalid")
	}
}

func TestSafeErrorCategoryDoesNotExposeDriverBody(t *testing.T) {
	raw := &mysql.MySQLError{Number: 1142, Message: "private schema and credentials must not escape"}
	err := fail("metadata_visibility_blocked", raw)
	category, code := errorCategory(err)
	if category != "metadata_visibility_blocked" || code != 1142 || strings.Contains(err.Error(), "private") {
		t.Fatal("error envelope leaked driver body or lost numeric code")
	}
	category, code = errorCategory(errors.New("private raw error"))
	if category != "internal_failure" || code != 0 {
		t.Fatal("unknown error was not sanitized")
	}
}

func TestVerifyRemovedRequiresConfirmedLedgerAndPreservesNonTargetSchema(t *testing.T) {
	for _, tc := range []struct {
		name          string
		state         string
		complete      bool
		schemaChanged bool
		ok            bool
	}{
		{"confirmed", "dropped", true, false, true},
		{"absence_unknown", "unknown", false, false, false},
		{"absence_pending", "pending", false, false, false},
		{"complete_flag_unknown", "unknown", true, false, false},
		{"all_ack_no_complete", "dropped", false, false, false},
		{"non_target_changed", "dropped", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, m := testManifest(t)
			m, mh := testWriteManifest(t, o, m)
			proof := restoreProof{FormatVersion: 1, OperationID: m.OperationID, SourceSHA: m.SourceSHA, SourceTargetHash: m.SourceTargetHash, ManifestSHA256: mh, DumpSHA256: m.DumpSHA256, MarkerSHA256: strings.Repeat("d", 64), RestoreServerUUID: "22222222-2222-2222-2222-222222222222", Verified: true, TargetTableCount: 22}
			if err := writeAtomicJSON(filepath.Join(o.archiveDir, "restore-proof.json"), proof, false); err != nil {
				t.Fatal(err)
			}
			_, ph, err := loadProof(o, m, mh)
			if err != nil {
				t.Fatal(err)
			}
			l := testLedger()
			l.OperationID, l.SourceSHA, l.SourceTargetHash, l.ManifestSHA256, l.ProofSHA256 = m.OperationID, m.SourceSHA, m.SourceTargetHash, mh, ph
			l.Complete = tc.complete
			for i := range l.Entries {
				l.Entries[i].State = "dropped"
			}
			l.Entries[0].State = tc.state
			if err := writeAtomicJSON(filepath.Join(o.archiveDir, "drop-ledger.json"), l, false); err != nil {
				t.Fatal(err)
			}
			c, mock := testConn(t)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT version, dirty FROM schema_migrations LIMIT 2")).WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).AddRow(95, false))
			catalog := sqlmock.NewRows([]string{"name", "kind", "engine"})
			for _, table := range m.NonTargets {
				catalog.AddRow(table.Name, table.Kind, "InnoDB")
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT table_name, table_type, COALESCE(engine,'') FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name LIMIT 1001")).WithArgs("synthetic").WillReturnRows(catalog)
			for i, table := range m.NonTargets {
				ddl := table.DDL + " AUTO_INCREMENT=998"
				if tc.schemaChanged && i == 0 {
					ddl = strings.Replace(ddl, "bigint", "int", 1)
				}
				mock.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE " + qi("synthetic") + "." + qi(table.Name))).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(table.Name, ddl))
			}
			r := receipt{TargetTableCount: 22}
			err = verifyRemoved(context.Background(), c, connectionConfig{Database: "synthetic"}, m.SourceServerUUID, m.SourceTargetHash, o, &r)
			if tc.ok {
				if err != nil || r.Status != "ok" || r.RemainingTargetCount == nil || *r.RemainingTargetCount != 0 || !r.LedgerComplete || r.DroppedCount != 22 || r.TargetTableCount != 22 {
					t.Fatal("confirmed removal receipt inconsistent")
				}
			} else if tc.schemaChanged {
				testErrorCategory(t, err, "non_target_schema_changed")
			} else {
				testErrorCategory(t, err, "drop_ledger_unconfirmed")
				if r.Status != "unknown" || r.RemainingTargetCount == nil || *r.RemainingTargetCount != 0 {
					t.Fatal("absence was incorrectly represented as execution confirmation")
				}
			}
		})
	}
}

func TestCanonicalColumnCharsetEquivalentDisplayOnly(t *testing.T) {
	source := "CREATE TABLE `t` (`id` bigint NOT NULL, `value` varchar(32) COLLATE utf8mb4_bin DEFAULT 'character set utf8mb4 collate utf8mb4_bin', `generated` varchar(64) COLLATE utf8mb4_bin GENERATED ALWAYS AS (concat(`value`,'x')) VIRTUAL, PRIMARY KEY (`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci"
	restored := strings.ReplaceAll(source, "varchar(32) COLLATE", "varchar(32) CHARACTER SET utf8mb4 COLLATE")
	restored = strings.ReplaceAll(restored, "varchar(64) COLLATE", "varchar(64) CHARACTER SET utf8mb4 COLLATE")
	if canonicalDDL(source) != canonicalDDL(restored) {
		t.Fatal("equivalent explicit column charset was not normalized")
	}
	for _, changed := range []string{
		strings.Replace(restored, "COLLATE utf8mb4_bin", "COLLATE utf8mb4_unicode_ci", 1),
		strings.Replace(restored, "SET utf8mb4 COLLATE", "SET latin1 COLLATE", 1),
		strings.Replace(restored, "DEFAULT CHARSET=utf8mb4", "DEFAULT CHARSET=latin1", 1),
		strings.Replace(restored, "concat(`value`,'x')", "concat(`value`,'y')", 1),
		strings.Replace(restored, "'character set utf8mb4 collate utf8mb4_bin'", "'collate utf8mb4_bin'", 1),
	} {
		if canonicalDDL(source) == canonicalDDL(changed) {
			t.Fatal("actual charset, collation, expression, or literal change was normalized away")
		}
	}
	for _, untouched := range []string{
		"CREATE TABLE `t` (`v` varchar(32) GENERATED ALWAYS AS (CAST(`x` AS CHAR CHARACTER SET utf8mb4 COLLATE utf8mb4_bin)))",
		"CREATE TABLE `t` (`character set utf8mb4 collate utf8mb4_bin` int)",
		"CREATE TABLE `t` (`v` varchar(32) COMMENT 'CHARACTER SET utf8mb4 COLLATE utf8mb4_bin')",
		"CREATE TABLE `t` (`v` varchar(32) /* CHARACTER SET utf8mb4 COLLATE utf8mb4_bin ( ) */)",
		"CREATE TABLE `t` (`v` varchar(32)) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin",
		"CREATE TABLE `t` (`v` varchar(32) CHARACTER SET latin1 COLLATE utf8mb4_bin)",
	} {
		if canonicalDDL(untouched) != untouched {
			t.Fatal("normalization escaped a column declaration or matching charset pair")
		}
	}
}

func TestEffectiveColumnCharsetAndCollationIndependentlyBindSchema(t *testing.T) {
	a := tableFingerprint{ContentMeasured: true, Name: targetTables()[0], SchemaSHA256: strings.Repeat("a", 64), ContentSHA256: strings.Repeat("b", 64), PK: []string{"id"}, Columns: []column{{Name: "id", DataType: "varchar", CharacterSet: "utf8mb4", Collation: "utf8mb4_bin"}}}
	for _, tc := range []struct{ charset, collation string }{
		{"utf8mb4", "utf8mb4_unicode_ci"},
		{"latin1", "utf8mb4_bin"},
		{"", "utf8mb4_bin"},
	} {
		b := a
		b.Columns = append([]column(nil), a.Columns...)
		b.Columns[0].CharacterSet, b.Columns[0].Collation = tc.charset, tc.collation
		if sameTable(a, b) {
			t.Fatal("identical DDL/content hashes hid effective charset or collation change")
		}
	}
}
