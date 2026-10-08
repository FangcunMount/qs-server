package retirement

import (
	"context"
	"io"
	"sync"
	"time"
)

const (
	ErrCoordinatorInvalid    SourceError = "historical_coordinator_invalid"
	ErrCoordinatorBounds     SourceError = "historical_coordinator_budget_exceeded"
	ErrCoordinatorExpired    SourceError = "historical_coordinator_epoch_expired"
	ErrCoordinatorPage       SourceError = "historical_coordinator_page_rejected"
	ErrCoordinatorIncomplete SourceError = "historical_coordinator_whole_coverage_incomplete"
)

// These are limits, never externally asserted completion/authorization flags.
// Logical reservations are not RSS claims. A production host must independently
// enforce and benchmark its process budget before opening an approved copy.
type HistoricalCoordinatorLimits struct {
	MaxPageRecords       int
	MaxPageBytes         uint64
	MaxAICommands        int
	MaxReservationBytes  uint64
	MaxDuration, PageTTL time.Duration
}

func DefaultHistoricalCoordinatorLimits() HistoricalCoordinatorLimits {
	return HistoricalCoordinatorLimits{128, 64 << 20, 1024, 2 << 30, 30 * time.Minute, time.Minute}
}
func (l HistoricalCoordinatorLimits) valid() bool {
	return l.MaxPageRecords >= 2 && l.MaxPageRecords <= 512 && l.MaxPageBytes >= MaxSourceRowBytes && l.MaxPageBytes <= 256<<20 && l.MaxAICommands > 0 && l.MaxAICommands <= 4096 && l.MaxReservationBytes > 0 && l.MaxReservationBytes <= 4<<30 && l.MaxDuration > 0 && l.MaxDuration <= 24*time.Hour && l.PageTTL >= time.Millisecond && l.PageTTL <= 2*time.Minute
}

// Bindings identify a host operation, not an authenticated production origin.
// The current coordinator cannot mint origin, fence, external AI or CAS proofs.
type HistoricalCoordinatorBinding struct{ SourceSHA, OperationID string }

type coordinatorRow struct {
	event          *VerifiedSourceEvent
	bridge, legacy *VerifiedSourceAICommand
	keys           []verifiedSourceKey
	bytes          uint64
}

// HistoricalSourcePage is minted only by NextPage. There is one outstanding
// page. Callers cannot construct rows, change sequence, or submit mutable DTOs.
type HistoricalSourcePage struct {
	owner    *HistoricalCoordinator
	sequence uint64
	issued   time.Time
	rows     []coordinatorRow
	consumed bool
}

func (*HistoricalSourcePage) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalSourcePage) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalSourcePage) String() string               { return "private coordinator source page" }
func (p *HistoricalSourcePage) GoString() string           { return p.String() }

// Events supplies immutable source capabilities for preparing an actual bounded
// SQL/Mongo business batch. It supplies neither facts DTOs nor approval flags.
func (p *HistoricalSourcePage) Events() ([]*VerifiedSourceEvent, error) {
	if p == nil || p.owner == nil {
		return nil, ErrCoordinatorPage
	}
	c := p.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.pageValid(p); err != nil {
		return nil, err
	}
	out := make([]*VerifiedSourceEvent, 0, len(p.rows))
	for _, r := range p.rows {
		if r.event != nil {
			out = append(out, r.event)
		}
	}
	return out, nil
}

// All streams are borrowed and must be seekable. Authentication consumes all
// four to clean EOF; only then does this coordinator rewind them for its OWN
// second complete pass. No host file, transaction or pool is opened or closed.
type HistoricalCoordinator struct {
	mu            sync.Mutex
	binding       HistoricalCoordinatorBinding
	limits        HistoricalCoordinatorLimits
	copies        [4]SourceCopyInput
	seekers       [4]io.ReadSeeker
	authenticated *VerifiedSourceCopies
	remaining     map[verifiedSourceKey]bool
	receipts      [4]SourceCopyReceipt
	consumed      [4]uint64
	started       time.Time
	now           func() time.Time
	object        int
	eventNext     func() (*DecodedSourceEvent, error)
	eventReceipt  func() SourceCopyReceipt
	aiLoaded      bool
	aiRows        []coordinatorRow
	aiCursor      int
	pending       *HistoricalSourcePage
	pendingRow    *coordinatorRow
	sequence      uint64
	pages         []HistoricalCoordinatorPageReceipt
	candidates    []HistoricalCandidate
	coverage      bool
	failed        bool
	reservation   uint64
}

func (*HistoricalCoordinator) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCoordinator) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCoordinator) String() string {
	return "private four-copy historical coordinator; no retirement approval"
}
func (c *HistoricalCoordinator) GoString() string { return c.String() }

