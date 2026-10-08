package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"strings"
	"testing"
	"time"
)

func coordinatorFixture(t *testing.T, aiCount int, paired bool) authFixture {
	t.Helper()
	var f authFixture
	sqlKinds := []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed"}
	var sqlRows [][][]byte
	for i, kind := range sqlKinds {
		body := bytes.ReplaceAll(wireFixture(t, kind), []byte("original-event-1"), []byte(fmt.Sprintf("coordinator-sql-%d", i)))
		sqlRows = append(sqlRows, fixtureSQLRow(t, body, fmt.Sprint(i+1)))
	}
	f.raw[0], f.expected[0] = fixtureSQLCopy(t, sqlRows, nil)
	var bridgeRows, legacyRows [][][]byte
	for i := 1; i <= aiCount; i++ {
		id := fmt.Sprintf("10000000-0000-4000-8000-%012d", i)
		bridge := aiFixtureRow(t, AIBridgeCommandSource, "start")
		bridge[0] = []byte(id)
		bridge[1] = []byte(id)
		bridge[3] = bytes.ReplaceAll(bridge[3], []byte(aiFixtureRequestID), []byte(id))
		bridge[4] = []byte(sourceSHA(bridge[3]))
		if !paired {
			bridge[5] = []byte("1")
		}
		bridgeRows = append(bridgeRows, bridge)
		if paired {
			legacy := aiFixtureRow(t, AILegacyCommandSource, "start")
			legacy[0] = []byte(id)
			legacy[1] = []byte(id)
			legacy[3] = bytes.ReplaceAll(legacy[3], []byte(aiFixtureRequestID), []byte(id))
			legacy[4] = []byte(sourceSHA(legacy[3]))
			legacyRows = append(legacyRows, legacy)
		}
	}
	f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, bridgeRows, nil)
	f.raw[2], f.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, legacyRows, nil)
	var mongoRows [][]byte
	for i, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
		body := bytes.ReplaceAll(wireFixture(t, kind), []byte("original-event-1"), []byte(fmt.Sprintf("coordinator-mongo-%d", i)))
		raw, err := bson.Marshal(fixtureMongoRow(t, body, int64(fixtureLargeID+int64(i))))
		if err != nil {
			t.Fatal(err)
		}
		mongoRows = append(mongoRows, raw)
	}
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, mongoRows)
	return f
}
func coordinatorBinding() HistoricalCoordinatorBinding {
	return HistoricalCoordinatorBinding{strings.Repeat("a", 40), "377001-1"}
}
func coordinatorDrain(t *testing.T, c *HistoricalCoordinator) {
	t.Helper()
	for {
		p, err := c.NextPage(t.Context())
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		events, err := p.Events()
		if err != nil {
			t.Fatal(err)
		}
		if len(events) > 0 {
			err = c.QualifyPage(t.Context(), p, nil, nil)
		} else {
			err = c.QualifyAIPage(t.Context(), p, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
func TestHistoricalCoordinatorWholeFourSecondPassSixTypesAndEightAI(t *testing.T) {
	for _, paired := range []bool{false, true} {
		t.Run(fmt.Sprint(paired), func(t *testing.T) {
			f := coordinatorFixture(t, 8, paired)
			limits := DefaultHistoricalCoordinatorLimits()
			limits.MaxPageRecords = 4
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
			if err != nil {
				t.Fatal(err)
			}
			if c.Receipt().SourceCoverageComplete {
				t.Fatal("first pass presented whole coordinator coverage")
			}
			if _, err = c.CandidateRange(0, 128); !errors.Is(err, ErrCoordinatorIncomplete) {
				t.Fatal("provisional candidate exported")
			}
			coordinatorDrain(t, c)
			r := c.Receipt()
			want := [4]uint64{4, 8, 0, 2}
			if paired {
				want[2] = 8
			}
			if !r.SourceCoverageComplete || r.ConsumedRecords != want || r.BusinessClosureVerified || r.CASComplete || r.FinalFreshComplete || r.DropReady || !evidenceHash(r.CandidateSHA256) || !evidenceHash(r.PageBoundarySHA256) {
				t.Fatalf("incorrect coordinator receipt: %+v", r)
			}
			rows, err := c.CandidateRange(0, 512)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range rows {
				if v.LocalQualified || v.LocalClassification != "blocked" || v.Source.Digest.SHA256 == "" || v.OwnerID == "" || len(v.RequiredAdapters) == 0 {
					t.Fatal("missing real batch became complete")
				}
			}
			for _, kind := range []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed", "answersheet.submitted", "interpretation.report.generated"} {
				if r.EventTypeCounts[kind] != 1 {
					t.Fatal("six-type source coverage missing")
				}
			}
			if r.EventTypeCounts["ai.command.start"] != want[1]+want[2] {
				t.Fatal("AI paired physical coverage collapsed")
			}
			rows[0].BlockingReasons[0] = "caller changed candidate"
			again, _ := c.CandidateRange(0, 1)
			if again[0].BlockingReasons[0] == rows[0].BlockingReasons[0] {
				t.Fatal("private candidate changed")
			}
			if _, err = json.Marshal(again[0]); !errors.Is(err, ErrSourceSerialization) {
				t.Fatal("private candidate serializable")
			}
			if p, err := c.NextPage(t.Context()); p != nil || err != io.EOF {
				t.Fatal("completed coordinator replayed source")
			}
		})
	}
}
func TestHistoricalCoordinatorEmptyWholeCopiesAndNoForgedPage(t *testing.T) {
	f := authFixture{}
	f.raw[0], f.expected[0] = fixtureSQLCopy(t, nil, nil)
	f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, nil, nil)
	f.raw[2], f.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, nil, nil)
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, nil)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err = c.QualifyPage(t.Context(), &HistoricalSourcePage{owner: c}, nil, nil); !errors.Is(err, ErrCoordinatorPage) {
		t.Fatal("caller created empty page accepted")
	}
	coordinatorDrain(t, c)
	if !c.Receipt().SourceCoverageComplete || c.Receipt().CandidateCount != 0 {
		t.Fatal("empty whole copies falsely missing or added event")
	}
}

