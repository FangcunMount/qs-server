package retirement

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"syscall"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

const aiHistoricalInputMaxRetained = 256 << 20

type aiHistoricalInputSink func(context.Context, AIReverseLedgerSummary, []aiReverseRow, bool) error
type aiHistoricalInputFrame struct {
	Version            int
	Epoch, SourceInput string
	Ledger             AIReverseLedgerSummary
	RowsJSON           []byte
	EOF                bool
}

// AIHistoricalInputEpoch captures the initial full AI input once, not once per
// event/component. Exact raw pages live on disk; compact nodes use an explicit
// bounded reservation during reverse classification. No old capability is
// extended and no physical/CAS/remote/fence authority is published here.
type AIHistoricalInputEpoch struct {
	self               *AIHistoricalInputEpoch
	source             *HistoricalSourceInputEpoch
	pool               any
	cycle, epoch       string
	started            time.Time
	limits             AIReverseLimits
	file               *os.File
	dev, ino           uint64
	end                int64
	pages              []historicalSpoolRef
	report             AIReverseSummary
	complete, poisoned bool
	// Pure typed relations to protected original pages; no live graph/pool is retained.
	componentRequests map[string][]string
	componentPages    map[string][]int
	metadata          []aiReverseMetadata
	componentIndexSHA string
}
type AIHistoricalInputPair struct {
	self          *AIHistoricalInputPair
	first, second *AIHistoricalInputEpoch
	sources       *HistoricalSourceInputPair
}
type AIHistoricalInputSummary struct {
	Protocol, EpochID, DatabaseIdentitySHA256, DataSHA256, SourceScopeSHA256, SourceInputSHA256                             string
	Sources                                                                                                                 [4]SourceCopyReceipt
	Ledgers                                                                                                                 []AIReverseLedgerSummary
	Rows, Bytes, Pages, Related, OutsideRetirement, Unknown, Blocking                                                       uint64
	CompleteInput, TwoIndependentInputsMatched                                                                              bool
	FreshComponentQualificationRequired, ExternalQSAIClosureRequired, StoredWireAuthenticationRequired, WriterFenceRequired bool
	CASAuthority, DropReady, BusinessClosureVerified                                                                        bool
}

