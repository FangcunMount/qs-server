package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

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
