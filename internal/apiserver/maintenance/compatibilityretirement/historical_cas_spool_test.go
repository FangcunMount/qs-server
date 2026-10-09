package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestHistoricalSpoolMongoPredecessorOnlyAllowsDedicatedSlot(t *testing.T) {
	clock := time.Date(2026, 10, 9, 1, 2, 3, 123000000, time.UTC)
	base := bson.D{{Key: "domain_id", Value: int64(7)}, {Key: "org_id", Value: int64(8)}, {Key: "filled_at", Value: clock}, {Key: "durable_acceptance", Value: nil}, {Key: "future", Value: []byte{0, 255}}}
	old, e := bson.Marshal(base)
	if e != nil {
		t.Fatal(e)
	}
	added := append(append(bson.D(nil), base...), bson.E{Key: "legacy_submission_evidence", Value: bson.D{{Key: "version", Value: 1}}})
	next, e := bson.Marshal(added)
	if e != nil {
		t.Fatal(e)
	}
	if !historicalSpoolMongoOnlySlot(old, next, "legacy_submission_evidence") {
		t.Fatal("exact dedicated append rejected")
	}
	for _, tc := range []struct {
		name   string
		change func(bson.D) bson.D
	}{
		{"org", func(d bson.D) bson.D { d[1].Value = int64(9); return d }},
		{"millisecond", func(d bson.D) bson.D { d[2].Value = clock.Add(time.Millisecond); return d }},
		{"standard_null", func(d bson.D) bson.D { d[3].Value = ""; return d }},
		{"ordered_bson", func(d bson.D) bson.D { d[0], d[1] = d[1], d[0]; return d }},
		{"future_column", func(d bson.D) bson.D { d[4].Value = []byte{0, 254}; return d }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.change(append(bson.D(nil), added...))
			changed, e := bson.Marshal(d)
			if e != nil {
				t.Fatal(e)
			}
			if historicalSpoolMongoOnlySlot(old, changed, "legacy_submission_evidence") {
				t.Fatal("ordinary raw fact changed under evidence exception")
			}
		})
	}
}
func TestHistoricalSpoolPrivateCodecPreservesSourceKeyAndBSONBytes(t *testing.T) {
	var pk, hash [32]byte
	pk[0] = 7
	hash[0] = 8
	raw, e := bson.Marshal(bson.D{{Key: "z", Value: nil}, {Key: "a", Value: []byte{0, 255}}})
	if e != nil {
		t.Fatal(e)
	}
	frame := historicalSpoolFrame{Version: 1, Entries: 1, Sources: []historicalSpoolSource{{"original", 3, pk, hash}}, Before: historicalSpoolRawOut(map[string][]bson.Raw{"answersheets": {raw}, "nil_rows": nil, "empty_rows": {}})}
	encoded, e := historicalSpoolEncode(frame)
	if e != nil {
		t.Fatal(e)
	}
	var copy historicalSpoolFrame
	if historicalSpoolDecode(encoded, &copy) != nil {
		t.Fatal("decode")
	}
	if !reflect.DeepEqual(historicalSpoolRawIn(frame.Before), historicalSpoolRawIn(copy.Before)) || !reflect.DeepEqual(frame.Sources, copy.Sources) || !bytes.Equal(historicalSpoolRawIn(frame.Before)["answersheets"][0], historicalSpoolRawIn(copy.Before)["answersheets"][0]) {
		t.Fatal("source-key or ordered BSON/physical empty changed")
	}
}
func TestHistoricalSpoolRejectsZeroAndNoActualPreparedPage(t *testing.T) {
	root := t.TempDir()
	open := func(name string) *os.File {
		f, e := os.CreateTemp(root, name)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if f.Close() != nil {
				t.Error("close")
			}
		})
		return f
	}
	s, e := NewHistoricalCASSpool(context.Background(), open("mongo-"), open("sql-"), 1<<20, 1<<19)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Append(context.Background(), nil, &HistoricalCASPersistencePage{}); e == nil {
		t.Fatal("zero sealed page admitted")
	}
	if _, e = json.Marshal(s); e == nil {
		t.Fatal("opaque holder serialized")
	}
	if _, _, e := (&WholeSourceJointReplayAnchor{}).Selectors(); e == nil {
		t.Fatal("zero replay anchor selectors admitted")
	}
}
