package retirement

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCoordinatorProfilesPreserveCompleteCandidateHashAndListOrder(t *testing.T) {
	rows := candidateStorageValueFixture()
	second := coordinatorCloneCandidate(rows[0])
	second.OriginalID = "different-original"
	rows = append(rows, second)
	want := coordinatorCandidateHash(rows)
	before := make([]HistoricalCandidate, len(rows))
	for i, row := range rows {
		before[i] = coordinatorCloneCandidate(row)
	}
	originalInput := rows[0].RequiredAdapters
	c := &HistoricalCoordinator{}
	if err := c.shareCandidateProfiles(rows); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, before) || coordinatorCandidateHash(rows) != want {
		t.Fatal("sharing changed a candidate field, list order or full value hash")
	}
	if &rows[0].RequiredAdapters[0] != &rows[2].RequiredAdapters[0] || &rows[0].BlockingReasons[0] != &rows[2].BlockingReasons[0] {
		t.Fatal("identical private profiles did not share their backing lists")
	}
	if &rows[0].RequiredAdapters[0] == &originalInput[0] || cap(rows[0].RequiredAdapters) != len(rows[0].RequiredAdapters) {
		t.Fatal("first retained profile aliases the caller or retains spare capacity")
	}
	originalInput[0] = "caller-mutated"
	if coordinatorCandidateHash(rows) != want {
		t.Fatal("original editable input changed the shared private list")
	}
	unusual := []HistoricalCandidate{{OriginalRunMissing: []string{"z", "a"}, HistoricalGaps: []string{"z", "a", "a"}, BlockingReasons: []string{"unknown-label", "z"}}}
	hash := coordinatorCandidateHash(unusual)
	if err := c.shareCandidateProfiles(unusual); err != nil || coordinatorCandidateHash(unusual) != hash {
		t.Fatal("interning sorted, deduplicated or dropped an unknown label", err)
	}
}

func TestCoordinatorProfilesPreserveNilAndNonNilEmptyPrivateHash(t *testing.T) {
	var pool candidateProfilePool
	for _, values := range [][]string{nil, {}, make([]string, 0, 32)} {
		before := HistoricalCandidate{OriginalRunMissing: values, HistoricalGaps: values, BlockingReasons: values, RequiredAdapters: values}
		want := coordinatorCandidateHash([]HistoricalCandidate{before})
		for role := candidateOriginalRunMissing; role <= candidateRequiredAdapters; role++ {
			shared, err := pool.intern(role, values)
			if err != nil || (shared == nil) != (values == nil) || len(shared) != 0 {
				t.Fatal("empty profile normalized its original presence", err)
			}
		}
		rows := []HistoricalCandidate{before}
		if err := (&HistoricalCoordinator{}).shareCandidateProfiles(rows); err != nil || coordinatorCandidateHash(rows) != want {
			t.Fatal("empty profile changed private candidate hash", err)
		}
		// Preserve the established public clone behavior rather than silently
		// changing it as part of this private optimization.
		exported := coordinatorCloneCandidate(rows[0])
		if exported.OriginalRunMissing != nil || exported.HistoricalGaps != nil || exported.BlockingReasons != nil || exported.RequiredAdapters != nil {
			t.Fatal("existing empty-list export behavior changed")
		}
	}
	if len(pool.entries) != 0 || pool.bytes != 0 {
		t.Fatal("empty profiles consumed optional cache capacity")
	}
	if coordinatorCandidateHash([]HistoricalCandidate{{RequiredAdapters: nil}}) == coordinatorCandidateHash([]HistoricalCandidate{{RequiredAdapters: []string{}}}) {
		t.Fatal("private value hash lost nil/empty distinction")
	}
}

