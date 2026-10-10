package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestSQLHistoricalCASFrozenInputExactPhysicalFootprint(t *testing.T) {
	id, foreign, raw := "42", "99", string([]byte{0, 0xff, 'x'})
	f := &SQLHistoricalCASFrozenInput{before: sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{
		"assessment":         {{"id": &id, "raw": &raw, "nullable": nil}},
		"runtime_checkpoint": {}, "evaluation_outcome": nil,
		"cas_migration_head":    {{"id": &foreign}},
		"shared_readonly_model": {{"id": &foreign}},
	}}, writes: map[string]bool{"assessment:42": true}}
	f.self, f.seal = f, f.digest()
	var visits int
	if err := f.RowDependencies(func(table string, n uint64, sha string, size uint64, write bool) error {
		visits++
		if table != "assessment" || n != 42 || sha != sqlSpoolRowSHA(f.before.rows["assessment"][0]) || size == 0 || !write {
			t.Fatal("physical row identity/hash/write altered")
		}
		return nil
	}); err != nil || visits != 1 {
		t.Fatalf("exact physical dependency: %d %v", visits, err)
	}
	stop := errors.New("stop visitor")
	if err := f.RowDependencies(func(string, uint64, string, uint64, bool) error { return stop }); !errors.Is(err, stop) {
		t.Fatal("visitor failure swallowed")
	}
	if _, err := FreezeSQLHistoricalCASInput(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("detached live producer accepted")
	}
	if json.Unmarshal([]byte(`{}`), f) == nil {
		t.Fatal("JSON input accepted")
	}
}

func TestSQLHistoricalCASFrozenInputRejectsAmbiguityMissingWriteAndMutation(t *testing.T) {
	for _, name := range []string{"duplicate", "missing-write", "zero-id", "changed", "copied"} {
		t.Run(name, func(t *testing.T) {
			id := "42"
			f := &SQLHistoricalCASFrozenInput{before: sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {{"id": &id}}}}, writes: map[string]bool{"assessment:42": true}}
			switch name {
			case "duplicate":
				f.before.rows["assessment"] = append(f.before.rows["assessment"], f.before.rows["assessment"][0])
			case "missing-write":
				f.writes["evaluation_outcome:99"] = true
			case "zero-id":
				id = "0"
			}
			f.self, f.seal = f, f.digest()
			if name == "changed" {
				id = "43"
			}
			if name == "copied" {
				copy := *f
				f = &copy
			}
			if err := f.RowDependencies(func(string, uint64, string, uint64, bool) error { return nil }); err == nil {
				t.Fatal("ambiguous, changed or unproduced footprint accepted")
			}
		})
	}
}

func TestSQLSpoolCodecPreservesPhysicalNullAndNonUTF8RawColumns(t *testing.T) {
	empty, raw, id, clock := "", string([]byte{0, 0xff, 0x80, 'x'}), "7", "2026-10-09 01:02:03.123000"
	image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {{"id": &id, "nullable": nil, "empty": &empty, "future_binary": &raw, "evaluated_at": &clock}}, "evaluation_outcome": {}, "runtime_checkpoint": nil, "business_clock_columns": {}}, schema: map[string]string{"assessment": "schema"}, columns: map[string][]string{"assessment": {"id", "nullable", "empty", "future_binary", "evaluated_at"}}}
	bytes, e := sqlSpoolEncode(sqlSpoolImageOut(image))
	if e != nil {
		t.Fatal(e)
	}
	var decoded sqlSpoolImage
	if sqlSpoolDecode(bytes, &decoded) != nil {
		t.Fatal("decode")
	}
	round := sqlSpoolImageIn(decoded)
	if !reflect.DeepEqual(round, image) || casImageHash(round) != casImageHash(image) {
		t.Fatal("codec altered full raw/null/empty/time baseline")
	}
	altered := casCloneImage(image)
	altered.rows["assessment"][0]["nullable"] = &empty
	if sqlSpoolRowSHA(altered.rows["assessment"][0]) == sqlSpoolRowSHA(image.rows["assessment"][0]) {
		t.Fatal("NULL collapsed to empty")
	}
}
func TestSQLSpoolPrivateBytesTamperAndUnmintedTicketReject(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "spool-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if f.Close() != nil {
			t.Error("close")
		}
	})
	s, e := NewSQLHistoricalCASSpool(f, 1<<20, 1<<19)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	ref, e := s.put(ctx, "real private synthetic codec bytes")
	if e != nil {
		t.Fatal(e)
	}
	var value string
	if s.get(ctx, ref, &value) != nil {
		t.Fatal("private file read")
	}
	if _, e = f.WriteAt([]byte{0}, ref.Offset); e != nil {
		t.Fatal(e)
	}
	if s.get(ctx, ref, &value) == nil {
		t.Fatal("tampered physical frame admitted")
	}
	if _, e = s.load(ctx, &SQLHistoricalCASSpoolTicket{}); e == nil {
		t.Fatal("public zero value minted ticket")
	}
	if _, e = json.Marshal(s); e == nil {
		t.Fatal("opaque spool serialized")
	}
}
func TestSQLSpoolRequiresPrivateEmptySingleLinkRegularFile(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "spool-")
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if f.Close() != nil {
			t.Error("close")
		}
	}()
	if e = f.Chmod(0o640); e != nil {
		t.Fatal(e)
	}
	if _, e = NewSQLHistoricalCASSpool(f, 1024, 512); e == nil {
		t.Fatal("insecure mode accepted")
	}
	if e = f.Chmod(0o600); e != nil {
		t.Fatal(e)
	}
	if e = os.Link(f.Name(), f.Name()+"-link"); e != nil {
		t.Fatal(e)
	}
	if _, e = NewSQLHistoricalCASSpool(f, 1024, 512); e == nil {
		t.Fatal("hard linked private file accepted")
	}
}
