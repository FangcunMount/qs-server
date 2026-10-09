package retirement

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// These are hash/clone fixtures, never local business or CAS capabilities.
func candidateStorageValueFixture() []HistoricalCandidate {
	attempt := uint32(3)
	v := HistoricalCandidate{
		Source:     evidence.HistoricalSourceReferenceV1{Database: "mysql", Object: "domain_event_outbox", PrimaryKeyKind: "uint64", PrimaryKeySHA256: strings.Repeat("a", 64), Digest: evidence.Digest{Kind: "source_row", SHA256: strings.Repeat("b", 64)}},
		OriginalID: "synthetic-original-1", EventType: "evaluation.failed", OwnerDatabase: "mysql", OwnerObject: "assessment", OwnerID: "900001", OrganizationID: "7",
		ContentDigest: evidence.Digest{Kind: "source_bytes", SHA256: strings.Repeat("c", 64)}, OriginalRunID: "original-run", OriginalRunMissing: []string{"historical_origin"}, OriginalAttempt: &attempt,
		ActualOriginalRun: &evidence.HistoricalRunReferenceV1{RunID: "original-run", Attempt: 3}, AuthorizationRun: &evidence.HistoricalRunReferenceV1{RunID: "authorization-run", Attempt: 2}, ExecutionRun: &evidence.HistoricalRunReferenceV1{RunID: "execution-run", Attempt: 3},
		WriterPayloadDigest: evidence.Digest{Kind: "writer_canonical", SHA256: strings.Repeat("d", 64)}, AIRequestID: "synthetic-request", AISubjectID: "synthetic-subject", AIResourceID: "synthetic-resource", AISourceAttempts: 2, AIHandoffBudgetFloor: 3, AIHandoffBudgetExhausted: true, AIAdmissionRevision: 4,
		MappedMessagingBodySHA256: strings.Repeat("e", 64), BusinessBindingSHA256: strings.Repeat("f", 64), BusinessBaselineSHA256: strings.Repeat("0", 64), LocalClassification: "blocked", LocalQualified: false,
		HistoricalGaps: []string{"historical_gap"}, BlockingReasons: []string{"unknown_execution", "owner_not_verified"}, RequiredAdapters: []string{"actual_joint_closure", "actual_writer_fence"},
	}
	other := coordinatorCloneCandidate(v)
	other.Source.Database, other.Source.Object, other.Source.PrimaryKeyKind = "mongodb", "domain_event_outbox", "objectID"
	other.OriginalID, other.EventType, other.OwnerDatabase, other.OwnerObject, other.OwnerID = "synthetic-original-2", "answersheet.submitted", "mongodb", "answersheets", "900002"
	other.OriginalRunID, other.OriginalAttempt, other.ActualOriginalRun = "", nil, nil
	other.AuthorizationRun, other.ExecutionRun = nil, nil
	other.LocalClassification, other.LocalQualified = "candidate_historical_gap_requires_joint_closure", true
	other.BlockingReasons = nil
	return []HistoricalCandidate{v, other}
}

func TestCoordinatorCandidateStorageValueHashContract(t *testing.T) {
	values := candidateStorageValueFixture()
	const want = "834939ac63f9a6954fa0a92d1942923a4db839bf67fea1f311e19d2c2cab8e3f"
	if got := coordinatorCandidateHash(values); got != want {
		t.Fatalf("candidate value hash changed: got %s", got)
	}
	values[0], values[1] = values[1], values[0]
	if coordinatorCandidateHash(values) == want {
		t.Fatal("candidate order lost")
	}
}

func TestCoordinatorCandidateStorageCloneContract(t *testing.T) {
	v := candidateStorageValueFixture()[0]
	before := coordinatorCandidateHash([]HistoricalCandidate{v})
	out := coordinatorCloneCandidate(v)
	if !reflect.DeepEqual(v, out) {
		t.Fatal("clone lost candidate fields")
	}
	out.Source.Object, out.OriginalID, out.ContentDigest.SHA256 = "changed", "changed", "changed"
	out.OriginalRunMissing[0], *out.OriginalAttempt = "changed", 99
	out.ActualOriginalRun.RunID, out.AuthorizationRun.RunID, out.ExecutionRun.RunID = "changed", "changed", "changed"
	out.HistoricalGaps[0], out.BlockingReasons[0], out.RequiredAdapters[0] = "changed", "changed", "changed"
	if got := coordinatorCandidateHash([]HistoricalCandidate{v}); got != before {
		t.Fatal("clone mutated original candidate")
	}
}

