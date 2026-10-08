package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
)

type authObservedReader struct {
	*bytes.Reader
	reads, closes int
}

func (v *authObservedReader) Read(p []byte) (int, error) { v.reads++; return v.Reader.Read(p) }
func (v *authObservedReader) Close() error               { v.closes++; return nil }

type authFixture struct {
	raw            [4][]byte
	expected       [4]SourceCopyExpectation
	sql, mongo     *DecodedSourceEvent
	bridge, legacy *DecodedAICommand
}

func sourceAuthFixture(t *testing.T, paired, distinctEvent bool) authFixture {
	t.Helper()
	var f authFixture
	f.raw[0], f.expected[0] = fixtureSQLCopy(t, [][][]byte{fixtureSQLRow(t, wireFixture(t, "evaluation.failed"), "1")}, nil)
	bridge := aiFixtureRow(t, AIBridgeCommandSource, "start")
	f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{bridge}, nil)
	var legacyRows [][][]byte
	if paired {
		legacyRows = [][][]byte{aiFixtureRow(t, AILegacyCommandSource, "start")}
	}
	f.raw[2], f.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, legacyRows, nil)
	body := wireFixture(t, "answersheet.submitted")
	if distinctEvent {
		body = bytes.ReplaceAll(body, []byte("original-event-1"), []byte("original-event-mongo-1"))
	}
	row := fixtureMongoRow(t, body, int64(fixtureLargeID))
	raw, err := bson.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, [][]byte{raw})
	sql, err := NewSQLSourceReader(bytes.NewReader(f.raw[0]), f.expected[0])
	if err != nil {
		t.Fatal(err)
	}
	f.sql, err = sql.Next()
	if err != nil {
		t.Fatal(err)
	}
	mongo, err := NewMongoSourceReader(bytes.NewReader(f.raw[3]), f.expected[3])
	if err != nil {
		t.Fatal(err)
	}
	f.mongo, err = mongo.Next()
	if err != nil {
		t.Fatal(err)
	}
	ai, err := NewAISQLSourceReader(bytes.NewReader(f.raw[1]), f.expected[1])
	if err != nil {
		t.Fatal(err)
	}
	f.bridge, err = ai.Next()
	if err != nil {
		t.Fatal(err)
	}
	if paired {
		ai, err = NewAISQLSourceReader(bytes.NewReader(f.raw[2]), f.expected[2])
		if err != nil {
			t.Fatal(err)
		}
		f.legacy, err = ai.Next()
		if err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func (f authFixture) inputs() []SourceCopyInput {
	out := make([]SourceCopyInput, 4)
	for i := range out {
		out[i] = SourceCopyInput{Input: bytes.NewReader(f.raw[i]), Expected: f.expected[i]}
	}
	return out
}

func TestSourceAuthWholeFourCopyEOFAndPrivateBindings(t *testing.T) {
	for _, paired := range []bool{false, true} {
		t.Run(fmt.Sprint(paired), func(t *testing.T) {
			f := sourceAuthFixture(t, paired, true)
			inputs := f.inputs()
			observed := make([]*authObservedReader, 4)
			for i := range inputs {
				observed[i] = &authObservedReader{Reader: bytes.NewReader(f.raw[i])}
				inputs[i].Input = observed[i]
			}
			index, err := VerifySourceCopies(context.Background(), inputs)
			if err != nil {
				t.Fatal(err)
			}
			s := index.Summary()
			want := uint64(3)
			if paired {
				want++
			}
			if !s.Complete || s.Entries != want || s.BusinessClosureVerified || s.DropReady || !s.ExternalOriginAuthenticationRequired || !s.HostProcessBudgetRequired {
				t.Fatal("incomplete copy or unearned approval")
			}
			for i, receipt := range s.Copies {
				if !receipt.Complete || receipt.BusinessClosureVerified || receipt.DropReady || receipt.DataHash != f.expected[i].DataHash {
					t.Fatal("unbound copy receipt")
				}
				if observed[i].reads == 0 || observed[i].closes != 0 {
					t.Fatal("reader lifecycle stolen or not scanned")
				}
			}
			bound, err := index.BindEvent(f.sql)
			if err != nil {
				t.Fatal(err)
			}
			again, err := bound.Facts()
			if err != nil || !reflect.DeepEqual(again, f.sql) {
				t.Fatal("facts changed or not detached")
			}
			f.sql.Failed.Reason = "CHANGED_PRIVATE_REASON"
			again, err = bound.Facts()
			if err != nil || again.Failed.Reason == f.sql.Failed.Reason {
				t.Fatal("bound facts retained mutable pointer")
			}
			again.BusinessIDs["assessment_id"] = "changed"
			again.Transport.Fields["last_error"] = strptr("changed")
			third, err := bound.Facts()
			if err != nil || third.BusinessIDs["assessment_id"] == "changed" || third.Transport.Fields["last_error"] != nil {
				t.Fatal("snapshot exposed mutable map")
			}
			if _, err = index.BindEvent(f.sql); err != ErrSourceAuthentication {
				t.Fatal("altered decoded source accepted")
			}
			if _, err = index.BindEvent(f.mongo); err != nil {
				t.Fatal(err)
			}
			command, err := index.BindAICommand(f.bridge)
			if err != nil {
				t.Fatal(err)
			}
			if paired {
				if _, err = index.BindAICommand(f.legacy); err != nil {
					t.Fatal(err)
				}
			}
			for _, value := range []any{index, bound, command} {
				if _, err = json.Marshal(value); err == nil {
					t.Fatal("private proof serialized")
				}
				if strings.Contains(fmt.Sprintf("%#v", value), "historical reason") {
					t.Fatal("private old body printed")
				}
			}
			if len(index.eventIDs) != 0 || len(index.pairs) != 0 {
				t.Fatal("transient cross-source identities retained")
			}
		})
	}
}

func TestSourceAuthLaterCopyFailureNeverPublishesPartialIndex(t *testing.T) {
	f := sourceAuthFixture(t, true, true)
	for i := 0; i < 4; i++ {
		for _, mode := range []string{"truncated", "extra", "digest", "bytes", "records"} {
			t.Run(fmt.Sprintf("%d/%s", i, mode), func(t *testing.T) {
				inputs := f.inputs()
				switch mode {
				case "truncated":
					inputs[i].Input = bytes.NewReader(f.raw[i][:len(f.raw[i])-2])
				case "extra":
					inputs[i].Input = bytes.NewReader(append(append([]byte(nil), f.raw[i]...), []byte("extra")...))
				case "digest":
					inputs[i].Expected.DataHash = strings.Repeat("c", 64)
				case "bytes":
					inputs[i].Expected.Bytes++
				case "records":
					inputs[i].Expected.Records++
				}
				index, err := VerifySourceCopies(context.Background(), inputs)
				if err == nil || index != nil {
					t.Fatal("partial or mismatched copy admitted")
				}
			})
		}
	}
	inputs := f.inputs()
	if index, err := VerifySourceCopies(context.Background(), inputs[:3]); err == nil || index != nil {
		t.Fatal("missing target admitted")
	}
	inputs[1], inputs[2] = inputs[2], inputs[1]
	if index, err := VerifySourceCopies(context.Background(), inputs); err == nil || index != nil {
		t.Fatal("target order/namespace confused")
	}
	inputs = f.inputs()
	inputs[2].Input = nil
	if index, err := VerifySourceCopies(context.Background(), inputs); err == nil || index != nil {
		t.Fatal("missing reader admitted")
	}
	for i := 0; i < 4; i++ {
		for _, reader := range []io.Reader{(*bytes.Reader)(nil), (*strings.Reader)(nil), (*authObservedReader)(nil)} {
			t.Run(fmt.Sprintf("typed_nil_reader/%d/%T", i, reader), func(t *testing.T) {
				inputs := f.inputs()
				inputs[i].Input = reader
				if index, err := VerifySourceCopies(context.Background(), inputs); err != ErrSourceAuthentication || index != nil {
					t.Fatal("typed nil reader admitted or dereferenced", err)
				}
			})
		}
	}
}

func TestSourceAuthCrossNamespaceEventIdentityAndAIPairConflicts(t *testing.T) {
	duplicate := sourceAuthFixture(t, false, false)
	if index, err := VerifySourceCopies(context.Background(), duplicate.inputs()); err != ErrSourceIdentity || index != nil {
		t.Fatal("cross-store event ID reuse accepted", err)
	}
	for _, conflict := range []string{"delivered", "budget", "missing_bridge"} {
		t.Run(conflict, func(t *testing.T) {
			f := sourceAuthFixture(t, true, true)
			switch conflict {
			case "delivered":
				row := aiFixtureRow(t, AIBridgeCommandSource, "start")
				row[5] = []byte("1")
				f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{row}, nil)
			case "budget":
				row := aiFixtureRow(t, AILegacyCommandSource, "start")
				row[5] = []byte("7")
				f.raw[2], f.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, [][][]byte{row}, nil)
			case "missing_bridge":
				f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, nil, nil)
			}
			if index, err := VerifySourceCopies(context.Background(), f.inputs()); err != ErrAISourceHandoff || index != nil {
				t.Fatal("conflicting handoff accepted", err)
			}
		})
	}
}

