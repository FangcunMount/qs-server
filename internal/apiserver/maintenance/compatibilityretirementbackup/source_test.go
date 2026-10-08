package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPtr(v string) *string { return &v }
func testColumns(index int) retirement.SQLColumns {
	names := [][]string{{"id", "event_id", "event_type", "aggregate_type", "aggregate_id", "org_id", "topic_name", "payload_json", "status", "attempt_count", "retry_disposition", "next_attempt_at", "last_error", "last_error_kind", "manual_replay_request_id", "created_at", "updated_at", "published_at"}, {"command_id", "request_id", "kind", "payload", "payload_hash", "delivered", "attempts", "available_at"}, {"command_id", "request_id", "source_kind", "source_payload", "source_payload_hash", "source_attempts", "source_available_at", "source_original_time", "messaging_body_sha256", "transferred_at"}}
	var cols retirement.SQLColumns
	for _, n := range names[index] {
		cols = append(cols, []*string{testPtr(n), testPtr("text"), testPtr("YES"), nil, testPtr(""), nil})
	}
	return cols
}
func testSQLCopy(t *testing.T, index int, cells [][][]byte, nulls [][]bool) ([]byte, SourceSnapshot, retirement.SQLColumns) {
	t.Helper()
	columns := testColumns(index)
	b := retirement.SourceBoundary{Database: "mysql", Name: targetNames[index], Kind: "base_table", Present: true, Empty: len(cells) == 0, PKType: "string", SchemaHash: strings.Repeat("a", 64), IdentityHash: strings.Repeat("b", 64)}
	if index == 0 {
		b.PKType = "uint64"
	}
	if len(cells) > 0 {
		b.UpperToken = base64.StdEncoding.EncodeToString(cells[len(cells)-1][0])
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	if enc.Encode(sqlHeader{Protocol: retirement.SQLSourceProtocol, Columns: columns, Boundary: b}) != nil {
		t.Fatal("fixture header failed")
	}
	h := sha256.New()
	frame(h, []byte(jsonSHA(columns)), false)
	var size uint64
	for i, row := range cells {
		line := make([]*string, len(row))
		for j, v := range row {
			if !nulls[i][j] {
				line[j] = testPtr(base64.StdEncoding.EncodeToString(v))
				size += uint64(len(v))
			}
			frame(h, v, nulls[i][j])
		}
		if enc.Encode(line) != nil {
			t.Fatal("fixture row failed")
		}
	}
	s := SourceSnapshot{Database: b.Database, Name: b.Name, Kind: b.Kind, Present: true, Complete: true, Records: uint64(len(cells)), Bytes: size, SchemaHash: b.SchemaHash, IdentityHash: b.IdentityHash, DataHash: hex.EncodeToString(h.Sum(nil)), Boundary: b, Passes: 2}
	return out.Bytes(), s, columns
}
func TestRawSQLPreservesNullEmptyLargeIDAndEOF(t *testing.T) {
	row := make([][]byte, 18)
	nulls := make([]bool, 18)
	for i := range row {
		row[i] = []byte("fixture")
	}
	row[0] = []byte("18446744073709551615")
	row[7] = []byte{}
	row[12] = nil
	nulls[12] = true
	data, s, columns := testSQLCopy(t, 0, [][][]byte{row}, [][]bool{nulls})
	r, e := newRawReader(context.Background(), bytes.NewReader(data), 0, s, columns)
	if e != nil {
		t.Fatal("reader rejected valid fixture")
	}
	v, e := r.next()
	if e != nil || v.nulls[7] || !v.nulls[12] || len(v.cells[7]) != 0 || string(v.cells[0]) != "18446744073709551615" {
		t.Fatal("original NULL/empty/uint64 lost")
	}
	if r.done {
		t.Fatal("premature receipt")
	}
	if _, e = r.next(); e != io.EOF || !r.done {
		t.Fatal("actual EOF not verified")
	}
}
func TestRawSQLRejectsPartialTrailingChangedOrIncorrectApproval(t *testing.T) {
	row := make([][]byte, 18)
	nulls := make([]bool, 18)
	for i := range row {
		row[i] = []byte("fixture")
	}
	row[0] = []byte("1")
	data, s, columns := testSQLCopy(t, 0, [][][]byte{row}, [][]bool{nulls})
	cases := []struct {
		name     string
		raw      []byte
		expected SourceSnapshot
	}{{"truncated", data[:len(data)-12], s}, {"extra_row", append(append([]byte(nil), data...), data[bytes.IndexByte(data, '\n')+1:]...), s}, {"body_changed", bytes.Replace(data, []byte(base64.StdEncoding.EncodeToString([]byte("fixture"))), []byte(base64.StdEncoding.EncodeToString([]byte("changed"))), 1), s}, {"incorrect_hash", data, s}, {"nextcycle", data, s}, {"wrong_identity", data, s}}
	cases[3].expected.DataHash = strings.Repeat("c", 64)
	cases[4].expected.NextCycle = true
	cases[5].expected.Boundary.IdentityHash = strings.Repeat("d", 64)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, e := newRawReader(context.Background(), bytes.NewReader(c.raw), 0, c.expected, columns)
			if e == nil {
				for {
					_, e = r.next()
					if e != nil {
						break
					}
				}
			}
			if e == io.EOF || e == nil {
				t.Fatal("invalid source accepted")
			}
			if strings.Contains(e.Error(), "fixture") {
				t.Fatal("private source leaked")
			}
		})
	}
}
func TestEmptyPresentSQLCopyStillRequiresRealHeaderAndEOF(t *testing.T) {
	data, s, cols := testSQLCopy(t, 2, nil, nil)
	r, e := newRawReader(context.Background(), bytes.NewReader(data), 2, s, cols)
	if e != nil {
		t.Fatal("empty present schema rejected")
	}
	if _, e = r.next(); e != io.EOF || !r.done {
		t.Fatal("zero-row proof missing")
	}
	s.Present = false
	if _, e = newRawReader(context.Background(), bytes.NewReader(data), 2, s, cols); e == nil {
		t.Fatal("absence mistaken for present empty")
	}
}
func TestOrderedMongoHashDistinguishesCompoundKeyOrderLostByInventoryMap(t *testing.T) {
	id := bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "name", Value: "_id_"}}
	col, _ := bson.Marshal(bson.D{{Key: "name", Value: "domain_event_outbox"}, {Key: "type", Value: "collection"}, {Key: "options", Value: bson.D{}}, {Key: "info", Value: bson.D{{Key: "readOnly", Value: false}, {Key: "uuid", Value: []byte("instance uuid")}}}, {Key: "idIndex", Value: id}})
	one, _ := bson.Marshal(bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "status", Value: int32(1)}, {Key: "created_at", Value: int32(-1)}}}, {Key: "name", Value: "ordered"}})
	two, _ := bson.Marshal(bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "created_at", Value: int32(-1)}, {Key: "status", Value: int32(1)}}}, {Key: "name", Value: "ordered"}})
	a := MongoStructure{Collection: col, Indexes: [][]byte{one}}
	b := MongoStructure{Collection: col, Indexes: [][]byte{two}}
	x, e := mongoTargetCanonical(a)
	if e != nil {
		t.Fatal("fixture structure invalid")
	}
	y, e := mongoTargetCanonical(b)
	if e != nil || jsonSHA(x) != jsonSHA(y) {
		t.Fatal("inventory map loss not reproduced")
	}
	if jsonSHA(a) == jsonSHA(b) || schemaEqual(a, b) {
		t.Fatal("ordered BSON proof lost compound order")
	}
}
func TestStrictJSONRejectsDuplicateAliasUnknownAndTrailing(t *testing.T) {
	for _, raw := range []string{`{"protocol":"v2","protocol":"other"}`, `{"Protocol":"v2"}`, `{"protocol":"v2","unexpected":"PRIVATE_SECRET"}`, `{"protocol":"v2"} {"body":"PRIVATE_SECRET"}`} {
		var v sqlHeader
		if exactJSON([]byte(raw), &v) == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
}
func TestPrivateAssetsNeverOverwriteOrFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("fixture directory failed")
	}
	p := filepath.Join(dir, "original")
	if writePrivate(p, []byte("PRIVATE_SECRET")) != nil {
		t.Fatal("fixture write failed")
	}
	if writePrivate(p, []byte("replacement")) == nil {
		t.Fatal("existing asset overwritten")
	}
	link := filepath.Join(dir, "link")
	if os.Symlink(p, link) != nil {
		t.Fatal("fixture link failed")
	}
	if f, e := openPrivateFile(link); e == nil {
		_ = f.Close()
		t.Fatal("symlink asset accepted")
	}
	raw, e := os.ReadFile(p)
	if e != nil || string(raw) != "PRIVATE_SECRET" {
		t.Fatal("original changed")
	}
}
func TestPrivateSummaryCannotAdvertiseDropOrProductionProof(t *testing.T) {
	s := (*Archive)(nil).Summary()
	raw, e := json.Marshal(s)
	if e != nil || s.DropReady || s.TargetCount != 0 || !s.PurgeAfterAcceptanceRequired || bytes.Contains(raw, []byte("PRIVATE_SECRET")) {
		t.Fatal("unsafe summary")
	}
	if _, e = json.Marshal(&Archive{}); !errors.Is(e, ErrSerialization) {
		t.Fatal("private archive serializable")
	}
}
func TestCancelledReadIsNotComplete(t *testing.T) {
	data, s, cols := testSQLCopy(t, 0, nil, nil)
	ctx, c := context.WithCancel(context.Background())
	c()
	if _, e := newRawReader(ctx, bytes.NewReader(data), 0, s, cols); e == nil {
		t.Fatal("cancelled copy accepted")
	}
	if _, _, e := boundedRestore(ctx); e == nil {
		t.Fatal("cancelled restore accepted")
	}
}
func TestMongoTrueBSONFramingTypeAndTruncation(t *testing.T) {
	raw, _ := bson.Marshal(bson.D{{Key: "_id", Value: int64(9223372036854775807)}, {Key: "blob", Value: []byte{0, 255}}, {Key: "nullable", Value: nil}})
	token, _ := bson.Marshal(bson.D{{Key: "_id", Value: int64(9223372036854775807)}})
	b := retirement.SourceBoundary{Database: "mongodb", Name: "domain_event_outbox", Kind: "collection", Present: true, PKType: "long", UpperToken: base64.StdEncoding.EncodeToString(token), SchemaHash: strings.Repeat("a", 64), IdentityHash: strings.Repeat("b", 64)}
	h := sha256.New()
	frame(h, raw, false)
	s := SourceSnapshot{Present: true, Complete: true, Records: 1, Bytes: uint64(len(raw)), DataHash: hex.EncodeToString(h.Sum(nil)), SchemaHash: b.SchemaHash, IdentityHash: b.IdentityHash, Boundary: b, Passes: 2}
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(raw)))
	data := append(size[:], raw...)
	r, e := newRawReader(context.Background(), bytes.NewReader(data), 3, s, nil)
	if e != nil {
		t.Fatal("BSON reader rejected valid fixture")
	}
	v, e := r.next()
	if e != nil || !bytes.Equal(v.bson, raw) || v.bson.Lookup("_id").Type != bson.TypeInt64 {
		t.Fatal("true BSON changed")
	}
	if _, e = r.next(); e != io.EOF {
		t.Fatal("BSON EOF proof failed")
	}
	r, e = newRawReader(context.Background(), bytes.NewReader(data[:len(data)-1]), 3, s, nil)
	if e == nil {
		_, e = r.next()
	}
	if e == nil || e == io.EOF {
		t.Fatal("partial BSON accepted")
	}
}

