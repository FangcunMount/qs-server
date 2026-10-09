package retirement

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// These tests exercise original authenticated copies and real coordinator
// consumption. No unit fixture is accepted as an actual database epoch or joint.
func replayStoredPageFixture(t *testing.T) (*HistoricalCoordinator, *WholeSourceJointIndex, authFixture) {
	t.Helper()
	f := wholeJointUnitFixture(t, 0, false)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	index, err := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits())
	if err != nil {
		t.Fatal(err)
	}
	coordinatorDrain(t, c)
	return c, index, f
}

func TestWholeSourceJointReplayRejectsUnmintedAndWrongOwner(t *testing.T) {
	c, index, _ := replayStoredPageFixture(t)
	for _, anchor := range []*WholeSourceJointReplayAnchor{nil, {}} {
		if _, err := c.ReplayWholeSourceJointPage(t.Context(), anchor, 1, 0, index, nil, nil); !errors.Is(err, ErrWholeSourceJoint) {
			t.Fatal("unminted replay admitted", err)
		}
	}
	if _, err := c.SealWholeSourceJointReplayAnchor(t.Context(), &WholeSourceJointPage{owner: c, consumed: true}); !errors.Is(err, ErrWholeSourceJoint) {
		t.Fatal("missing real joint epoch admitted", err)
	}
	if _, err := (*HistoricalCoordinator)(nil).ReplayWholeSourceJointPage(t.Context(), nil, 1, 0, nil, nil, nil); !errors.Is(err, ErrWholeSourceJoint) {
		t.Fatal(err)
	}
	if _, err := json.Marshal(&WholeSourceJointReplayAnchor{}); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque replay anchor was serializable", err)
	}
	if err := json.Unmarshal([]byte(`{}`), &WholeSourceJointReplayAnchor{}); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("DTO minted replay anchor", err)
	}
}

func TestWholeSourceJointReplayPrivateReceiptAndCandidateRange(t *testing.T) {
	c, _, _ := replayStoredPageFixture(t)
	before := c.Receipt()
	c.mu.Lock()
	values, err := wholeJointReplayCandidatesLocked(c, 1, 0, 2, c.pages[0])
	c.mu.Unlock()
	if err != nil || len(values) != 2 {
		t.Fatal("actual stored page rejected", err)
	}
	values[0].BlockingReasons[0] = "caller-edited"
	if !reflect.DeepEqual(before, c.Receipt()) {
		t.Fatal("cloned page changed source coverage or private candidates")
	}
	for _, name := range []string{"sequence", "offset", "count", "candidate_digest", "source_count", "first_key", "last_key", "source_first_key", "source_last_key", "missing_authenticated_key", "candidate_owner_change"} {
		t.Run(name, func(t *testing.T) {
			c, _, _ := replayStoredPageFixture(t)
			c.mu.Lock()
			defer c.mu.Unlock()
			r := c.pages[0]
			sequence, offset, count := uint64(1), 0, 2
			switch name {
			case "sequence":
				sequence = 2
			case "offset":
				offset = 1
			case "count":
				count = 1
			case "candidate_digest":
				r.CandidateSHA256 = sourceSHA([]byte("different"))
			case "source_count":
				r.Records[0]--
			case "first_key":
				r.FirstPrimaryKeySHA256 = r.LastPrimaryKeySHA256
			case "last_key":
				r.LastPrimaryKeySHA256 = r.FirstPrimaryKeySHA256
			case "source_first_key":
				r.FirstPrimaryKeyBySourceSHA256[0] = r.LastPrimaryKeyBySourceSHA256[0]
			case "source_last_key":
				r.LastPrimaryKeyBySourceSHA256[0] = r.FirstPrimaryKeyBySourceSHA256[0]
			case "missing_authenticated_key":
				key, e := sourceAuthKey(c.candidates[0].Source.Database, c.candidates[0].Source.Object, c.candidates[0].Source.PrimaryKeySHA256)
				if e != nil {
					t.Fatal(e)
				}
				delete(c.authenticated.rows, key)
			case "candidate_owner_change":
				c.candidates[0].OrganizationID = "999"
			}
			// Mutating both receipt copies still cannot authorize a mismatching
			// actual candidate block, boundary or authenticated source identity.
			c.pages[0] = r
			if _, err := wholeJointReplayCandidatesLocked(c, sequence, offset, count, r); err == nil {
				t.Fatal("changed private page contract accepted")
			}
		})
	}
}

func TestWholeSourceJointReplayRereadsActualFramesAndOwnSourceKey(t *testing.T) {
	c, index, f := replayStoredPageFixture(t)
	c.mu.Lock()
	values, err := wholeJointReplayCandidatesLocked(c, 1, 0, 2, c.pages[0])
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	before := c.Receipt()
	current, rows, err := wholeJointReplaySourceRows(t.Context(), index, values)
	if err != nil || len(current) != 2 || len(rows) != 2 {
		t.Fatal(err)
	}
	for i := range rows {
		facts, e := current[i].Facts()
		if e != nil {
			t.Fatal(e)
		}
		key, kerr := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		if e != nil || kerr != nil || rows[i].event != current[i] || len(rows[i].keys) != 1 || rows[i].keys[0] != key || facts.EventID != values[i].OriginalID {
			t.Fatal("replayed row lost actual source capability")
		}
	}
	entry := index.entries[values[0].OriginalID]
	position := int(entry.Offset)
	old := f.raw[0][position]
	f.raw[0][position] ^= 1
	if _, _, err = wholeJointReplaySourceRows(t.Context(), index, values); !errors.Is(err, ErrSourceAuthentication) {
		t.Fatal("changed physical source frame accepted", err)
	}
	f.raw[0][position] = old
	values[0].OrganizationID = "999"
	if _, _, err = wholeJointReplaySourceRows(t.Context(), index, values); !errors.Is(err, ErrSourceAuthentication) {
		t.Fatal("source organization mismatch accepted", err)
	}
	if !reflect.DeepEqual(before, c.Receipt()) {
		t.Fatal("source reread changed consumption/coverage")
	}
}

func TestWholeSourceJointReplayPreservesTypedEmptyProfile(t *testing.T) {
	c, _, _ := replayStoredPageFixture(t)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.candidates[0].HistoricalGaps = []string{}
	r := c.pages[0]
	r.CandidateSHA256 = coordinatorStoredCandidateHash(c.candidates[:2])
	c.pages[0] = r
	values, err := wholeJointReplayCandidatesLocked(c, 1, 0, 2, r)
	if err != nil || values[0].HistoricalGaps == nil || coordinatorCandidateHash(values) != r.CandidateSHA256 {
		t.Fatal("replay normalized the original private candidate profile", err)
	}
}