func TestCoordinatorCandidateStoragePhysicalOrderContract(t *testing.T) {
	f := coordinatorFixture(t, 2, true)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CandidateRange(0, 512); err != ErrCoordinatorIncomplete {
		t.Fatal("candidates exported before own complete EOF")
	}
	coordinatorDrain(t, c)
	values, err := c.CandidateRange(0, 512)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, v := range values {
		ids = append(ids, v.OriginalID)
	}
	want := []string{"coordinator-sql-0", "coordinator-sql-1", "coordinator-sql-2", "coordinator-sql-3", "10000000-0000-4000-8000-000000000001", "10000000-0000-4000-8000-000000000001", "10000000-0000-4000-8000-000000000002", "10000000-0000-4000-8000-000000000002", "coordinator-mongo-0", "coordinator-mongo-1"}
	if !reflect.DeepEqual(ids, want) || len(values) != 10 {
		t.Fatal("physical original identity/order lost")
	}
	if r := c.Receipt(); !r.SourceCoverageComplete || r.CandidateCount != 10 || r.BlockedLocalCount != 10 || r.LocallyQualifiedCount != 0 || r.CandidateSHA256 != coordinatorCandidateHash(values) || r.DropReady || r.CASComplete {
		t.Fatal("full-source blocked candidates or receipt changed")
	}
}

func TestCoordinatorCandidateStoragePointerHashMatchesValues(t *testing.T) {
	values := candidateStorageValueFixture()
	stored := make([]*HistoricalCandidate, len(values))
	for i := range values {
		stored[i] = &values[i]
	}
	want := coordinatorCandidateHash(values)
	if coordinatorStoredCandidateHash(stored) != want || !coordinatorStoredCandidateMatches(stored, want) {
		t.Fatal("stored value facts or hash framing changed")
	}
	if coordinatorStoredCandidateHash(nil) != coordinatorCandidateHash(nil) {
		t.Fatal("empty-source hash changed")
	}
	stored[0], stored[1] = stored[1], stored[0]
	if coordinatorStoredCandidateMatches(stored, want) {
		t.Fatal("stored order omitted from digest")
	}
	stored[0], stored[1] = stored[1], stored[0]
	stored[0].Source.Digest.SHA256 = strings.Repeat("1", 64)
	if coordinatorStoredCandidateMatches(stored, want) {
		t.Fatal("stored original source digest omitted")
	}
	for _, i := range []int{0, len(stored) - 1} {
		saved := stored[i]
		stored[i] = nil
		if coordinatorStoredCandidateHash(stored) != "" || coordinatorStoredCandidateMatches(stored, "") || coordinatorStoredCandidateMatches(stored, want) || coordinatorStoredCandidatesPresent(stored) {
			t.Fatal("missing internal value accepted")
		}
		stored[i] = saved
	}
}