func PrepareHistoricalCoordinator(ctx context.Context, binding HistoricalCoordinatorBinding, copies []SourceCopyInput, limits HistoricalCoordinatorLimits) (*HistoricalCoordinator, error) {
	if ctx == nil || ctx.Err() != nil || len(copies) != 4 || !limits.valid() || !aiLocalOperationID(binding.OperationID) || !coordinatorSourceSHA(binding.SourceSHA) {
		return nil, ErrCoordinatorInvalid
	}
	c := &HistoricalCoordinator{binding: binding, limits: limits, started: time.Now(), now: time.Now, remaining: map[verifiedSourceKey]bool{}}
	var records uint64
	for i, v := range copies {
		if sourceReaderAbsent(v.Input) {
			return nil, ErrCoordinatorInvalid
		}
		s, ok := v.Input.(io.ReadSeeker)
		if !ok || sourceReaderAbsent(s) {
			return nil, ErrCoordinatorInvalid
		}
		if v.Expected.Records > limits.MaxReservationBytes/1536-records {
			return nil, ErrCoordinatorBounds
		}
		records += v.Expected.Records
		c.copies[i] = v
		c.seekers[i] = s
		if _, err := s.Seek(0, io.SeekStart); err != nil {
			return nil, ErrCoordinatorInvalid
		}
	}
	if copies[1].Expected.Records > uint64(limits.MaxAICommands) || copies[2].Expected.Records > copies[1].Expected.Records {
		return nil, ErrCoordinatorBounds
	}
	c.reservation = records * 1536
	verified, err := VerifySourceCopies(ctx, copies)
	if err != nil {
		return nil, err
	}
	c.authenticated = verified
	for key := range verified.rows {
		c.remaining[key] = true
	}
	for _, s := range c.seekers {
		if _, err = s.Seek(0, io.SeekStart); err != nil {
			return nil, ErrCoordinatorInvalid
		}
	}
	if err = c.alive(ctx); err != nil {
		return nil, err
	}
	return c, nil
}
func coordinatorSourceSHA(v string) bool { return len(v) == 40 && evidenceHex(v, 20) }
func (c *HistoricalCoordinator) alive(ctx context.Context) error {
	if ctx == nil || c == nil || c.authenticated == nil || c.failed {
		return ErrCoordinatorInvalid
	}
	if ctx.Err() != nil {
		return ErrSourceIncomplete
	}
	if c.now().Sub(c.started) > c.limits.MaxDuration {
		return ErrCoordinatorExpired
	}
	return nil
}
func (c *HistoricalCoordinator) pageValid(p *HistoricalSourcePage) error {
	if c == nil || c.failed || p == nil || p.owner != c || c.pending != p || p.consumed || p.sequence != c.sequence || len(p.rows) == 0 {
		return ErrCoordinatorPage
	}
	if c.now().Sub(p.issued) > c.limits.PageTTL || c.now().Sub(c.started) > c.limits.MaxDuration {
		return ErrCoordinatorExpired
	}
	return nil
}
func (c *HistoricalCoordinator) copyComplete(i int, r SourceCopyReceipt) error {
	e := c.copies[i].Expected
	if !r.Complete || r.BusinessClosureVerified || r.DropReady || r.Records != e.Records || r.Bytes != e.Bytes || r.DataHash != e.DataHash || r != c.authenticated.receipts[i] {
		return ErrSourceIncomplete
	}
	c.receipts[i] = r
	return nil
}

func (c *HistoricalCoordinator) nextRow(ctx context.Context) (*coordinatorRow, error) {
	for c.object < 4 {
		if err := c.alive(ctx); err != nil {
			return nil, err
		}
		if c.object == 1 || c.object == 2 {
			if !c.aiLoaded {
				if err := c.loadAI(ctx); err != nil {
					return nil, err
				}
			}
			if c.aiCursor < len(c.aiRows) {
				r := c.aiRows[c.aiCursor]
				c.aiRows[c.aiCursor] = coordinatorRow{}
				c.aiCursor++
				return &r, nil
			}
			c.aiRows = nil
			c.object = 3
			continue
		}
		if c.eventNext == nil {
			if c.object == 0 {
				r, err := NewSQLSourceReader(c.seekers[0], c.copies[0].Expected)
				if err != nil {
					return nil, err
				}
				c.eventNext = r.Next
				c.eventReceipt = r.Receipt
			} else {
				r, err := NewMongoSourceReader(c.seekers[3], c.copies[3].Expected)
				if err != nil {
					return nil, err
				}
				c.eventNext = r.Next
				c.eventReceipt = r.Receipt
			}
		}
		before := c.eventReceipt().Bytes
		event, err := c.eventNext()
		if err == io.EOF {
			if err = c.copyComplete(c.object, c.eventReceipt()); err != nil {
				return nil, err
			}
			c.eventNext = nil
			c.eventReceipt = nil
			c.object++
			continue
		}
		if err != nil {
			return nil, err
		}
		bound, err := c.authenticated.BindEvent(event)
		if err != nil {
			return nil, err
		}
		key, err := sourceAuthKey(event.Source.Database, event.Source.Object, event.Source.PrimaryKeySHA256)
		if err != nil || !c.remaining[key] {
			return nil, ErrSourceIdentity
		}
		return &coordinatorRow{event: bound, keys: []verifiedSourceKey{key}, bytes: c.eventReceipt().Bytes - before}, nil
	}
	return nil, io.EOF
}

