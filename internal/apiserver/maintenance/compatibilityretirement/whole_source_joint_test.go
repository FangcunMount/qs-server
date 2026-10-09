package retirement

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func wholeJointUnitFixture(t *testing.T, ai int, paired bool) authFixture {
	t.Helper()
	return wholeJointUnitFixtureAt(t, ai, paired, fixtureClock)
}

func wholeJointUnitFixtureAt(t *testing.T, ai int, paired bool, clock time.Time) authFixture {
	t.Helper()
	f := coordinatorFixture(t, ai, paired)
	var sqlRows [][][]byte
	for i, kind := range []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed"} {
		body := bytes.ReplaceAll(wireFixtureAt(t, kind, clock), []byte("original-event-1"), []byte(fmt.Sprintf("coordinator-sql-%d", i)))
		body = bytes.ReplaceAll(body, []byte("sheet-1"), []byte("10042"))
		sqlRows = append(sqlRows, fixtureSQLRow(t, body, fmt.Sprint(i+1)))
	}
	f.raw[0], f.expected[0] = fixtureSQLCopy(t, sqlRows, nil)
	var mongoRows [][]byte
	for i, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
		body := bytes.ReplaceAll(wireFixtureAt(t, kind, clock), []byte("original-event-1"), []byte(fmt.Sprintf("coordinator-mongo-%d", i)))
		body = bytes.ReplaceAll(body, []byte("sheet-1"), []byte("10042"))
		raw, err := bson.Marshal(fixtureMongoRow(t, body, int64(fixtureLargeID+int64(i))))
		if err != nil {
			t.Fatal(err)
		}
		mongoRows = append(mongoRows, raw)
	}
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, mongoRows)
	return f
}

func wholeJointCopies(f authFixture) []WholeSourceJointCopy {
	out := make([]WholeSourceJointCopy, 4)
	for i := range out {
		out[i] = WholeSourceJointCopy{Input: bytes.NewReader(f.raw[i]), Expected: f.expected[i]}
	}
	return out
}

func TestWholeSourceJointIndexSixTypesOriginalOffsetsAndIndependentEOF(t *testing.T) {
	f := wholeJointUnitFixture(t, 2, true)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := c.seekers[0].Seek(0, io.SeekCurrent)
	x, err := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits())
	if err != nil {
		t.Fatal(err)
	}
	after, _ := c.seekers[0].Seek(0, io.SeekCurrent)
	if before != after || !x.Summary().Complete || len(x.entries) != 6 || x.Summary().DropReady || !x.Summary().ExternalOriginRequired {
		t.Fatal("independent source index changed own streaming cursor or overstated closure")
	}
	handles, err := page.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, handle := range handles {
		original, err := handle.Facts()
		if err != nil {
			t.Fatal(err)
		}
		indexed, err := x.event(t.Context(), original.EventID)
		if err != nil {
			t.Fatal(err)
		}
		observed, err := indexed.Facts()
		if err != nil || !reflect.DeepEqual(original, observed) {
			t.Fatal("actual original SQL/BSON frame facts changed", err)
		}
	}
	for _, receipt := range x.Summary().Copies {
		if !receipt.Complete || receipt.BusinessClosureVerified || receipt.DropReady {
			t.Fatal("index confused EOF with business closure")
		}
	}
	if err = x.RecheckSourceCopies(t.Context(), wholeJointCopies(f)); err != nil {
		t.Fatal(err)
	}
	if c.coverage {
		t.Fatal("third source pass fabricated coordinator consumption EOF")
	}
	if _, err = json.Marshal(x); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("private source index leaked identities", err)
	}
}