func TestCoordinatorCandidateStoragePagePointersStayStable(t *testing.T) {
	f := coordinatorFixture(t, 2, true)
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.candidates) != 0 || cap(c.candidates) != 10 || len(c.remaining) != 10 || c.authenticated.entries != 10 {
		t.Fatal("storage reservation did not use authenticated physical count")
	}
	var retained []*HistoricalCandidate
	var hashes []string
	for {
		page, err := c.NextPage(t.Context())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events, err := page.Events()
		if err != nil {
			t.Fatal(err)
		}
		if len(events) > 0 {
			err = c.QualifyPage(t.Context(), page, nil, nil)
		} else {
			err = c.QualifyAIPage(t.Context(), page, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		for i, original := range retained {
			if c.candidates[i] != original || coordinatorStoredCandidateHash(c.candidates[i:i+1]) != hashes[i] {
				t.Fatal("later page moved or mutated a retained candidate")
			}
		}
		for i := len(retained); i < len(c.candidates); i++ {
			for _, existing := range retained {
				if c.candidates[i] == existing {
					t.Fatal("distinct physical candidates share one slot")
				}
			}
			retained = append(retained, c.candidates[i])
			hashes = append(hashes, coordinatorStoredCandidateHash(c.candidates[i:i+1]))
		}
	}
	all, err := c.CandidateRange(0, 512)
	if err != nil || len(retained) != 10 || cap(c.candidates) != 10 || len(c.remaining) != 0 {
		t.Fatal("original physical candidates lost", err)
	}
	if all[4].OriginalID != all[5].OriginalID || all[4].Source.Object == all[5].Source.Object || all[4].Source.Digest == all[5].Source.Digest {
		t.Fatal("paired physical sources collapsed")
	}
	for offset := 0; offset < len(all); offset++ {
		rangeRows, err := c.CandidateRange(offset, 3)
		if err != nil {
			t.Fatal(err)
		}
		end := min(offset+3, len(all))
		if !reflect.DeepEqual(rangeRows, all[offset:end]) {
			t.Fatal("range crossing page/source boundaries changed values")
		}
	}
}

func TestCoordinatorCandidateStorageRangeDeepClone(t *testing.T) {
	values := candidateStorageValueFixture()
	now := time.Now()
	// This tests defensive export only; it supplies no source or CAS authority.
	c := &HistoricalCoordinator{coverage: true, started: now, now: func() time.Time { return now }, limits: DefaultHistoricalCoordinatorLimits(), candidates: []*HistoricalCandidate{&values[0], &values[1]}}
	before := coordinatorStoredCandidateHash(c.candidates)
	rows, err := c.CandidateRange(0, 2)
	if err != nil || !reflect.DeepEqual(rows, values) {
		t.Fatal("range lost original candidate fields", err)
	}
	rows[0].Source.Object, rows[0].OriginalID, rows[0].ContentDigest.SHA256 = "changed", "changed", "changed"
	rows[0].OriginalRunMissing[0], *rows[0].OriginalAttempt = "changed", 99
	rows[0].ActualOriginalRun.RunID, rows[0].AuthorizationRun.RunID, rows[0].ExecutionRun.RunID = "changed", "changed", "changed"
	rows[0].HistoricalGaps[0], rows[0].BlockingReasons[0], rows[0].RequiredAdapters[0] = "changed", "changed", "changed"
	if coordinatorStoredCandidateHash(c.candidates) != before {
		t.Fatal("range mutation changed stored identity or evidence")
	}
}

func TestCoordinatorCandidateStorageNilFailsClosed(t *testing.T) {
	for _, stage := range []string{"before_eof", "after_eof"} {
		for _, index := range []int{0, 5} {
			t.Run(fmt.Sprintf("%s_%d", stage, index), func(t *testing.T) {
				f := coordinatorFixture(t, 0, false)
				c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
				if err != nil {
					t.Fatal(err)
				}
				page, err := c.NextPage(t.Context())
				if err != nil || c.QualifyPage(t.Context(), page, nil, nil) != nil || len(c.candidates) != 6 {
					t.Fatal("failed genuine source consumption", err)
				}
				if stage == "after_eof" {
					coordinatorDrain(t, c)
				}
				c.candidates[index] = nil
				if page, err := c.NextPage(t.Context()); page != nil || !errors.Is(err, ErrCoordinatorIncomplete) {
					t.Fatal("nil internal slot retained complete EOF", err)
				}
				if r := c.Receipt(); r.SourceCoverageComplete || r.CandidateSHA256 != "" || r.DropReady || r.CASComplete {
					t.Fatal("nil storage produced complete receipt")
				}
				if _, err := c.CandidateRange(index, 1); !errors.Is(err, ErrCoordinatorIncomplete) {
					t.Fatal("nil candidate exported", err)
				}
				if r := c.SQLCASReadiness(); r.WholeFourCopyCoverage || r.CASAuthorized || r.DropReady {
					t.Fatal("nil storage produced SQL readiness")
				}
				if r := c.MongoCASReadiness(); r.WholeFourCopyCoverage || r.CASAuthorized || r.DropReady {
					t.Fatal("nil storage produced Mongo readiness")
				}
			})
		}
	}
}

func TestCoordinatorCandidateStorageNilCannotMintCAS(t *testing.T) {
	now := time.Now()
	c := &HistoricalCoordinator{coverage: true, authenticated: &VerifiedSourceCopies{}, started: now, now: func() time.Time { return now }, limits: DefaultHistoricalCoordinatorLimits(), candidates: []*HistoricalCandidate{nil}}
	if r := c.Receipt(); r.SourceCoverageComplete || r.CandidateSHA256 != "" {
		t.Fatal("nil storage produced complete receipt")
	}
	if _, err := c.CandidateRange(0, 1); !errors.Is(err, ErrCoordinatorIncomplete) {
		t.Fatal("nil storage exported candidate", err)
	}
	if r := c.SQLCASReadiness(); r.WholeFourCopyCoverage || r.CASAuthorized || r.DropReady {
		t.Fatal("nil storage produced SQL readiness")
	}
	if r := c.MongoCASReadiness(); r.WholeFourCopyCoverage || r.CASAuthorized || r.DropReady {
		t.Fatal("nil storage produced Mongo readiness")
	}
	sqlBatch := &SQLBusinessOwnerBatch{facts: &sqlevaluation.SQLHistoricalOwnerBatch{}}
	if plan, err := c.PrepareSQLCAS(t.Context(), sqlBatch); plan != nil || !errors.Is(err, ErrCoordinatorIncomplete) {
		t.Fatal("nil storage reached SQL capability gate", err)
	}
	mongoBatch := &MongoHistoricalOwnerBatch{global: &MongoResponsibilitySnapshot{}}
	if plan, err := c.PrepareMongoCAS(t.Context(), mongoBatch); plan != nil || !errors.Is(err, ErrCoordinatorIncomplete) {
		t.Fatal("nil storage reached Mongo capability gate", err)
	}
}