func TestCoordinatorProfilesSaturationKeepsLegalCompleteLists(t *testing.T) {
	t.Run("entry_limit", func(t *testing.T) {
		var pool candidateProfilePool
		var first []string
		for i := 0; i < maxCandidateProfiles; i++ {
			values := []string{fmt.Sprintf("unknown-but-retained-%d", i)}
			shared, err := pool.intern(candidateBlockingReasons, values)
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				first = shared
			}
		}
		beforeBytes := pool.bytes
		fallback := []string{"new-unknown-reason", "every-required-adapter-stays"}
		shared, err := pool.intern(candidateBlockingReasons, fallback)
		if err != nil || !reflect.DeepEqual(shared, fallback) || &shared[0] != &fallback[0] || len(pool.entries) != maxCandidateProfiles || pool.bytes != beforeBytes {
			t.Fatal("cache limit rejected or changed legal complete fallback", err)
		}
		again, err := pool.intern(candidateBlockingReasons, []string{"unknown-but-retained-0"})
		if err != nil || &again[0] != &first[0] {
			t.Fatal("saturated cache stopped reusing an existing exact profile", err)
		}
	})
	t.Run("byte_limit", func(t *testing.T) {
		var pool candidateProfilePool
		values := []string{strings.Repeat("x", int(maxCandidateProfileBytes-candidateProfileOverhead-16))}
		if _, err := pool.intern(candidateHistoricalGaps, values); err != nil || pool.bytes != maxCandidateProfileBytes {
			t.Fatal("exact optional cache byte limit failed", err)
		}
		for _, fallback := range [][]string{{"unresolved-original-run"}, {strings.Repeat("y", int(maxCandidateProfileBytes)+1)}} {
			shared, err := pool.intern(candidateHistoricalGaps, fallback)
			if err != nil || !slices.Equal(shared, fallback) || &shared[0] != &fallback[0] || len(pool.entries) != 1 {
				t.Fatal("large legal fallback was rejected or shortened", err)
			}
		}
	})
}

func TestCoordinatorProfilesUnknownRoleCollisionAndDamageFailClosed(t *testing.T) {
	for _, badRole := range []candidateProfileRole{0, 5, 255} {
		var pool candidateProfilePool
		if _, err := pool.intern(badRole, nil); !errors.Is(err, ErrCoordinatorPage) {
			t.Fatal("unknown role accepted", err)
		}
	}
	values := []string{"actual_required_gap"}
	charge, _ := candidateProfileCharge(values)
	key := candidateProfileDigest(candidateRequiredAdapters, values)
	for _, bad := range []candidateProfileEntry{
		{values: []string{"different_gap"}, charge: charge},
		{values: nil, charge: charge},
		{values: []string{}, charge: charge},
		{values: append(make([]string, 0, 8), values...), charge: charge},
		{values: slices.Clone(values), charge: charge - 1},
	} {
		pool := candidateProfilePool{entries: map[candidateProfileKey]candidateProfileEntry{key: bad}, bytes: charge}
		if _, err := pool.intern(candidateRequiredAdapters, values); !errors.Is(err, ErrCoordinatorPage) {
			t.Fatal("damaged profile or hash content conflict silently shared", err)
		}
	}
	for _, pool := range []candidateProfilePool{{bytes: 1}, {bytes: maxCandidateProfileBytes + 1}, {entries: map[candidateProfileKey]candidateProfileEntry{key: {values: values, charge: charge}}}} {
		if _, err := pool.intern(candidateRequiredAdapters, values); !errors.Is(err, ErrCoordinatorPage) {
			t.Fatal("damaged cache accounting accepted", err)
		}
	}
	if candidateProfileDigest(candidateRequiredAdapters, []string{"a", "bc"}) == candidateProfileDigest(candidateRequiredAdapters, []string{"ab", "c"}) || candidateProfileDigest(candidateRequiredAdapters, values) == candidateProfileDigest(candidateBlockingReasons, values) {
		t.Fatal("length framing or private role omitted from profile identity")
	}
}