func TestWholeSourceJointIndexDetachedClonePreservesOriginalFactsAndDigest(t *testing.T) {
	f := wholeJointUnitFixture(t, 2, true)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	x, err := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits())
	if err != nil {
		t.Fatal(err)
	}
	before := x.Summary().IndexSHA256
	legacy := *x
	legacy.entries = make(map[string]wholeSourceJointEntry, len(x.entries))
	observed := 0
	for _, object := range []int{0, 3} {
		var next func() (*DecodedSourceEvent, error)
		var receipt func() SourceCopyReceipt
		if object == 0 {
			r, e := NewSQLSourceReader(bytes.NewReader(f.raw[object]), f.expected[object])
			if e != nil {
				t.Fatal(e)
			}
			next, receipt = r.Next, r.Receipt
		} else {
			r, e := NewMongoSourceReader(bytes.NewReader(f.raw[object]), f.expected[object])
			if e != nil {
				t.Fatal(e)
			}
			next, receipt = r.Next, r.Receipt
		}
		for {
			original, e := next()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(e)
			}
			id := original.EventID
			entry, exists := x.entries[id]
			if !exists {
				t.Fatal("independently decoded original is missing from the index")
			}
			// The old index read these fields from the public decoder DTO and
			// calculated its facts digest again. Reconstruct that contract from
			// a separate complete read, without copying the index hash protocol.
			old := entry
			old.Key, e = sourceAuthKey(original.Source.Database, original.Source.Object, original.Source.PrimaryKeySHA256)
			if e != nil {
				t.Fatal(e)
			}
			old.FactsSHA256, e = privateFactsSHA(original)
			if e != nil {
				t.Fatal(e)
			}
			old.EventID, old.EventType, old.AggregateType, old.AggregateID = original.EventID, original.EventType, original.AggregateType, original.AggregateID
			old.OrgID = original.OrgID
			old.AssessmentID, old.AnswerSheetID = 0, 0
			if original.Submitted != nil {
				old.AnswerSheetID, e = mongoCycleStringID(original.Submitted.AnswerSheetID)
			} else if original.Generated != nil {
				old.AssessmentID, e = mongoCycleStringID(original.Generated.AssessmentID)
			} else {
				old.AssessmentID, e = sqlSourceAssessment(original)
			}
			if e != nil || !reflect.DeepEqual(entry, old) {
				t.Fatal("opaque-clone index changed original facts or identity fields")
			}
			legacy.entries[id] = old
			bound, e := c.authenticated.BindEvent(original)
			if e != nil || bound == nil || bound.facts == nil {
				t.Fatal("original source did not yield an opaque detached clone")
			}
			original.EventID += "-changed"
			original.EventType = "changed.type"
			original.OrgID++
			original.AggregateID = "changed-owner"
			if original.BusinessIDs != nil {
				original.BusinessIDs["assessment_id"] = "999"
			}
			if original.Submitted != nil {
				original.Submitted.AnswerSheetID = "999"
			}
			if original.Generated != nil {
				original.Generated.AssessmentID = "999"
			}
			if _, e = c.authenticated.BindEvent(original); !errors.Is(e, ErrSourceAuthentication) {
				t.Fatal("changed original DTO reused the authenticated facts digest")
			}
			clonedDigest, e := privateFactsSHA(bound.facts)
			if e != nil || clonedDigest != old.FactsSHA256 || bound.facts.EventID != id {
				t.Fatal("original DTO mutation reached the private opaque clone")
			}
			indexed, e := x.event(t.Context(), id)
			if e != nil {
				t.Fatal(e)
			}
			indexedDigest, e := privateFactsSHA(indexed.facts)
			if e != nil || indexedDigest != old.FactsSHA256 {
				t.Fatal("index no longer reads the original authenticated frame")
			}
			observed++
		}
		if !receipt().Complete || receipt() != c.authenticated.receipts[object] {
			t.Fatal("old-contract comparison did not reach independent source EOF")
		}
	}
	if observed != 6 || observed != len(x.entries) || legacy.digest() != before || x.digest() != before || x.Summary().DropReady {
		t.Fatal("complete original index digest or authority changed")
	}
}

func TestWholeSourceJointIndexRejectsUnmatchedAuthenticatedFactsBeforeReuse(t *testing.T) {
	f := wholeJointUnitFixture(t, 1, true)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for key, row := range c.authenticated.rows {
		if key.object == 0 {
			row.facts[0] ^= 1
			c.authenticated.rows[key] = row
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("fixture did not authenticate an original SQL source row")
	}
	if x, e := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits()); !errors.Is(e, ErrSourceAuthentication) || x != nil {
		t.Fatal("index reused a sealed digest without validating the original and clone")
	}
}