func TestSourceAuthFullTypedFactsTamperingCannotReuseSourceDigest(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	index, err := VerifySourceCopies(context.Background(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	changes := []func(*DecodedSourceEvent){
		func(v *DecodedSourceEvent) { v.Source.Digest.Kind = "other" },
		func(v *DecodedSourceEvent) { v.Source.Digest.SHA256 = strings.Repeat("a", 64) },
		func(v *DecodedSourceEvent) { v.EventType = "evaluation.requested" },
		func(v *DecodedSourceEvent) { v.EventID = "other" },
		func(v *DecodedSourceEvent) { v.OrgID++ },
		func(v *DecodedSourceEvent) { v.OuterOrgID = nil },
		func(v *DecodedSourceEvent) { v.Failed.Reason = "different" },
		func(v *DecodedSourceEvent) { v.Failed.FailedAt = v.Failed.FailedAt.Add(time.Nanosecond) },
		func(v *DecodedSourceEvent) { v.BusinessAt = v.BusinessAt.Add(time.Millisecond) },
		func(v *DecodedSourceEvent) { v.BusinessIDs["testee_id"] = "1" },
		func(v *DecodedSourceEvent) { v.OriginalRun.Attempt = new(uint32) },
		func(v *DecodedSourceEvent) { v.Transport.Status = "pending" },
		func(v *DecodedSourceEvent) { v.Transport.Fields["last_error"] = strptr("") },
		func(v *DecodedSourceEvent) { v.ResolverGaps = append(v.ResolverGaps, "manufactured") },
	}
	for i, change := range changes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			handle, err := index.BindEvent(f.sql)
			if err != nil {
				t.Fatal(err)
			}
			v, err := handle.Facts()
			if err != nil {
				t.Fatal(err)
			}
			change(v)
			if _, err = index.BindEvent(v); err != ErrSourceAuthentication {
				t.Fatal("changed facts with old row hash admitted", err)
			}
		})
	}
	handle, err := index.BindEvent(f.mongo)
	if err != nil {
		t.Fatal(err)
	}
	v, err := handle.Facts()
	if err != nil {
		t.Fatal(err)
	}
	v.Submitted.Admission.QuestionnaireCode = "other"
	if _, err = index.BindEvent(v); err != ErrSourceAuthentication {
		t.Fatal("nested frozen admission altered")
	}
	command, err := index.BindAICommand(f.bridge)
	if err != nil {
		t.Fatal(err)
	}
	ai, err := command.Facts()
	if err != nil {
		t.Fatal(err)
	}
	ai.Business.AssessmentIDs = append(ai.Business.AssessmentIDs, "other")
	if _, err = index.BindAICommand(ai); err != ErrSourceAuthentication {
		t.Fatal("AI business facts altered")
	}
	fake := &VerifiedSourceCopies{}
	if _, err = fake.BindEvent(f.sql); err != ErrSourceAuthentication {
		t.Fatal("zero-value proof minted")
	}
	var absent *VerifiedSourceCopies
	if _, err = absent.BindAICommand(f.bridge); err != ErrSourceAuthentication {
		t.Fatal("absent proof minted")
	}
}