func (c *HistoricalCoordinator) loadAI(ctx context.Context) error {
	byID := map[string]int{}
	for i := 1; i <= 2; i++ {
		r, err := NewAISQLSourceReader(c.seekers[i], c.copies[i].Expected)
		if err != nil {
			return err
		}
		for {
			if err = c.alive(ctx); err != nil {
				return err
			}
			before := r.Receipt().Bytes
			command, e := r.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			handle, e := c.authenticated.BindAICommand(command)
			if e != nil {
				return e
			}
			key, e := sourceAuthKey(command.Source.Database, command.Source.Object, command.Source.PrimaryKeySHA256)
			if e != nil || !c.remaining[key] {
				return ErrSourceIdentity
			}
			if i == 1 {
				if _, exists := byID[command.CommandID]; exists {
					return ErrSourceIdentity
				}
				byID[command.CommandID] = len(c.aiRows)
				c.aiRows = append(c.aiRows, coordinatorRow{bridge: handle, keys: []verifiedSourceKey{key}, bytes: r.Receipt().Bytes - before})
			} else {
				index, exists := byID[command.CommandID]
				if !exists || c.aiRows[index].legacy != nil {
					return ErrAISourceHandoff
				}
				row := &c.aiRows[index]
				bridge, e := row.bridge.Facts()
				if e != nil {
					return e
				}
				a, e := aiSharedFactsSHA(bridge)
				if e != nil {
					return e
				}
				b, e := aiSharedFactsSHA(command)
				if e != nil || a != b || bridge.Transport.Delivered == nil || *bridge.Transport.Delivered {
					return ErrAISourceHandoff
				}
				row.legacy = handle
				row.keys = append(row.keys, key)
				row.bytes += r.Receipt().Bytes - before
			}
		}
		if err = c.copyComplete(i, r.Receipt()); err != nil {
			return err
		}
	}
	c.aiLoaded = true
	return nil
}

// EOF is returned only after all pages have been consumed and all four second
// pass digests/counts match the approved first pass, including empty copies.
func (c *HistoricalCoordinator) NextPage(ctx context.Context) (*HistoricalSourcePage, error) {
	if c == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if c.pending != nil {
		return nil, ErrCoordinatorPage
	}
	if c.coverage {
		return nil, io.EOF
	}
	p := &HistoricalSourcePage{owner: c, sequence: c.sequence + 1, issued: c.now()}
	var bytes uint64
	for {
		var row *coordinatorRow
		var err error
		if c.pendingRow != nil {
			row = c.pendingRow
			c.pendingRow = nil
		} else {
			row, err = c.nextRow(ctx)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			c.failed = true
			return nil, err
		}
		if row.bytes > c.limits.MaxPageBytes {
			c.failed = true
			return nil, ErrCoordinatorBounds
		}
		n := 0
		for _, r := range p.rows {
			n += len(r.keys)
		}
		if len(p.rows) > 0 && ((p.rows[0].event == nil) != (row.event == nil)) || n+len(row.keys) > c.limits.MaxPageRecords || bytes+row.bytes > c.limits.MaxPageBytes {
			c.pendingRow = row
			break
		}
		p.rows = append(p.rows, *row)
		bytes += row.bytes
		if n+len(row.keys) == c.limits.MaxPageRecords {
			break
		}
	}
	if len(p.rows) == 0 {
		if len(c.remaining) != 0 || uint64(len(c.candidates)) != c.authenticated.entries {
			c.failed = true
			return nil, ErrCoordinatorIncomplete
		}
		for i, r := range c.receipts {
			if !r.Complete || c.consumed[i] != r.Records {
				c.failed = true
				return nil, ErrCoordinatorIncomplete
			}
		}
		c.coverage = true
		return nil, io.EOF
	}
	c.sequence++
	c.pending = p
	return p, nil
}
