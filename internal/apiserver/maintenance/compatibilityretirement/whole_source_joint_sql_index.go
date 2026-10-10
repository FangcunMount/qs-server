package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"reflect"
	"sort"
	"strconv"
)

const (
	ErrWholeSourceJoint       SourceError = "whole_source_joint_qualification_rejected"
	ErrWholeSourceJointBounds SourceError = "whole_source_joint_budget_exceeded"
)

// Input is borrowed. It is deliberately ReaderAt: an independent full pass
// must not change the coordinator's own ongoing second-pass cursor.
type WholeSourceJointCopy struct {
	Input    io.ReaderAt
	Expected SourceCopyExpectation
}

type WholeSourceJointLimits struct {
	MaxIndexEntries, MaxIndexReservationBytes, MaxEncodedCopyBytes uint64
	MaxRelatedSources                                              int
}

func DefaultWholeSourceJointLimits() WholeSourceJointLimits {
	return WholeSourceJointLimits{2_000_000, 2 << 30, 8 << 30, 128}
}

func (l WholeSourceJointLimits) valid() bool {
	return l.MaxIndexEntries > 0 && l.MaxIndexEntries <= 2_000_000 && l.MaxIndexReservationBytes > 0 && l.MaxIndexReservationBytes <= 4<<30 && l.MaxEncodedCopyBytes > 0 && l.MaxEncodedCopyBytes <= 32<<30 && l.MaxRelatedSources > 0 && l.MaxRelatedSources <= 512
}

type wholeSourceJointEntry struct {
	Key                                            verifiedSourceKey
	Offset, Length                                 int64
	PhysicalSHA256                                 string
	FactsSHA256                                    [32]byte
	EventID, EventType, AggregateType, AggregateID string
	OrgID, AssessmentID, AnswerSheetID             uint64
}

// The index retains frame locations and immutable identities, never payloads.
// Its own four clean EOF receipts are in addition to the original source seal
// and the coordinator's independent exactly-once consumption pass.
type WholeSourceJointIndex struct {
	self                  *WholeSourceJointIndex
	binding               HistoricalCoordinatorBinding
	input                 *HistoricalSourceInputRecipe
	inputSeal             string
	owner                 *HistoricalCoordinator
	auth                  *VerifiedSourceCopies
	copies                [4]WholeSourceJointCopy
	headers               [4][]byte
	encodedSHA            [4]string
	receipts              [4]SourceCopyReceipt
	entries               map[string]wholeSourceJointEntry
	byAssessment, bySheet map[uint64][]string
	limits                WholeSourceJointLimits
	indexSHA              string
	complete              bool
}

type WholeSourceJointIndexSummary struct {
	Complete                                                         bool
	Entries, ReservationBytes                                        uint64
	IndexSHA256                                                      string
	Copies                                                           [4]SourceCopyReceipt
	PhysicalEncodingSHA256                                           [4]string
	ExternalOriginRequired, BusinessQualificationRequired, DropReady bool
}

func (*WholeSourceJointIndex) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*WholeSourceJointIndex) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*WholeSourceJointIndex) String() string {
	return "private body-free whole-source joint index; not closure proof"
}
func (x *WholeSourceJointIndex) GoString() string { return x.String() }
func (x *WholeSourceJointIndex) Summary() WholeSourceJointIndexSummary {
	s := WholeSourceJointIndexSummary{ExternalOriginRequired: true, BusinessQualificationRequired: true}
	if x != nil {
		s.Complete = x.complete
		s.Entries = uint64(len(x.entries))
		s.ReservationBytes = s.Entries * 1024
		s.IndexSHA256 = x.indexSHA
		s.Copies = x.receipts
		s.PhysicalEncodingSHA256 = x.encodedSHA
	}
	return s
}

// SQL scanner reads are stopped at actual newline boundaries so positions
// reflect the original encoded frame, not Scanner's otherwise hidden prefetch.
// At the byte cap, probe the real input: a synthetic EOF is never success.
type wholeSourceJointAtReader struct {
	input           io.ReaderAt
	position, limit int64
	line            bool
	h               hash.Hash
}