func TestWholeSourceJointIndexNeverTreatsCapOrTruncationAsEOF(t *testing.T) {
	for _, mutation := range []string{"truncated_mongo", "trailing_mongo", "truncated_ai", "encoded_cap", "changed_expectation", "nil_copy", "typed_nil_copy", "invalid_limits"} {
		t.Run(mutation, func(t *testing.T) {
			f := wholeJointUnitFixture(t, 1, true)
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			copies := wholeJointCopies(f)
			limits := DefaultWholeSourceJointLimits()
			switch mutation {
			case "truncated_mongo":
				copies[3].Input = bytes.NewReader(f.raw[3][:len(f.raw[3])-1])
			case "trailing_mongo":
				copies[3].Input = bytes.NewReader(append(append([]byte(nil), f.raw[3]...), 0))
			case "truncated_ai":
				copies[1].Input = bytes.NewReader(f.raw[1][:len(f.raw[1])-3])
			case "encoded_cap":
				limits.MaxEncodedCopyBytes = 16
			case "changed_expectation":
				copies[3].Expected.Boundary.IdentityHash = sourceSHA([]byte("unapproved identity"))
			case "nil_copy":
				copies[3].Input = nil
			case "typed_nil_copy":
				var typedNil *bytes.Reader
				copies[3].Input = typedNil
			case "invalid_limits":
				limits.MaxRelatedSources = 513
			}
			if x, err := c.PrepareWholeSourceJointIndex(t.Context(), copies, limits); err == nil || x != nil {
				t.Fatal("incomplete or altered source was accepted")
			}
		})
	}
}

func TestWholeSourceJointIndexFreshAndRandomReadDetectSourceMutation(t *testing.T) {
	for _, database := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+database)), func(t *testing.T) {
			f := wholeJointUnitFixture(t, 1, true)
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			copies := wholeJointCopies(f)
			x, err := c.PrepareWholeSourceJointIndex(t.Context(), copies, DefaultWholeSourceJointLimits())
			if err != nil {
				t.Fatal(err)
			}
			if database == 0 || database == 3 {
				for id, entry := range x.entries {
					if int(entry.Key.object) == database {
						f.raw[database][entry.Offset+entry.Length/2] ^= 1
						if _, err = x.event(t.Context(), id); err == nil {
							t.Fatal("changed original encoded frame accepted")
						}
						break
					}
				}
			} else {
				f.raw[database][len(f.raw[database])-3] ^= 1
			}
			if err = x.RecheckSourceCopies(t.Context(), wholeJointCopies(f)); err == nil {
				t.Fatal("fresh failed to inspect nonselected source to real EOF")
			}
		})
	}
}

func TestWholeSourceJointRejectsOtherSourceOwnerAndEditableQualification(t *testing.T) {
	f := wholeJointUnitFixture(t, 0, false)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	x, err := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.PrepareWholeSourceJointPage(t.Context(), p, x, nil, nil, nil); err == nil {
		t.Fatal("nil database capabilities accepted")
	}
	if err = c.QualifyWholeSourceJointPage(t.Context(), p, &WholeSourceJointPage{}); err == nil {
		t.Fatal("caller-created qualification accepted")
	}
	other, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	//nolint:staticcheck // Deliberately prove this private reader rejects a nil context.
	if _, err = x.event(nil, "coordinator-sql-0"); err == nil {
		t.Fatal("nil context accepted")
	}
	x.owner = other
	if _, err = x.event(t.Context(), "coordinator-sql-0"); err == nil {
		t.Fatal("index transplanted to other source owner")
	}
	if (&WholeSourceJointPage{}).Summary().DropReady {
		t.Fatal("empty public summary asserted delete authority")
	}
}

func TestWholeSourceJointAtReaderUsesRealNewlineAndProbe(t *testing.T) {
	input := []byte("head\nfirst\nlast")
	r := &wholeSourceJointAtReader{input: bytes.NewReader(input), line: true, limit: int64(len(input)), h: sha256.New()}
	buffer := make([]byte, 1024)
	n, err := r.Read(buffer)
	if err != nil || string(buffer[:n]) != "head\n" || r.position != 5 {
		t.Fatal("scanner boundary prefetched another frame")
	}
	n, err = r.Read(buffer)
	if err != nil || string(buffer[:n]) != "first\n" {
		t.Fatal("second frame offset lost")
	}
	n, err = r.Read(buffer)
	if n != 4 || string(buffer[:n]) != "last" {
		t.Fatal("real final partial frame lost", err)
	}
	if _, err = r.Read(buffer); err != io.EOF {
		t.Fatal("real EOF not recognized", err)
	}
	r = &wholeSourceJointAtReader{input: bytes.NewReader(input), line: false, limit: 5, h: sha256.New()}
	if _, err = r.Read(buffer); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Read(buffer); !errors.Is(err, ErrWholeSourceJointBounds) {
		t.Fatal("cap became synthetic EOF", err)
	}
}