type coordinatorSeekMutation struct {
	*bytes.Reader
	alternate []byte
	seeks     int
}

func (r *coordinatorSeekMutation) Seek(offset int64, whence int) (int64, error) {
	r.seeks++
	if r.seeks == 2 {
		r.Reader = bytes.NewReader(r.alternate)
	}
	return r.Reader.Seek(offset, whence)
}
func TestHistoricalCoordinatorSecondPassTruncationAndModificationNeverComplete(t *testing.T) {
	f := coordinatorFixture(t, 8, false)
	for _, mode := range []string{"truncated", "changed"} {
		t.Run(mode, func(t *testing.T) {
			alternate := append([]byte(nil), f.raw[3]...)
			if mode == "truncated" {
				alternate = alternate[:len(alternate)-8]
			} else {
				alternate = bytes.ReplaceAll(alternate, []byte("coordinator-mongo-0"), []byte("coordinator-mongo-X"))
			}
			in := f.inputs()
			in[3].Input = &coordinatorSeekMutation{Reader: bytes.NewReader(f.raw[3]), alternate: alternate}
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), in, DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			var failure error
			for {
				p, e := c.NextPage(t.Context())
				if e != nil {
					failure = e
					break
				}
				events, _ := p.Events()
				if len(events) > 0 {
					e = c.QualifyPage(t.Context(), p, nil, nil)
				} else {
					e = c.QualifyAIPage(t.Context(), p, nil)
				}
				if e != nil {
					failure = e
					break
				}
			}
			if failure == nil || failure == io.EOF || c.Receipt().SourceCoverageComplete {
				t.Fatal("second copy mutation accepted")
			}
			if _, e := c.CandidateRange(0, 128); e == nil {
				t.Fatal("partial candidates leaked after second-copy failure")
			}
		})
	}
}
func TestHistoricalCoordinatorPageReplayForeignAndExpired(t *testing.T) {
	f := coordinatorFixture(t, 0, false)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	other, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = other.QualifyPage(t.Context(), p, nil, nil); !errors.Is(err, ErrCoordinatorPage) {
		t.Fatal("foreign page accepted")
	}
	if _, err = c.NextPage(t.Context()); !errors.Is(err, ErrCoordinatorPage) {
		t.Fatal("outstanding page bypassed")
	}
	if err = c.QualifyPage(t.Context(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err = c.QualifyPage(t.Context(), p, nil, nil); !errors.Is(err, ErrCoordinatorPage) {
		t.Fatal("page consumed twice")
	}
	p, err = c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return p.issued.Add(limits.PageTTL + time.Millisecond) }
	if err = c.QualifyPage(t.Context(), p, nil, nil); !errors.Is(err, ErrCoordinatorExpired) {
		t.Fatal("expired page accepted")
	}
	if _, err = p.Events(); !errors.Is(err, ErrCoordinatorExpired) {
		t.Fatal("expired handles escaped")
	}
	if c.Receipt().SourceCoverageComplete {
		t.Fatal("expired page marked complete")
	}
}
func TestHistoricalCoordinatorInputBoundsAndBorrowedReaderLifecycle(t *testing.T) {
	f := coordinatorFixture(t, 8, false)
	cases := []string{"plain_reader", "typed_nil", "budget", "ai_budget", "op_uuid", "source", "canceled"}
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			in := f.inputs()
			limits := DefaultHistoricalCoordinatorLimits()
			binding := coordinatorBinding()
			ctx := context.Background()
			switch kind {
			case "plain_reader":
				in[0].Input = bytes.NewBuffer(f.raw[0])
			case "typed_nil":
				var r *bytes.Reader
				in[0].Input = r
			case "budget":
				limits.MaxReservationBytes = 1536
			case "ai_budget":
				limits.MaxAICommands = 7
			case "op_uuid":
				binding.OperationID = aiFixtureRequestID
			case "source":
				binding.SourceSHA = strings.Repeat("A", 40)
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if c, e := PrepareHistoricalCoordinator(ctx, binding, in, limits); e == nil || c != nil {
				t.Fatal("invalid input acquired coordinator")
			}
		})
	}
	observed := make([]*authObservedReader, 4)
	in := f.inputs()
	for i := range in {
		observed[i] = &authObservedReader{Reader: bytes.NewReader(f.raw[i])}
		in[i].Input = observed[i]
	}
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), in, DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	coordinatorDrain(t, c)
	for _, r := range observed {
		if r.closes != 0 || r.reads < 2 {
			t.Fatal("borrowed stream closed or not fully reread")
		}
	}
}
func TestHistoricalCoordinatorOriginalIDReuseRejectedBeforeQualification(t *testing.T) {
	f := coordinatorFixture(t, 0, false)
	body := bytes.ReplaceAll(wireFixture(t, "answersheet.submitted"), []byte("original-event-1"), []byte("coordinator-sql-0"))
	raw, err := bson.Marshal(fixtureMongoRow(t, body, int64(fixtureLargeID)))
	if err != nil {
		t.Fatal(err)
	}
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, [][]byte{raw})
	if c, e := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits()); e == nil || c != nil {
		t.Fatal("cross-source original ID reuse accepted")
	}
}
func TestHistoricalCoordinatorCandidateHashProductionScaleDoesNotHitPerRowBound(t *testing.T) {
	values := make([]HistoricalCandidate, 10001)
	for i := range values {
		values[i] = HistoricalCandidate{OriginalID: fmt.Sprint(i), EventType: "evaluation.failed", RequiredAdapters: coordinatorRequiredAdapters()}
	}
	h := coordinatorCandidateHash(values)
	if !evidenceHash(h) {
		t.Fatal("large whole candidate list incorrectly used per-row hash budget")
	}
	values[9999].OriginalID = "changed"
	if coordinatorCandidateHash(values) == h {
		t.Fatal("late candidate omitted from aggregate")
	}
	pages := make([]HistoricalCoordinatorPageReceipt, 10001)
	for i := range pages {
		pages[i].Sequence = uint64(i + 1)
	}
	if !evidenceHash(coordinatorPageHash(pages)) {
		t.Fatal("large page list hit per-row hash budget")
	}
}

func TestHistoricalCoordinatorCompletedCandidateEpochExpires(t *testing.T) {
	f := coordinatorFixture(t, 0, false)
	c, e := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if e != nil {
		t.Fatal(e)
	}
	coordinatorDrain(t, c)
	c.now = func() time.Time { return c.started.Add(c.limits.MaxDuration + time.Millisecond) }
	if _, e = c.CandidateRange(0, 128); !errors.Is(e, ErrCoordinatorExpired) {
		t.Fatal("expired business candidates exported")
	}
	if _, e = c.PageReceipts(0, 128); !errors.Is(e, ErrCoordinatorExpired) {
		t.Fatal("expired pages exported as current")
	}
}