func (r *wholeSourceJointAtReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.position >= r.limit {
		var probe [1]byte
		n, e := r.input.ReadAt(probe[:], r.position)
		if n == 0 && e == io.EOF {
			return 0, io.EOF
		}
		return 0, ErrWholeSourceJointBounds
	}
	if int64(len(p)) > r.limit-r.position {
		p = p[:int(r.limit-r.position)]
	}
	n, e := r.input.ReadAt(p, r.position)
	if n < 0 || n > len(p) || n == 0 && e == nil {
		return 0, ErrSourceProtocol
	}
	if r.line {
		if end := bytes.IndexByte(p[:n], '\n'); end >= 0 {
			n = end + 1
			e = nil
		}
	}
	if n > 0 {
		_, _ = r.h.Write(p[:n])
		r.position += int64(n)
	}
	return n, e
}

func (c *HistoricalCoordinator) PrepareWholeSourceJointIndex(ctx context.Context, copies []WholeSourceJointCopy, limits WholeSourceJointLimits) (*WholeSourceJointIndex, error) {
	if c == nil || len(copies) != 4 || !limits.valid() {
		return nil, ErrWholeSourceJoint
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	x := &WholeSourceJointIndex{owner: c, auth: c.authenticated, limits: limits}
	var expected [4]SourceCopyExpectation
	for i := range expected {
		expected[i] = c.copies[i].Expected
	}
	return readWholeSourceJointIndex(ctx, x, copies, expected, c.alive)
}

// PrepareHistoricalSourceInputIndex indexes actual authenticated source bytes
// for planning only. It borrows one original recipe/auth index, does not create
// a coordinator, and never renews an origin or business qualification lifetime.
func PrepareHistoricalSourceInputIndex(ctx context.Context, binding HistoricalCoordinatorBinding, actual *HistoricalSourceInputRecipe, copies []WholeSourceJointCopy, limits WholeSourceJointLimits) (*WholeSourceJointIndex, error) {
	if !coordinatorSourceSHA(binding.SourceSHA) || !aiLocalOperationID(binding.OperationID) || actual == nil || !actual.valid() || actual.captureStopped || len(copies) != 4 || !limits.valid() {
		return nil, ErrWholeSourceJoint
	}
	x := &WholeSourceJointIndex{binding: binding, input: actual, auth: actual.binding.copies, limits: limits}
	recipeSHA := actual.hash
	validate := func(ctx context.Context) error {
		// The immutable full recipe is checked before/after the physical pass;
		// per-row checks must not re-encode it or clone the million-row auth map.
		if ctx == nil || ctx.Err() != nil || actual.self != actual || actual.hash != recipeSHA || actual.binding.hash != recipeSHA || actual.captureStopped || actual.binding.copies != x.auth {
			return ErrSourceOrigin
		}
		return nil
	}
	x, err := readWholeSourceJointIndex(ctx, x, copies, actual.binding.expected, validate)
	if err != nil {
		return nil, err
	}
	if !actual.valid() || x.encodedSHA != actual.binding.fileHashes || x.receipts != x.auth.receipts {
		return nil, ErrSourceAuthentication
	}
	x.self = x
	x.inputSeal = x.inputDigest()
	return x, nil
}

// The two callers supply their original strict guard. This shared physical
// reader grants no capability and preserves all four real EOF/auth checks.
func readWholeSourceJointIndex(ctx context.Context, x *WholeSourceJointIndex, copies []WholeSourceJointCopy, expected [4]SourceCopyExpectation, validate func(context.Context) error) (*WholeSourceJointIndex, error) {
	if x == nil || x.auth == nil || !x.auth.complete || len(copies) != 4 || !x.limits.valid() || validate == nil {
		return nil, ErrWholeSourceJoint
	}
	if err := validate(ctx); err != nil {
		return nil, err
	}
	limits := x.limits
	x.byAssessment, x.bySheet = map[uint64][]string{}, map[uint64][]string{}
	var records uint64
	for i, v := range copies {
		if wholeJointReaderAbsent(v.Input) || !reflect.DeepEqual(v.Expected, expected[i]) || v.Expected.Records > limits.MaxIndexEntries-records {
			return nil, ErrWholeSourceJoint
		}
		records += v.Expected.Records
		x.copies[i] = v
	}
	if records > limits.MaxIndexReservationBytes/1024 {
		return nil, ErrWholeSourceJointBounds
	}
	// Use the original authenticated EOF counts only after all four new
	// expectations and the existing entry/reservation bounds have matched.
	eventRecords := x.auth.receipts[0].Records + x.auth.receipts[3].Records
	x.entries = make(map[string]wholeSourceJointEntry, int(eventRecords))
	for i, v := range copies {
		r := &wholeSourceJointAtReader{input: v.Input, limit: int64(limits.MaxEncodedCopyBytes), line: i != 3, h: sha256.New()}
		var next func() (*DecodedSourceEvent, error)
		var receipt func() SourceCopyReceipt
		switch i {
		case 0:
			s, e := NewSQLSourceReader(r, v.Expected)
			if e != nil {
				return nil, e
			}
			next = s.Next
			receipt = s.Receipt
		case 3:
			s, e := NewMongoSourceReader(r, v.Expected)
			if e != nil {
				return nil, e
			}
			next = s.Next
			receipt = s.Receipt
		default:
			s, e := NewAISQLSourceReader(r, v.Expected)
			if e != nil {
				return nil, e
			}
			for {
				if e = validate(ctx); e != nil {
					return nil, e
				}
				command, e := s.Next()
				if e == io.EOF {
					break
				}
				if e != nil {
					return nil, e
				}
				if _, e = x.auth.BindAICommand(command); e != nil {
					return nil, e
				}
			}
			x.receipts[i] = s.Receipt()
			x.encodedSHA[i] = hex.EncodeToString(r.h.Sum(nil))
			if !reflect.DeepEqual(x.receipts[i], x.auth.receipts[i]) {
				return nil, ErrSourceIncomplete
			}
			continue
		}
		if i == 0 {
			if r.position <= 0 || r.position > 2*MaxSourceRowBytes {
				return nil, ErrWholeSourceJointBounds
			}
			x.headers[i] = make([]byte, int(r.position))
			if _, e := v.Input.ReadAt(x.headers[i], 0); e != nil {
				return nil, ErrSourceIncomplete
			}
		}
		for {
			if e := validate(ctx); e != nil {
				return nil, e
			}
			before := r.position
			event, e := next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			bound, e := x.auth.BindEvent(event)
			if e != nil {
				return nil, e
			}
			if bound == nil || bound.facts == nil {
				return nil, ErrSourceAuthentication
			}
			// BindEvent already compared both the original DTO and its detached
			// clone with the sealed facts. Keep that private clone for all fields
			// below instead of discarding it and hashing the original a third time.
			event = bound.facts
			length := r.position - before
			if length <= 0 || length > 2*MaxSourceRowBytes+8 {
				return nil, ErrWholeSourceJointBounds
			}
			frame := make([]byte, int(length))
			if _, e = v.Input.ReadAt(frame, before); e != nil {
				return nil, ErrSourceIncomplete
			}
			key, e := sourceAuthKey(event.Source.Database, event.Source.Object, event.Source.PrimaryKeySHA256)
			if e != nil {
				return nil, e
			}
			approved, exists := x.auth.rows[key]
			if !exists {
				return nil, ErrSourceAuthentication
			}
			if _, exists := x.entries[event.EventID]; exists {
				return nil, ErrSourceIdentity
			}
			physical := sha256.Sum256(frame)
			entry := wholeSourceJointEntry{Key: key, Offset: before, Length: length, PhysicalSHA256: hex.EncodeToString(physical[:]), FactsSHA256: approved.facts, EventID: event.EventID, EventType: event.EventType, AggregateType: event.AggregateType, AggregateID: event.AggregateID, OrgID: event.OrgID}
			if event.Submitted != nil {
				entry.AnswerSheetID, e = mongoCycleStringID(event.Submitted.AnswerSheetID)
			} else if event.Generated != nil {
				entry.AssessmentID, e = mongoCycleStringID(event.Generated.AssessmentID)
			} else {
				entry.AssessmentID, e = sqlSourceAssessment(event)
			}
			if e != nil {
				return nil, e
			}
			x.entries[event.EventID] = entry
			if entry.AssessmentID != 0 {
				x.byAssessment[entry.AssessmentID] = append(x.byAssessment[entry.AssessmentID], entry.EventID)
			}
			if entry.AnswerSheetID != 0 {
				x.bySheet[entry.AnswerSheetID] = append(x.bySheet[entry.AnswerSheetID], entry.EventID)
			}
		}
		x.receipts[i] = receipt()
		x.encodedSHA[i] = hex.EncodeToString(r.h.Sum(nil))
		if !reflect.DeepEqual(x.receipts[i], x.auth.receipts[i]) {
			return nil, ErrSourceIncomplete
		}
	}
	if err := validate(ctx); err != nil {
		return nil, err
	}
	x.indexSHA = x.digest()
	x.complete = true
	return x, nil
}

func wholeJointReaderAbsent(reader io.ReaderAt) bool {
	if reader == nil {
		return true
	}
	v := reflect.ValueOf(reader)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

func (x *WholeSourceJointIndex) digest() string {
	h := sha256.New()
	sourceFrame(h, []byte("whole-source-joint-index/v1"), false)
	for _, s := range x.encodedSHA {
		sourceFrame(h, []byte(s), false)
	}
	ids := make([]string, 0, len(x.entries))
	for id := range x.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := x.entries[id]
		for _, v := range []string{e.EventID, e.EventType, e.AggregateType, e.AggregateID, strconv.FormatUint(e.OrgID, 10), strconv.FormatUint(e.AssessmentID, 10), strconv.FormatUint(e.AnswerSheetID, 10), strconv.FormatInt(e.Offset, 10), strconv.FormatInt(e.Length, 10), e.PhysicalSHA256, hex.EncodeToString(e.FactsSHA256[:]), strconv.Itoa(int(e.Key.object)), hex.EncodeToString(e.Key.pk[:])} {
			sourceFrame(h, []byte(v), false)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (x *WholeSourceJointIndex) event(ctx context.Context, id string) (*VerifiedSourceEvent, error) {
	if x == nil || !x.complete || x.owner == nil || x.auth != x.owner.authenticated || ctx == nil || ctx.Err() != nil {
		return nil, ErrWholeSourceJoint
	}
	facts, err := x.readEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	return x.auth.BindEvent(facts)
}

func (x *WholeSourceJointIndex) readEvent(ctx context.Context, id string) (*DecodedSourceEvent, error) {
	if x == nil || !x.complete || ctx == nil || ctx.Err() != nil {
		return nil, ErrWholeSourceJoint
	}
	e, ok := x.entries[id]
	if !ok {
		return nil, ErrSourceAuthentication
	}
	frame := make([]byte, int(e.Length))
	if _, err := x.copies[e.Key.object].Input.ReadAt(frame, e.Offset); err != nil {
		return nil, ErrSourceIncomplete
	}
	digest := sha256.Sum256(frame)
	if hex.EncodeToString(digest[:]) != e.PhysicalSHA256 {
		return nil, ErrSourceAuthentication
	}
	var facts *DecodedSourceEvent
	var err error
	switch e.Key.object {
	case 0:
		r, e2 := NewSQLSourceReader(io.MultiReader(bytes.NewReader(x.headers[0]), bytes.NewReader(frame)), x.copies[0].Expected)
		if e2 != nil {
			return nil, e2
		}
		facts, err = r.Next()
	case 3:
		r, e2 := NewMongoSourceReader(bytes.NewReader(frame), x.copies[3].Expected)
		if e2 != nil {
			return nil, e2
		}
		facts, err = r.Next()
	default:
		return nil, ErrWholeSourceJoint
	}
	if err != nil {
		return nil, err
	}
	hash, err := privateFactsSHA(facts)
	if err != nil || hash != e.FactsSHA256 || facts.EventID != id {
		return nil, ErrSourceAuthentication
	}
	return facts, nil
}

// Re-authenticate all physical frames and all four logical source receipts to
// actual EOF. Random-access page decoding is never advertised as fresh EOF.
func (x *WholeSourceJointIndex) RecheckSourceCopies(ctx context.Context, copies []WholeSourceJointCopy) error {
	if x == nil || !x.complete || x.owner == nil || x.auth != x.owner.authenticated {
		return ErrWholeSourceJoint
	}
	fresh, err := x.owner.PrepareWholeSourceJointIndex(ctx, copies, x.limits)
	if err != nil {
		return err
	}
	if fresh.indexSHA != x.indexSHA || fresh.encodedSHA != x.encodedSHA || !reflect.DeepEqual(fresh.receipts, x.receipts) {
		return ErrSourceAuthentication
	}
	return nil
}

// inputDigest binds only immutable planning provenance, not a write permit.
func (x *WholeSourceJointIndex) inputDigest() string {
	if x == nil || x.input == nil {
		return ""
	}
	return mongoOwnerHashParts("historical-source-input-index/v1", x.binding.SourceSHA, x.binding.OperationID, x.input.hash, x.indexSHA)
}

func (x *WholeSourceJointIndex) inputIndexIntact(ctx context.Context, pair *HistoricalSourceInputPair, requireAuth bool) error {
	if ctx == nil || ctx.Err() != nil || x == nil || x.self != x || x.owner != nil || !x.complete || x.input == nil || !x.input.valid() || x.inputSeal == "" || x.inputSeal != x.inputDigest() || sourceComponentPairIntact(ctx, pair) != nil || pair.first.recipe != x.input || pair.second.recipe != x.input || x.receipts != pair.second.receipts || x.encodedSHA != x.input.binding.fileHashes {
		return ErrWholeSourceJoint
	}
	if requireAuth && (x.input.captureStopped || x.auth == nil || x.auth != x.input.binding.copies) {
		return ErrSourceOrigin
	}
	return nil
}

// InputEvents binds only requested actual original frames, after the two real
// input reads matched. The original SECOND SQL/Mongo read scope must still be
// alive; ending it or releasing membership prevents further handle issuance.
func (x *WholeSourceJointIndex) InputEvents(ctx context.Context, pair *HistoricalSourceInputPair, ids []string) ([]*VerifiedSourceEvent, error) {
	if x.inputIndexIntact(ctx, pair, true) != nil || !pair.first.captureStopped || pair.second.alive(ctx) != nil || pair.second.sql.ValidateBorrowedSnapshot(ctx) != nil || pair.second.mongo.ValidateBorrowedInputEpoch(ctx) != nil || len(ids) == 0 || len(ids) > x.limits.MaxRelatedSources {
		return nil, ErrWholeSourceJoint
	}
	out := make([]*VerifiedSourceEvent, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return nil, ErrSourceIdentity
		}
		seen[id] = true
		facts, err := x.readEvent(ctx, id)
		if err != nil {
			return nil, err
		}
		event, err := x.auth.BindEvent(facts)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	if pair.second.alive(ctx) != nil || pair.second.sql.ValidateBorrowedSnapshot(ctx) != nil || pair.second.mongo.ValidateBorrowedInputEpoch(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	return out, nil
}

// ReleaseInputAuthentication drops this pure index's membership reference
// after actual input scopes ended. It never mutates an existing old capability.
// The host also calls pair.ReleaseCaptureIndex to release its recipe's maps;
// only one offset/facts/owner index remains for selected fresh component reads.
func (x *WholeSourceJointIndex) ReleaseInputAuthentication(ctx context.Context, pair *HistoricalSourceInputPair) error {
	if x.inputIndexIntact(ctx, pair, false) != nil || !pair.first.captureStopped || !pair.second.captureStopped || pair.ValidateFrozen(ctx) != nil {
		return ErrWholeSourceJoint
	}
	x.auth = nil
	return nil
}