func (*AIHistoricalInputEpoch) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIHistoricalInputEpoch) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIHistoricalInputPair) MarshalJSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*AIHistoricalInputPair) MarshalBSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*AIHistoricalInputEpoch) String() string {
	return "private initial full AI input; scoped live qualification required"
}
func (*AIHistoricalInputPair) String() string {
	return "private two AI inputs; no write or writer-fence authority"
}
func (e *AIHistoricalInputEpoch) alive(ctx context.Context) error {
	if e == nil || e.self != e || e.poisoned || ctx == nil || ctx.Err() != nil || !time.Now().Before(e.started.Add(e.limits.MaxDuration)) || e.source.alive(ctx) != nil {
		return ErrAIReverseBounds
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool != e.pool {
		return ErrAIReverseFresh
	}
	return nil
}
func (e *AIHistoricalInputEpoch) validFile(ctx context.Context) error {
	if e == nil || e.self != e || e.poisoned || e.file == nil || ctx == nil || ctx.Err() != nil {
		return ErrAIReverseBinding
	}
	info, err := e.file.Stat()
	if err != nil {
		return ErrAIReverseBinding
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || uint64(st.Dev) != e.dev || uint64(st.Ino) != e.ino || info.Size() != e.end {
		return ErrAIReverseBinding
	}
	return nil
}
func (e *AIHistoricalInputEpoch) freeze(ctx context.Context, l AIReverseLedgerSummary, rows []aiReverseRow, eof bool) error {
	if e.alive(ctx) != nil || e.validFile(ctx) != nil || len(e.pages) > int(e.limits.MaxRows)+len(aiReverseSpecs) {
		return ErrAIReverseBounds
	}
	// JSON base64 values preserve source bytes and distinguish SQL NULL (null)
	// from an existing empty byte value (""). Bodies are not interpreted here.
	values, err := json.Marshal(rows)
	if err != nil {
		return ErrAIReverseRead
	}
	f := aiHistoricalInputFrame{1, e.epoch, e.source.resultHash, l, values, eof}
	raw, err := historicalSpoolEncode(f)
	if err != nil || len(raw) > 2*sourceOriginInputPageBytes+(1<<20) || uint64(e.end)+uint64(len(raw)) > 2*e.limits.MaxBytes+(64<<20) {
		return ErrAIReverseBounds
	}
	ref := historicalSpoolRef{e.end, int64(len(raw)), historicalSpoolSHA(raw)}
	n, err := e.file.WriteAt(raw, e.end)
	if err != nil || n != len(raw) {
		e.poisoned = true
		return ErrAIReverseRead
	}
	e.end += int64(n)
	if e.file.Sync() != nil || e.validFile(ctx) != nil || e.alive(ctx) != nil {
		e.poisoned = true
		return ErrAIReverseRead
	}
	e.pages = append(e.pages, ref)
	return nil
}

// The host supplies four freshly rewound original streams, the live actual
// source epoch in this SQL scope, and an empty protected spool. The existing
// source-scope binding authenticates all four copies to EOF, retaining unknown
// and held/orphan facts. It does not imply qs-ai remote business completion.
func PrepareAIHistoricalInputEpoch(parent context.Context, coordinator *HistoricalCoordinator, source *HistoricalSourceInputEpoch, copies []SourceCopyInput, file *os.File, limits AIReverseLimits) (result *AIHistoricalInputEpoch, err error) {
	if parent == nil || source == nil || !source.complete || source.alive(parent) != nil || source.verifyFrozen(parent) != nil || len(copies) != 4 || file == nil || !limits.valid() || limits.MaxRetainedBytes > aiHistoricalInputMaxRetained {
		return nil, ErrAIReverseBinding
	}
	for i := range copies {
		if !reflect.DeepEqual(copies[i].Expected, source.recipe.binding.expected[i]) {
			return nil, ErrAIReverseBinding
		}
	}
	info, err := file.Stat()
	if err != nil {
		return nil, ErrAIReverseBinding
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || info.Size() != 0 {
		return nil, ErrAIReverseBinding
	}
	e := &AIHistoricalInputEpoch{source: source, pool: source.sqlConnection, cycle: source.sqlCycleID, epoch: source.epochHash, started: time.Now(), limits: limits, file: file, dev: uint64(st.Dev), ino: uint64(st.Ino)}
	e.self = e
	defer func() {
		if err != nil {
			e.poisoned = true
		}
	}()
	ctx, cancel := context.WithDeadline(parent, e.started.Add(limits.MaxDuration))
	defer cancel()
	s, err := prepareAIReverseSnapshotWithInput(ctx, &SQLResponsibilitySnapshot{cycle: source.sql}, 99, limits, e.freeze)
	if err != nil {
		return nil, err
	}
	if coordinator == nil {
		err = bindAIReverseInputSourceScope(ctx, s, source, copies)
	} else {
		err = coordinator.BindAIReverseSourceScope(ctx, s, copies)
	}
	if err != nil {
		return nil, err
	}
	if e.alive(ctx) != nil || source.sql.ValidateBorrowedSnapshot(ctx) != nil || s.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAIReverseFresh
	}
	e.report = s.Summary()
	if !e.report.ActualReadOnlyRR || !e.report.WholeLedgerEOF || e.report.ResponsibilityCycleID != e.cycle || e.report.SourceCopies != source.receipts || len(e.report.Ledgers) != len(aiReverseSpecs) || e.report.SourceScopeSHA256 == "" {
		return nil, ErrAIReverseBinding
	}
	if e.validFile(ctx) != nil {
		return nil, ErrAIReverseBinding
	}
	if err = e.freezeComponentIndex(s); err != nil {
		return nil, err
	}
	e.complete = true
	return e, nil
}
func (e *AIHistoricalInputEpoch) verifyFrozen(ctx context.Context) error {
	if e.validFile(ctx) != nil || !e.complete || e.source.verifyFrozen(ctx) != nil {
		return ErrAIReverseBinding
	}
	table := 0
	var ended bool
	for _, ref := range e.pages {
		if ctx.Err() != nil {
			return ErrAIReverseRead
		}
		if ref.Offset < 0 || ref.Length <= 0 || ref.Length > 2*sourceOriginInputPageBytes+(1<<20) || ref.Offset > e.end-ref.Length {
			return ErrAIReverseRead
		}
		raw := make([]byte, int(ref.Length))
		n, err := e.file.ReadAt(raw, ref.Offset)
		if err != nil || n != len(raw) || historicalSpoolSHA(raw) != ref.SHA256 {
			return ErrAIReverseRead
		}
		var f aiHistoricalInputFrame
		if historicalSpoolDecode(raw, &f) != nil || f.Version != 1 || f.Epoch != e.epoch || f.SourceInput != e.source.resultHash || table >= len(aiReverseSpecs) || f.Ledger.Store != aiReverseSpecs[table].table || ended {
			return ErrAIReverseBinding
		}
		if f.EOF {
			if f.Ledger != e.report.Ledgers[table] || string(f.RowsJSON) != "null" {
				return ErrAIReverseBinding
			}
			table++
			ended = table == len(aiReverseSpecs)
		}
	}
	if !ended || e.componentIndexSHA == "" || e.componentIndexSHA != e.componentIndexDigest() {
		return ErrAIReverseBinding
	}
	return e.validFile(ctx)
}
func (e *AIHistoricalInputEpoch) Summary() AIHistoricalInputSummary {
	r := AIHistoricalInputSummary{Protocol: "historical-ai-input/v1", FreshComponentQualificationRequired: true, ExternalQSAIClosureRequired: true, StoredWireAuthenticationRequired: true, WriterFenceRequired: true}
	if e != nil && e.self == e && e.complete && !e.poisoned {
		r.EpochID = e.report.EpochID
		r.DatabaseIdentitySHA256 = e.report.DatabaseIdentitySHA256
		r.DataSHA256 = e.report.DataSHA256
		r.SourceScopeSHA256 = e.report.SourceScopeSHA256
		r.SourceInputSHA256 = e.source.resultHash
		r.Sources = e.report.SourceCopies
		r.Ledgers = append([]AIReverseLedgerSummary(nil), e.report.Ledgers...)
		r.Rows = e.report.Rows
		r.Bytes = e.report.Bytes
		r.Pages = uint64(len(e.pages))
		r.Related = e.report.Related
		r.OutsideRetirement = e.report.OutsideRetirement
		r.Unknown = e.report.Unknown
		r.Blocking = e.report.Blocking
		r.CompleteInput = true
	}
	return r
}
func CompareIndependentAIHistoricalInputs(ctx context.Context, first, second *AIHistoricalInputEpoch, sources *HistoricalSourceInputPair) (*AIHistoricalInputPair, error) {
	if first == nil || second == nil || first == second || sources == nil || sources.ValidateFrozen(ctx) != nil || first.verifyFrozen(ctx) != nil || second.verifyFrozen(ctx) != nil {
		return nil, ErrAIReverseBinding
	}
	if first.source != sources.first || second.source != sources.second || first.pool == second.pool || first.cycle == second.cycle || first.report.EpochID == second.report.EpochID {
		return nil, ErrAIReverseFresh
	}
	if !reflect.DeepEqual(first.componentRequests, second.componentRequests) || !reflect.DeepEqual(first.componentPages, second.componentPages) || !reflect.DeepEqual(first.metadata, second.metadata) {
		return nil, ErrAIReverseChanged
	}
	a, b := first.report, second.report
	if a.DatabaseIdentitySHA256 != b.DatabaseIdentitySHA256 || a.DataSHA256 != b.DataSHA256 || a.SourceScopeSHA256 != b.SourceScopeSHA256 || a.SourceCopies != b.SourceCopies || !reflect.DeepEqual(a.Ledgers, b.Ledgers) || a.Related != b.Related || a.OutsideRetirement != b.OutsideRetirement || a.Unknown != b.Unknown || a.Blocking != b.Blocking {
		return nil, ErrAIReverseChanged
	}
	p := &AIHistoricalInputPair{first: first, second: second, sources: sources}
	p.self = p
	return p, nil
}
func (p *AIHistoricalInputPair) ValidateFrozen(ctx context.Context) error {
	if p == nil || p.self != p {
		return ErrAIReverseBinding
	}
	_, err := CompareIndependentAIHistoricalInputs(ctx, p.first, p.second, p.sources)
	return err
}
func (p *AIHistoricalInputPair) Summary() AIHistoricalInputSummary {
	if p == nil || p.self != p {
		return (*AIHistoricalInputEpoch)(nil).Summary()
	}
	r := p.second.Summary()
	r.TwoIndependentInputsMatched = r.CompleteInput
	return r
}

// Built only from the same actual decoded full read that wrote the raw pages.
// This index does not retain the old snapshot, coordinator or source authority.
func (e *AIHistoricalInputEpoch) freezeComponentIndex(s *AIReverseSnapshot) error {
	if e == nil || s == nil || len(s.metadata) != len(aiReverseSpecs) || s.inputPage != len(e.pages) {
		return ErrAIReverseBinding
	}
	e.componentRequests, e.componentPages = map[string][]string{}, map[string][]int{}
	e.metadata = make([]aiReverseMetadata, len(s.metadata))
	for i, m := range s.metadata {
		e.metadata[i] = m
		e.metadata[i].columns = append([]string(nil), m.columns...)
		e.metadata[i].sourceColumns = make(SQLColumns, len(m.sourceColumns))
		for j, row := range m.sourceColumns {
			for _, v := range row {
				if v == nil {
					e.metadata[i].sourceColumns[j] = append(e.metadata[i].sourceColumns[j], nil)
				} else {
					x := *v
					e.metadata[i].sourceColumns[j] = append(e.metadata[i].sourceColumns[j], &x)
				}
			}
		}
	}
	requests := s.byTable["ai_bridge_requests"]
	addRequest := func(key, id string) { e.componentRequests[key] = append(e.componentRequests[key], id) }
	for id, r := range requests {
		for _, a := range r.assessments {
			addRequest("assessment:"+a, id)
			if anchor := s.anchors[a]; anchor.sheet != "" {
				addRequest("sheet:"+anchor.sheet, id)
			}
		}
	}
	for _, n := range s.nodes {
		if n.inputPage < 0 || n.inputPage >= len(e.pages) {
			return ErrAIReverseBinding
		}
		e.componentPages["id:"+n.id] = append(e.componentPages["id:"+n.id], n.inputPage)
		req := n.request
		if req == "" && requests[n.aggregate] != nil {
			req = n.aggregate
		}
		if req != "" {
			e.componentPages["request:"+req] = append(e.componentPages["request:"+req], n.inputPage)
		}
	}
	var cost uint64
	for key, ids := range e.componentRequests {
		sort.Strings(ids)
		out := ids[:0]
		for _, id := range ids {
			if len(out) == 0 || out[len(out)-1] != id {
				out = append(out, id)
			}
		}
		e.componentRequests[key] = out
		cost += uint64(128 + len(key))
		for _, id := range out {
			cost += uint64(32 + len(id))
		}
	}
	for key, pages := range e.componentPages {
		sort.Ints(pages)
		out := pages[:0]
		for _, page := range pages {
			if len(out) == 0 || out[len(out)-1] != page {
				out = append(out, page)
			}
		}
		e.componentPages[key] = out
		cost += uint64(128 + len(key) + 16*len(out))
	}
	if s.report.RetainedBudgetBytes > e.limits.MaxRetainedBytes || cost > e.limits.MaxRetainedBytes-s.report.RetainedBudgetBytes {
		return ErrAIReverseBounds
	}
	e.componentIndexSHA = e.componentIndexDigest()
	if e.componentIndexSHA == "" {
		return ErrAIReverseBinding
	}
	return nil
}
func (e *AIHistoricalInputEpoch) componentIndexDigest() string {
	if e == nil || len(e.metadata) != len(aiReverseSpecs) {
		return ""
	}
	parts := []string{"historical-ai-component-input-index/v1", e.epoch, e.report.DataSHA256}
	for _, m := range e.metadata {
		raw, err := json.Marshal(m.sourceColumns)
		if err != nil {
			return ""
		}
		parts = append(parts, m.schema, m.pk, string(raw), stringsJoinAIColumns(m.columns))
	}
	for _, key := range aiReverseSortedKeys(e.componentRequests) {
		parts = append(parts, key)
		parts = append(parts, e.componentRequests[key]...)
	}
	for _, key := range aiReverseSortedKeys(e.componentPages) {
		parts = append(parts, key)
		for _, i := range e.componentPages[key] {
			if i < 0 || i >= len(e.pages) {
				return ""
			}
			ref := e.pages[i]
			parts = append(parts, strconv.Itoa(i), strconv.FormatInt(ref.Offset, 10), strconv.FormatInt(ref.Length, 10), ref.SHA256)
		}
	}
	return aiReverseHash(parts...)
}
func stringsJoinAIColumns(columns []string) string {
	raw, _ := json.Marshal(columns)
	return string(raw)
}