func TestSourceAuthBudgetAndCancellationPrecedeSourceRead(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	inputs := f.inputs()
	observed := make([]*authObservedReader, 4)
	for i := range inputs {
		observed[i] = &authObservedReader{Reader: bytes.NewReader(f.raw[i])}
		inputs[i].Input = observed[i]
		inputs[i].Expected.Records = 800000
	}
	if index, err := VerifySourceCopies(context.Background(), inputs); err != ErrSourceBounds || index != nil {
		t.Fatal("index budget not rejected before allocation/read")
	}
	for _, r := range observed {
		if r.reads != 0 || r.closes != 0 {
			t.Fatal("oversized input touched")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inputs = f.inputs()
	if index, err := VerifySourceCopies(ctx, inputs); err != ErrSourceIncomplete || index != nil {
		t.Fatal("cancelled source admitted")
	}
}

func TestSourceAuthPrivateFactsCanonicalNullPrecisionAndHooks(t *testing.T) {
	type facts struct {
		Map     map[string]*string
		List    []string
		Pointer *int
		Number  float64
	}
	first := facts{Map: map[string]*string{"b": nil, "a": strptr("value")}, List: []string{}, Number: 0}
	equivalent := facts{Map: map[string]*string{"a": strptr("value"), "b": nil}, List: []string{}, Number: 0}
	a, err := privateFactsSHA(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := privateFactsSHA(equivalent)
	if err != nil || a != b {
		t.Fatal("map iteration affects fingerprint")
	}
	for _, change := range []func(*facts){func(v *facts) { v.Map["b"] = strptr("") }, func(v *facts) { v.List = nil }, func(v *facts) { v.Pointer = new(int) }, func(v *facts) { v.Number = math.Copysign(0, -1) }} {
		v := equivalent
		v.Map = map[string]*string{"a": strptr("value"), "b": nil}
		change(&v)
		c, err := privateFactsSHA(v)
		if err != nil || c == a {
			t.Fatal("NULL/type/IEEE precision collapsed")
		}
	}
	if _, err = privateFactsSHA(math.NaN()); err != ErrSourceAuthentication {
		t.Fatal("nonfinite facts accepted")
	}
	if _, err = privateFactsSHA(make(chan int)); err != ErrSourceAuthentication {
		t.Fatal("unsupported fact type accepted")
	}
	f := sourceAuthFixture(t, false, true)
	if _, err = json.Marshal(f.sql); err == nil {
		t.Fatal("private DTO marshal hook bypassed")
	}
	if _, err = privateFactsSHA(f.sql); err != nil {
		t.Fatal("hash incorrectly invoked serialization hook", err)
	}
	digest := evidence.Digest{Kind: SQLRowDigestKind, SHA256: strings.Repeat("a", 64)}
	if h, err := privateFactsSHA(digest); err != nil || h == ([32]byte{}) {
		t.Fatal("typed digest fingerprint invalid")
	}
}

func TestSourceAuthPublishedIndexSupportsConcurrentDetachedBindings(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	index, err := VerifySourceCopies(context.Background(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 4; j++ {
				handle, err := index.BindEvent(f.sql)
				if err != nil {
					t.Error(err)
					return
				}
				facts, err := handle.Facts()
				if err != nil {
					t.Error(err)
					return
				}
				facts.Failed.Reason = "local private edit"
				command, err := index.BindAICommand(f.bridge)
				if err != nil {
					t.Error(err)
					return
				}
				factsAI, err := command.Facts()
				if err != nil {
					t.Error(err)
					return
				}
				factsAI.Business.AssessmentIDs = append(factsAI.Business.AssessmentIDs, "local")
			}
		}()
	}
	workers.Wait()
}

var _ io.ReadCloser = (*authObservedReader)(nil)
