package retirement

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"sort"
	"strconv"
	"time"
)

func evidenceHex(v string, n int) bool {
	raw, e := hex.DecodeString(v)
	return e == nil && len(raw) == n && hex.EncodeToString(raw) == v
}

// A candidate is a private, body-free location and local observation. It is
// deliberately not EventEvidenceV1 and cannot be written by existing CAS APIs.
// Absent source Run/attempt, SDK fingerprint and stable binding stay absent.
type HistoricalCandidate struct {
	Source                                                     evidence.HistoricalSourceReferenceV1
	OriginalID, EventType, OwnerDatabase, OwnerObject, OwnerID string
	OrganizationID                                             string
	ContentDigest                                              evidence.Digest
	OriginalRunID                                              string
	OriginalRunMissing                                         []string
	OriginalAttempt                                            *uint32
	ActualOriginalRun, AuthorizationRun, ExecutionRun          *evidence.HistoricalRunReferenceV1
	WriterPayloadDigest                                        evidence.Digest
	AIRequestID, AISubjectID, AIResourceID                     string
	AISourceAttempts, AIHandoffBudgetFloor                     uint32
	AIHandoffBudgetExhausted                                   bool
	AIAdmissionRevision                                        uint64
	MappedMessagingBodySHA256                                  string
	BusinessBindingSHA256                                      string
	BusinessBaselineSHA256                                     string
	LocalClassification                                        string
	LocalQualified                                             bool
	HistoricalGaps, BlockingReasons, RequiredAdapters          []string
}

func (HistoricalCandidate) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (HistoricalCandidate) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (HistoricalCandidate) String() string {
	return "private historical candidate; not persisted retirement evidence"
}
func (v HistoricalCandidate) GoString() string { return v.String() }
func coordinatorCloneCandidate(v HistoricalCandidate) HistoricalCandidate {
	v.OriginalRunMissing = append([]string(nil), v.OriginalRunMissing...)
	v.HistoricalGaps = append([]string(nil), v.HistoricalGaps...)
	v.BlockingReasons = append([]string(nil), v.BlockingReasons...)
	v.RequiredAdapters = append([]string(nil), v.RequiredAdapters...)
	if v.OriginalAttempt != nil {
		n := *v.OriginalAttempt
		v.OriginalAttempt = &n
	}
	cloneRun := func(run *evidence.HistoricalRunReferenceV1) *evidence.HistoricalRunReferenceV1 {
		if run == nil {
			return nil
		}
		copy := *run
		return &copy
	}
	v.ActualOriginalRun = cloneRun(v.ActualOriginalRun)
	v.AuthorizationRun = cloneRun(v.AuthorizationRun)
	v.ExecutionRun = cloneRun(v.ExecutionRun)
	return v
}

type HistoricalCoordinatorPageReceipt struct {
	Sequence                                                                uint64    `json:"sequence"`
	Records                                                                 [4]uint64 `json:"records"`
	FirstPrimaryKeyBySourceSHA256, LastPrimaryKeyBySourceSHA256             [4]string
	FirstPrimaryKeySHA256, LastPrimaryKeySHA256                             string
	CandidateSHA256, SQLBusinessBaselineSHA256, MongoBusinessBaselineSHA256 string
	IssuedAt, ConsumedAt                                                    time.Time
	PageTTLMilliseconds                                                     int64
}
type HistoricalCoordinatorReceipt struct {
	Protocol, SourceSHA, OperationID                                                  string
	SourceCoverageComplete                                                            bool
	ApprovedCopies, SecondPassCopies                                                  [4]SourceCopyReceipt
	ConsumedRecords                                                                   [4]uint64
	Pages                                                                             uint64
	CandidateCount, LocallyQualifiedCount, BlockedLocalCount, LogicalReservationBytes uint64
	CandidateSHA256, PageBoundarySHA256                                               string
	EventTypeCounts                                                                   map[string]uint64
	RequiredAdapters                                                                  []string
	BusinessClosureVerified, CASComplete, FinalFreshComplete, DropReady               bool
}

