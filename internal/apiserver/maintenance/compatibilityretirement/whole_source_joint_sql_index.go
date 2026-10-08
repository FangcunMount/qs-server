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
	x := &WholeSourceJointIndex{owner: c, auth: c.authenticated, entries: map[string]wholeSourceJointEntry{}, byAssessment: map[uint64][]string{}, bySheet: map[uint64][]string{}, limits: limits}
	var records uint64
	for i, v := range copies {
		if wholeJointReaderAbsent(v.Input) || !reflect.DeepEqual(v.Expected, c.copies[i].Expected) || v.Expected.Records > limits.MaxIndexEntries-records {
			return nil, ErrWholeSourceJoint
		}
		records += v.Expected.Records
		x.copies[i] = v
	}
	if records > limits.MaxIndexReservationBytes/1024 {
		return nil, ErrWholeSourceJointBounds
	}
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
				if e = c.alive(ctx); e != nil {
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
			if e := c.alive(ctx); e != nil {
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
			if _, e = x.auth.BindEvent(event); e != nil {
				return nil, e
			}
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
			facts, e := privateFactsSHA(event)
			if e != nil {
				return nil, e
			}
			if _, exists := x.entries[event.EventID]; exists {
				return nil, ErrSourceIdentity
			}
			physical := sha256.Sum256(frame)
			entry := wholeSourceJointEntry{Key: key, Offset: before, Length: length, PhysicalSHA256: hex.EncodeToString(physical[:]), FactsSHA256: facts, EventID: event.EventID, EventType: event.EventType, AggregateType: event.AggregateType, AggregateID: event.AggregateID, OrgID: event.OrgID}
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
	return x.auth.BindEvent(facts)
}

// Re-authenticate all physical frames and all four logical source receipts to
// actual EOF. Random-access page decoding is never advertised as fresh EOF.
func (x *WholeSourceJointIndex) RecheckSourceCopies(ctx context.Context, copies []WholeSourceJointCopy) error {
	if x == nil || !x.complete {
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