func TestSQLStructureOnlyNormalizesVerifiedRedundantCharset(t *testing.T) {
	value := func(s string) *string { return &s }
	a := SQLStructure{DDL: "CREATE TABLE `domain_event_outbox` (\n  `event_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,\n  PRIMARY KEY (`event_id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci", Columns: retirement.SQLColumns{{value("event_id"), value("varchar(64)"), value("NO"), nil, value(""), value("utf8mb4_unicode_ci")}}}
	a.CharacterSets = [][]*string{{value("event_id"), value("utf8mb4")}}
	a.ShowCreateEnvironment = [][]*string{{value("8.4.8"), value("1"), value("STRICT_TRANS_TABLES"), value("utf8mb4"), value("utf8mb4_unicode_ci")}}
	b := a
	b.DDL = strings.Replace(a.DDL, "varchar(64) COLLATE", "varchar(64) CHARACTER SET utf8mb4 COLLATE", 1)
	if !sqlStructuresEqual(a, b) {
		t.Fatal("equivalent verified charset display rejected")
	}
	for _, altered := range []string{
		strings.Replace(b.DDL, "utf8mb4_unicode_ci NOT NULL", "utf8mb4_bin NOT NULL", 1),
		strings.Replace(b.DDL, "CHARACTER SET utf8mb4 COLLATE", "CHARACTER SET latin1 COLLATE", 1),
		strings.Replace(b.DDL, "PRIMARY KEY", "UNIQUE KEY", 1),
		strings.Replace(b.DDL, "ENGINE=InnoDB", "ENGINE=MyISAM", 1),
		strings.Replace(b.DDL, "varchar(64)", "varchar(65)", 1),
		strings.Replace(b.DDL, "NOT NULL,", "DEFAULT ' CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci',", 1),
		strings.Replace(b.DDL, "NOT NULL,", "NOT NULL COMMENT ' CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci',", 1),
	} {
		b.DDL = altered
		if sqlStructuresEqual(a, b) {
			t.Fatal("non-equivalent structure accepted")
		}
	}
	b.DDL = strings.Replace(a.DDL, "varchar(64) COLLATE", "varchar(64) CHARACTER SET utf8mb4 COLLATE", 1)
	b.Columns = nil
	if sqlStructuresEqual(a, b) {
		t.Fatal("missing original column proof accepted")
	}
}