func coordinatorRequiredAdapters() []string {
	return []string{"production_source_origin_authentication", "production_process_budget", "global_ai_and_inbox_reverse_coverage", "historical_rerun_and_live_writer_fence", "exact_prepared_business_baseline_to_writer_cas", "all_four_targets_committed_cas_and_independent_readback", "post_cas_expected_business_baseline", "different_actual_final_snapshots_and_full_hash_recheck", "prewindow_exact_backup_and_actual_isolated_restore", "maintenance_acceptance_and_immediate_private_asset_purge"}
}

// Receipt is safe diagnostic output. A DTO of this type is never consumed as
// authorization; only this private coordinator owns its actual coverage state.
func (c *HistoricalCoordinator) Receipt() HistoricalCoordinatorReceipt {
	r := HistoricalCoordinatorReceipt{Protocol: "historical-four-copy-coordinator/v1", EventTypeCounts: map[string]uint64{}, RequiredAdapters: coordinatorRequiredAdapters()}
	if c == nil {
		return r
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r.SourceSHA = c.binding.SourceSHA
	r.OperationID = c.binding.OperationID
	r.LogicalReservationBytes = c.reservation
	if c.authenticated != nil {
		r.ApprovedCopies = c.authenticated.receipts
	}
	r.SecondPassCopies = c.receipts
	r.ConsumedRecords = c.consumed
	r.Pages = uint64(len(c.pages))
	if !c.coverage || c.failed {
		return r
	}
	r.SourceCoverageComplete = true
	r.CandidateCount = uint64(len(c.candidates))
	for _, v := range c.candidates {
		r.EventTypeCounts[v.EventType]++
		if v.LocalQualified {
			r.LocallyQualifiedCount++
		} else {
			r.BlockedLocalCount++
		}
	}
	r.CandidateSHA256 = coordinatorCandidateHash(c.candidates)
	r.PageBoundarySHA256 = coordinatorPageHash(c.pages)
	return r
}

// CandidateRange bounds exports even when the approved source has a million
// rows. No candidate is observable before whole-four second EOF and coverage.
func (c *HistoricalCoordinator) CandidateRange(offset, limit int) ([]HistoricalCandidate, error) {
	if c == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.coverage || c.failed {
		return nil, ErrCoordinatorIncomplete
	}
	if c.now().Sub(c.started) > c.limits.MaxDuration {
		return nil, ErrCoordinatorExpired
	}
	if offset < 0 || offset > len(c.candidates) || limit < 1 || limit > 512 {
		return nil, ErrCoordinatorBounds
	}
	end := offset + limit
	if end > len(c.candidates) {
		end = len(c.candidates)
	}
	out := make([]HistoricalCandidate, end-offset)
	for i := range out {
		out[i] = coordinatorCloneCandidate(c.candidates[offset+i])
	}
	return out, nil
}
func (c *HistoricalCoordinator) PageReceipts(offset, limit int) ([]HistoricalCoordinatorPageReceipt, error) {
	if c == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.coverage || c.failed {
		return nil, ErrCoordinatorIncomplete
	}
	if c.now().Sub(c.started) > c.limits.MaxDuration {
		return nil, ErrCoordinatorExpired
	}
	if offset < 0 || offset > len(c.pages) || limit < 1 || limit > 512 {
		return nil, ErrCoordinatorBounds
	}
	end := offset + limit
	if end > len(c.pages) {
		end = len(c.pages)
	}
	return append([]HistoricalCoordinatorPageReceipt(nil), c.pages[offset:end]...), nil
}
func coordinatorCandidateHash(values []HistoricalCandidate) string {
	h := sha256.New()
	sourceFrame(h, []byte("historical-candidate-sequence/v1"), false)
	sourceFrame(h, []byte(strconv.Itoa(len(values))), false)
	for _, value := range values {
		digest, err := privateFactsSHA(value)
		if err != nil {
			return ""
		}
		sourceFrame(h, digest[:], false)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func coordinatorPageHash(values []HistoricalCoordinatorPageReceipt) string {
	h := sha256.New()
	sourceFrame(h, []byte("historical-page-sequence/v1"), false)
	sourceFrame(h, []byte(strconv.Itoa(len(values))), false)
	for _, value := range values {
		digest, err := privateFactsSHA(value)
		if err != nil {
			return ""
		}
		sourceFrame(h, digest[:], false)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func coordinatorReasons(v []string) []string {
	sort.Strings(v)
	out := v[:0]
	for _, s := range v {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
