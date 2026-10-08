package retirement

import (
	"bytes"
	"io"
	"testing"
)

// These digests were independently observed from the complete preallocation-free
// 7546aaaf baseline and the capacity version using the same explicit UTC fixture.
// Default fixtures retain their original +08:00 inputs; only this golden selects
// UTC so decoded timezone names do not depend on the test process location.
// Capacity changes must preserve original bytes, order and blocked candidates.
func TestSourceIndexCapacityPreservesWholeEOFDigests(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		empty                  bool
		events, candidates     uint64
		indexSHA, candidateSHA string
	}{
		{"six_types_and_paired_ai", false, 6, 10, "d8a92b73920b536bd1e5fb664af767d58796d6e76397b3359ebfb98683fb9394", "d7358d0a2dba18b784bf0ababaf7ec055c6b9822163f1951419bb2f03765157d"},
		{"empty_events_and_paired_ai", true, 0, 4, "f54d70135c51a5178952f96e1e35d669026c074c7c2b66a87c2d03de9f931e35", "8c889c300a1f9dbad55fe4f74f4628b2fb8a790b9712f0579ab459716abaa282"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := wholeJointUnitFixtureAt(t, 2, true, fixtureClock.UTC())
			if tc.empty {
				f.raw[0], f.expected[0] = fixtureSQLCopy(t, nil, nil)
				f.raw[3], f.expected[3] = fixtureMongoCopy(t, nil)
			}
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			if c.authenticated.entries != tc.candidates || c.authenticated.eventIDs != nil || c.authenticated.pairs != nil {
				t.Fatal("original authenticated counts or temporary identity release changed")
			}
			x, err := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits())
			if err != nil {
				t.Fatal(err)
			}
			s := x.Summary()
			if !s.Complete || s.Entries != tc.events || s.ReservationBytes != tc.events*1024 || s.DropReady || x.indexSHA != tc.indexSHA {
				t.Fatal("index capacity changed original EOF, typed identities, order or authority")
			}
			if err := x.RecheckSourceCopies(t.Context(), wholeJointCopies(f)); err != nil {
				t.Fatal(err)
			}
			coordinatorDrain(t, c)
			r := c.Receipt()
			if !r.SourceCoverageComplete || r.CandidateCount != tc.candidates || r.BlockedLocalCount != tc.candidates || r.CandidateSHA256 != tc.candidateSHA || r.DropReady {
				t.Fatal("whole consumption lost or reordered original blocked candidates")
			}
		})
	}
}

type mapCapacityObservedReaderAt struct {
	*bytes.Reader
	reads int
}

func (r *mapCapacityObservedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	return r.Reader.ReadAt(p, off)
}

var _ io.ReaderAt = (*mapCapacityObservedReaderAt)(nil)

func TestSourceIndexCapacityRequiresOriginalBindingsAndBudgetsBeforeRead(t *testing.T) {
	for _, mutation := range []string{"unapproved_event_count", "entry_budget", "reservation_budget"} {
		t.Run(mutation, func(t *testing.T) {
			f := wholeJointUnitFixture(t, 2, true)
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			copies := wholeJointCopies(f)
			readers := make([]*mapCapacityObservedReaderAt, 4)
			for i := range readers {
				readers[i] = &mapCapacityObservedReaderAt{Reader: bytes.NewReader(f.raw[i])}
				copies[i].Input = readers[i]
			}
			limits := DefaultWholeSourceJointLimits()
			want := ErrWholeSourceJoint
			switch mutation {
			case "unapproved_event_count":
				copies[0].Expected.Records++
			case "entry_budget":
				limits.MaxIndexEntries = c.authenticated.entries - 1
			case "reservation_budget":
				limits.MaxIndexReservationBytes = c.authenticated.entries*1024 - 1
				want = ErrWholeSourceJointBounds
			}
			if x, err := c.PrepareWholeSourceJointIndex(t.Context(), copies, limits); err != want || x != nil {
				t.Fatal("unapproved count or reservation fabricated a source index")
			}
			for _, r := range readers {
				if r.reads != 0 {
					t.Fatal("budget or independent expectation rejection occurred after source access")
				}
			}
			if c.coverage {
				t.Fatal("allocation bounds granted consumption EOF")
			}
		})
	}
}

func TestSourceIndexCapacityStillRejectsCrossSourceDuplicateIdentity(t *testing.T) {
	f := sourceAuthFixture(t, false, false)
	if auth, err := VerifySourceCopies(t.Context(), f.inputs()); err != ErrSourceIdentity || auth != nil {
		t.Fatal("capacity allocation hid a conflicting original identity")
	}
}