func TestStrictRestoreReceiptTimeAndOpaqueProof(t *testing.T) {
	v := Verification{StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var out Verification
	if exactJSON(raw, &out) != nil {
		t.Fatal("actual timestamp receipt rejected")
	}
	if exactJSON([]byte(`{"started_at":"not-a-time"}`), &out) == nil {
		t.Fatal("invalid timestamp accepted")
	}
	if new(RestoreVerification).Summary().SchemaEqual {
		t.Fatal("caller-created proof succeeded")
	}
	if _, e = json.Marshal(new(RestoreVerification)); !errors.Is(e, ErrSerialization) {
		t.Fatal("opaque proof serialized")
	}
}

func TestPurgeKeepsOriginAndAssetsWhenPrivateDirectoryContainsUnregisteredFile(t *testing.T) {
	dir := t.TempDir()
	// macOS TMPDIR can enter via /var -> /private/var. The backend correctly
	// requires a canonical path with no symlink ancestor; use its real path.
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal("canonical owned directory")
	}
	dir = canonical
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("mode")
	}
	a := Approval{InventorySHA256: strings.Repeat("a", 64), SQLMetadataSHA256: strings.Repeat("b", 64), MongoMetadataSHA256: strings.Repeat("c", 64), OrderedMongoSchemaSHA256: strings.Repeat("d", 64), SourceSHA: strings.Repeat("e", 40), RequestHash: strings.Repeat("f", 64), OperationID: "1-1", RunID: "2-1"}
	reg := registration{Version: 1, Approval: a, Files: registrationNames(), ContainsOriginalBodies: true, PurgeAfterAcceptance: true}
	b, e := json.Marshal(reg)
	if e != nil || writePrivate(filepath.Join(dir, "assets.private.json"), b) != nil {
		t.Fatal("registry")
	}
	for _, name := range reg.Files {
		if writePrivate(filepath.Join(dir, name), []byte("PRIVATE_OWNED_TEST_BYTES")) != nil {
			t.Fatal("asset")
		}
	}
	unknown := filepath.Join(dir, "not_registered.private")
	if writePrivate(unknown, []byte("private")) != nil {
		t.Fatal("unknown")
	}
	if PurgeRegistered(dir, a) != ErrPrivate {
		t.Fatal("unregistered asset accepted")
	}
	for _, name := range append([]string{"assets.private.json"}, reg.Files...) {
		if _, e := os.Stat(filepath.Join(dir, name)); e != nil {
			t.Fatal("cleanup removed assets before refusing unknown entry")
		}
	}
	if os.Remove(unknown) != nil {
		t.Fatal("owned unknown cleanup")
	}
	wrong := a
	wrong.RunID = "3-1"
	if PurgeRegistered(dir, wrong) != ErrPrivate {
		t.Fatal("different approval purged originals")
	}
	// Simulate one already deleted asset after an interrupted purge. The valid
	// original registry still authorizes removal of only its remaining assets.
	if os.Remove(filepath.Join(dir, reg.Files[0])) != nil {
		t.Fatal("partial cleanup")
	}
	if PurgeRegistered(dir, a) != nil {
		t.Fatal("origin-bound partial cleanup refused")
	}
	if entries, e := os.ReadDir(dir); e != nil || len(entries) != 0 {
		t.Fatal("registered raw assets remain")
	}
}