func TestCoordinatorProfilesActualEventAndAIPagesKeepHashAndExportIsolation(t *testing.T) {
	f := coordinatorFixture(t, 2, true)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	coordinatorDrain(t, c)
	rows, err := c.CandidateRange(0, 512)
	if err != nil || len(rows) != 10 || c.Receipt().CandidateSHA256 != coordinatorCandidateHash(rows) || len(c.profiles.entries) == 0 {
		t.Fatal("real source event/paired AI consumption lost full value hash", err)
	}
	if &c.candidates[0].RequiredAdapters[0] != &c.candidates[1].RequiredAdapters[0] {
		t.Fatal("real page candidates did not share finalized private profiles")
	}
	before := c.Receipt().CandidateSHA256
	rows[0].RequiredAdapters[0], rows[0].BlockingReasons[0], rows[0].OriginalRunMissing[0] = "edited", "edited", "edited"
	if c.Receipt().CandidateSHA256 != before || c.Receipt().DropReady || c.SQLCASReadiness().CASAuthorized || c.MongoCASReadiness().CASAuthorized {
		t.Fatal("public range mutated private shared facts or created CAS authority")
	}
}

func TestCoordinatorProfilesSaturationActualConsumptionMatchesCompleteHash(t *testing.T) {
	f := coordinatorFixture(t, 2, true)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	ordinary, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	coordinatorDrain(t, ordinary)
	want, err := ordinary.CandidateRange(0, 512)
	if err != nil {
		t.Fatal(err)
	}
	saturated, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxCandidateProfiles; i++ {
		if _, err := saturated.profiles.intern(candidateBlockingReasons, []string{fmt.Sprintf("retained-unknown-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	beforeBytes := saturated.profiles.bytes
	coordinatorDrain(t, saturated)
	actual, err := saturated.CandidateRange(0, 512)
	if err != nil || !reflect.DeepEqual(actual, want) || saturated.Receipt().CandidateSHA256 != ordinary.Receipt().CandidateSHA256 || coordinatorStoredCandidateHash(saturated.candidates) != coordinatorCandidateHash(want) {
		t.Fatal("saturated legal source consumption changed complete candidates or hash", err)
	}
	if len(saturated.profiles.entries) != maxCandidateProfiles || saturated.profiles.bytes != beforeBytes || !saturated.Receipt().SourceCoverageComplete || saturated.SQLCASReadiness().CASAuthorized || saturated.MongoCASReadiness().CASAuthorized {
		t.Fatal("saturated optional cache altered capacity, EOF coverage or authority")
	}
}

func TestCoordinatorProfilesIdentityAndCacheFaultConsumeNoRows(t *testing.T) {
	f := coordinatorFixture(t, 2, false)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var bad []HistoricalCandidate
	for _, row := range p.rows {
		facts, e := row.event.Facts()
		if e != nil {
			t.Fatal(e)
		}
		bad = append(bad, coordinatorEventCandidate(facts))
	}
	bad[len(bad)-1].Source.PrimaryKeySHA256 = strings.Repeat("f", 64)
	if err = c.consume(p, bad, HistoricalCoordinatorPageReceipt{}); !errors.Is(err, ErrCoordinatorPage) || len(c.profiles.entries) != 0 || len(c.candidates) != 0 || c.consumed != [4]uint64{} || p.consumed {
		t.Fatal("profiles interned or source rows consumed before all identities passed", err)
	}
	if err = c.QualifyPage(t.Context(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	for key, entry := range c.profiles.entries {
		if key.role == candidateRequiredAdapters {
			entry.values[0] = "damaged-private-cache"
			break
		}
	}
	p, err = c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	beforeCount, beforeRemaining, beforeConsumed := len(c.candidates), len(c.remaining), c.consumed
	if err = c.QualifyPage(t.Context(), p, nil, nil); !errors.Is(err, ErrCoordinatorPage) || len(c.candidates) != beforeCount || len(c.remaining) != beforeRemaining || c.consumed != beforeConsumed || p.consumed || !c.failed {
		t.Fatal("cache fault partially consumed a page", err)
	}
	if c.Receipt().SourceCoverageComplete || c.SQLCASReadiness().WholeFourCopyCoverage || c.MongoCASReadiness().WholeFourCopyCoverage {
		t.Fatal("cache fault retained complete coverage or readiness")
	}
	if _, err = c.NextPage(t.Context()); err == nil || err == io.EOF {
		t.Fatal("failed cache epoch reached successful EOF")
	}
}
