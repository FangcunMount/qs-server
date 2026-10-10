package evaluation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"syscall"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

var ErrSQLHistoricalCASSpool = errors.New("sql_historical_cas_spool_rejected")

// The host supplies a NEW private regular 0600 file and owns its lifecycle.
// No file, report or serialized frame can manufacture the private tickets below.
// This stores bounded complete raw baselines on short-lived disk. It grants no
// source/closure/production authority and never starts or ends a transaction.
type SQLHistoricalCASSpool struct {
	mu                      sync.Mutex
	self                    *SQLHistoricalCASSpool
	file                    *os.File
	dev, ino                uint64
	end, maxBytes, frameMax int64
	sealed, poisoned        bool
	frames                  []*SQLHistoricalCASSpoolTicket
	originals               map[string]string
	deltas                  map[string]sqlSpoolDelta
	oldPool, writePool      gorm.ConnPool
	writeTransaction        sqlResponsibilityTransaction
	applied                 int
	finalSeal               string
}
type SQLHistoricalCASSpoolTicket struct {
	self                   *SQLHistoricalCASSpoolTicket
	owner                  *SQLHistoricalCASSpool
	record                 sqlSpoolRef
	identity, originalSeal string
	readPool               gorm.ConnPool
	applied                bool
}
type SQLHistoricalCASSpoolStatement struct {
	self  *SQLHistoricalCASSpoolStatement
	owner *SQLHistoricalCASSpool
	count int
	seal  string
}
type SQLHistoricalCASSpoolReadback struct {
	ExpectedRowsSHA256                                     string
	IndependentRawReadbackMatched                          bool
	HostCommitVerified, WholeRetirementComplete, DropReady bool
}
type sqlSpoolRef struct {
	Offset, Length int64
	SHA256         string
}
type sqlSpoolDelta struct {
	OriginalSHA string
	Expected    sqlSpoolRef
	Table       string
	ID          uint64
}

// Gob is a PRIVATE byte-preserving storage codec, never a public import API.
// In particular string values can contain non-UTF8 CAST AS BINARY bytes.
type sqlSpoolCell struct {
	Null  bool
	Value []byte
}
type sqlSpoolRows struct {
	Nil    bool
	Values []map[string]sqlSpoolCell
}
type sqlSpoolImage struct {
	Rows    map[string]sqlSpoolRows
	Schema  map[string]string
	Columns map[string][]string
}
type sqlSpoolGroup struct {
	Table, Column string
	ID            uint64
	Entries       []evidence.HistoricalReferenceEntryV1
	Set           *evidence.HistoricalReferenceSetV1
}
type sqlSpoolPlan struct {
	Version                                  int
	Identity, Server, Database, OriginalSeal string
	Old                                      [3]uint64
	Request                                  SQLHistoricalOwnerBatchRequest
	Limits                                   SQLHistoricalOwnerBatchLimits
	Before                                   sqlSpoolImage
	Groups                                   []sqlSpoolGroup
	Attachments                              []SQLHistoricalBatchAttachment
	Missing                                  []string
	ReadOnly                                 bool
}

// SQLHistoricalCASFrozenInput retains original physical inputs only. It has no
// transaction, Apply method, expiry renewal, or serialized import constructor.
type SQLHistoricalCASFrozenInput struct {
	self   *SQLHistoricalCASFrozenInput
	before sqlHistoricalCASImage
	writes map[string]bool
	seal   string
}

func (*SQLHistoricalCASFrozenInput) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASFrozenInput) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASFrozenInput) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASFrozenInput) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASFrozenInput) String() string {
	return "private frozen SQL inputs; no CAS authority"
}
func (f *SQLHistoricalCASFrozenInput) digest() string {
	if f == nil {
		return ""
	}
	keys := make([]string, 0, len(f.writes))
	for k, write := range f.writes {
		if !write {
			return ""
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return cycleKeyDigest(append([]string{"sql-cas-frozen-input/v1", casImageHash(f.before)}, keys...))
}

// Freeze is called while the actual original owner page is alive, before a
// whole source EOF if necessary. The resulting input cannot qualify a write.
func FreezeSQLHistoricalCASInput(ctx context.Context, original *SQLHistoricalOwnerBatch, p *SQLHistoricalCASProvenance, b *SQLHistoricalCASReadBaseline) (*SQLHistoricalCASFrozenInput, error) {
	if ctx == nil || ctx.Err() != nil || original == nil || original.cycle == nil || original.ValidateBorrowedSnapshot(ctx) != nil || (p == nil) == (b == nil) {
		return nil, ErrSQLHistoricalCASSpool
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil || actual != original.cycle.transaction {
		return nil, ErrSQLHistoricalCASSpool
	}
	f := &SQLHistoricalCASFrozenInput{writes: map[string]bool{}}
	if p != nil {
		if !p.intact() || p.readPool != tx.Statement.ConnPool || p.plan.oldTransaction != actual || p.plan.identity != original.report.DatabaseIdentitySHA256 || !reflect.DeepEqual(p.plan.request, original.request) {
			return nil, ErrSQLHistoricalCASSpool
		}
		f.before = casCloneImage(p.plan.before)
		for _, g := range p.plan.groups {
			f.writes[sqlSpoolKey(g.table, g.id)] = true
		}
	} else {
		if !b.intact() || b.readPool != tx.Statement.ConnPool || b.identity != original.report.DatabaseIdentitySHA256 || !reflect.DeepEqual(b.request, original.request) {
			return nil, ErrSQLHistoricalCASSpool
		}
		f.before = casCloneImage(b.before)
	}
	observed := casCloneImage(f.before)
	delete(observed.rows, "cas_migration_head")
	if !reflect.DeepEqual(observed, sqlHistoricalCASImage{rows: original.rows, schema: original.schema, columns: original.columns}) {
		return nil, ErrSQLHistoricalCASSpool
	}
	f.self, f.seal = f, f.digest()
	if err = f.RowDependencies(func(string, uint64, string, uint64, bool) error { return nil }); err != nil {
		return nil, err
	}
	if original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalCASSpool
	}
	return f, nil
}

// RowDependencies exposes only exact captured business row identities/hashes.
// Schema/head/model reads never become write edges. No caller supplied map is
// accepted, and missing or ambiguous physical write targets reject the input.
func (f *SQLHistoricalCASFrozenInput) RowDependencies(visit func(table string, id uint64, rawSHA string, rawBytes uint64, write bool) error) error {
	if f == nil || f.self != f || f.seal == "" || f.seal != f.digest() || visit == nil {
		return ErrSQLHistoricalCASSpool
	}
	seen := map[string]bool{}
	for _, table := range batchBusinessTables {
		for _, row := range f.before.rows[table] {
			id, err := sqlHistoricalUint(row, "id")
			key := sqlSpoolKey(table, id)
			if err != nil || id == 0 || seen[key] {
				return ErrSQLHistoricalCASSpool
			}
			seen[key] = true
			// The existing cell codec preserves NULL and original binary bytes;
			// gob cannot encode a nil pointer directly in historicalSQLRow.
			storage := sqlSpoolImageOut(sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{table: {row}}}).Rows[table]
			raw, err := sqlSpoolEncode(storage)
			if err != nil {
				return err
			}
			if err = visit(table, id, sqlSpoolRowSHA(row), uint64(len(raw)), f.writes[key]); err != nil {
				return err
			}
		}
	}
	for key := range f.writes {
		if !seen[key] {
			return ErrSQLHistoricalCASSpool
		}
	}
	if f.seal != f.digest() {
		return ErrSQLHistoricalCASSpool
	}
	return nil
}

func (*SQLHistoricalCASSpool) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolTicket) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolStatement) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (s *SQLHistoricalCASSpool) String() string {
	return "private prepared SQL disk spool; no retirement authority"
}
func sqlSpoolSHA(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func sqlSpoolEncode(v any) ([]byte, error) {
	var b bytes.Buffer
	if gob.NewEncoder(&b).Encode(v) != nil {
		return nil, ErrSQLHistoricalCASSpool
	}
	return b.Bytes(), nil
}
func sqlSpoolDecode(raw []byte, v any) error {
	d := gob.NewDecoder(bytes.NewReader(raw))
	if d.Decode(v) != nil {
		return ErrSQLHistoricalCASSpool
	}
	var extra any
	if !errors.Is(d.Decode(&extra), io.EOF) {
		return ErrSQLHistoricalCASSpool
	}
	return nil
}
func sqlSpoolImageOut(v sqlHistoricalCASImage) sqlSpoolImage {
	out := sqlSpoolImage{Rows: map[string]sqlSpoolRows{}, Schema: v.schema, Columns: v.columns}
	for k, rows := range v.rows {
		data := sqlSpoolRows{Nil: rows == nil, Values: make([]map[string]sqlSpoolCell, len(rows))}
		for i, row := range rows {
			data.Values[i] = map[string]sqlSpoolCell{}
			for column, value := range row {
				cell := sqlSpoolCell{Null: value == nil}
				if value != nil {
					cell.Value = []byte(*value)
				}
				data.Values[i][column] = cell
			}
		}
		out.Rows[k] = data
	}
	return out
}
func sqlSpoolImageIn(v sqlSpoolImage) sqlHistoricalCASImage {
	out := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: v.Schema, columns: v.Columns}
	for k, data := range v.Rows {
		if data.Nil {
			out.rows[k] = nil
			continue
		}
		rows := make([]historicalSQLRow, len(data.Values))
		for i, row := range data.Values {
			rows[i] = historicalSQLRow{}
			for column, cell := range row {
				if cell.Null {
					rows[i][column] = nil
				} else {
					value := string(cell.Value)
					rows[i][column] = &value
				}
			}
		}
		out.rows[k] = rows
	}
	return out
}
func sqlSpoolRowSHA(r historicalSQLRow) string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{"sql-spool-full-raw-row/v1"}
	for _, k := range keys {
		parts = append(parts, k)
		if r[k] == nil {
			parts = append(parts, "NULL")
		} else {
			parts = append(parts, "VALUE", *r[k])
		}
	}
	return cycleKeyDigest(parts)
}
func sqlSpoolKey(table string, id uint64) string { return table + ":" + strconv.FormatUint(id, 10) }
func NewSQLHistoricalCASSpool(file *os.File, maxBytes, frameMax int64) (*SQLHistoricalCASSpool, error) {
	if file == nil || maxBytes <= 0 || maxBytes > 64<<30 || frameMax <= 0 || frameMax > 512<<20 || frameMax > maxBytes {
		return nil, ErrSQLHistoricalCASSpool
	}
	info, err := file.Stat()
	if err != nil {
		return nil, ErrSQLHistoricalCASSpool
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() != 0 {
		return nil, ErrSQLHistoricalCASSpool
	}
	s := &SQLHistoricalCASSpool{file: file, dev: uint64(st.Dev), ino: uint64(st.Ino), maxBytes: maxBytes, frameMax: frameMax, originals: map[string]string{}, deltas: map[string]sqlSpoolDelta{}}
	s.self = s
	return s, nil
}
func (s *SQLHistoricalCASSpool) valid(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || s == nil || s.self != s || s.poisoned || s.file == nil {
		return ErrSQLHistoricalCASSpool
	}
	info, err := s.file.Stat()
	if err != nil {
		return ErrSQLHistoricalCASSpool
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || uint64(st.Dev) != s.dev || uint64(st.Ino) != s.ino || info.Size() != s.end {
		return ErrSQLHistoricalCASSpool
	}
	return nil
}
func (s *SQLHistoricalCASSpool) put(ctx context.Context, v any) (sqlSpoolRef, error) {
	if s.valid(ctx) != nil {
		return sqlSpoolRef{}, ErrSQLHistoricalCASSpool
	}
	raw, err := sqlSpoolEncode(v)
	if err != nil || int64(len(raw)) > s.frameMax || int64(len(raw)) > s.maxBytes-s.end {
		return sqlSpoolRef{}, ErrSQLHistoricalCASSpool
	}
	ref := sqlSpoolRef{s.end, int64(len(raw)), sqlSpoolSHA(raw)}
	n, err := s.file.WriteAt(raw, s.end)
	if err != nil || n != len(raw) || s.file.Sync() != nil {
		s.poisoned = true
		return sqlSpoolRef{}, ErrSQLHistoricalCASSpool
	}
	s.end += int64(n)
	if s.valid(ctx) != nil {
		s.poisoned = true
		return sqlSpoolRef{}, ErrSQLHistoricalCASSpool
	}
	return ref, nil
}
func (s *SQLHistoricalCASSpool) get(ctx context.Context, ref sqlSpoolRef, v any) error {
	if s.valid(ctx) != nil || ref.Offset < 0 || ref.Length <= 0 || ref.Length > s.frameMax || ref.Offset > s.end-ref.Length {
		return ErrSQLHistoricalCASSpool
	}
	raw := make([]byte, ref.Length)
	n, err := s.file.ReadAt(raw, ref.Offset)
	if err != nil || n != len(raw) || sqlSpoolSHA(raw) != ref.SHA256 {
		return ErrSQLHistoricalCASSpool
	}
	return sqlSpoolDecode(raw, v)
}

// Append accepts only the actual existing provenance/read-baseline producer in
// its STILL ACTIVE original RRRO. Its stored old pool is never reopened.
func (s *SQLHistoricalCASSpool) Append(ctx context.Context, p *SQLHistoricalCASProvenance, b *SQLHistoricalCASReadBaseline) (ticket *SQLHistoricalCASSpoolTicket, result error) {
	if s == nil {
		return nil, ErrSQLHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if result != nil {
			s.poisoned = true
		}
	}()
	if s.valid(ctx) != nil || s.sealed || (p == nil) == (b == nil) || len(s.frames) >= 2_000_000 {
		return nil, ErrSQLHistoricalCASSpool
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil {
		return nil, err
	}
	record := sqlSpoolPlan{Version: 1}
	pool := tx.Statement.ConnPool
	if p != nil {
		if !p.intact() || p.readPool != pool || p.plan.oldTransaction != actual {
			return nil, ErrSQLHistoricalCASSpool
		}
		v := p.plan
		record.Identity, record.Server, record.Database, record.OriginalSeal = p.plan.identity, v.server, v.database, p.seal
		record.Old = [3]uint64{actual.connection, actual.thread, actual.event}
		record.Request, record.Limits, record.Before = v.request, v.limits, sqlSpoolImageOut(v.before)
		record.Attachments, record.Missing = v.attachments, v.missingOriginalRunIDs
		for _, g := range v.groups {
			record.Groups = append(record.Groups, sqlSpoolGroup{g.table, g.column, g.id, g.entries, g.set})
		}
	} else {
		if !b.intact() || b.readPool != pool {
			return nil, ErrSQLHistoricalCASSpool
		}
		server, database, e := historicalDatabase(tx)
		if e != nil {
			return nil, e
		}
		record.Identity, record.Server, record.Database, record.OriginalSeal = b.identity, server, database, b.seal
		record.Old = [3]uint64{actual.connection, actual.thread, actual.event}
		record.Request, record.Before, record.ReadOnly = b.request, sqlSpoolImageOut(b.before), true
		record.Limits = DefaultSQLHistoricalOwnerBatchLimits()
	}
	if s.oldPool != nil && s.oldPool != pool {
		return nil, ErrSQLHistoricalCASSpool
	}
	image := sqlSpoolImageIn(record.Before)
	if record.ReadOnly {
		// The old scoped batch is not global original-Run absence authority. Derive
		// IDs only from actual Outcome rows, then really re-read every scope/owner.
		present := map[string]bool{}
		for _, row := range image.rows["runtime_checkpoint"] {
			present[valueOrEmpty(row["resource_id"])] = true
		}
		absent := map[string]bool{}
		for _, row := range image.rows["evaluation_outcome"] {
			id := valueOrEmpty(row["evaluation_run_id"])
			if id == "" {
				return nil, ErrSQLHistoricalBatchConflict
			}
			if !present[id] {
				absent[id] = true
			}
		}
		for id := range absent {
			record.Missing = append(record.Missing, id)
		}
		sort.Strings(record.Missing)
		if len(record.Missing) > 0 && casMissingOriginalRuns(tx, record.Missing, false) != nil {
			return nil, ErrSQLHistoricalBatchConflict
		}
	}
	for _, table := range batchBusinessTables {
		for _, row := range image.rows[table] {
			id, e := sqlHistoricalUint(row, "id")
			if e != nil {
				return nil, e
			}
			key := sqlSpoolKey(table, id)
			hash := sqlSpoolRowSHA(row)
			if len(s.originals) >= 2_000_000 && s.originals[key] == "" {
				return nil, ErrSQLHistoricalCASSpool
			}
			if old, ok := s.originals[key]; ok && old != hash {
				return nil, ErrSQLHistoricalBatchConflict
			}
			s.originals[key] = hash
		}
	}
	ref, err := s.put(ctx, record)
	if err != nil {
		return nil, err
	}
	again, err := cycleActualTransaction(tx)
	if err != nil || again != actual {
		return nil, ErrSQLHistoricalCASSpool
	}
	ticket = &SQLHistoricalCASSpoolTicket{owner: s, record: ref, identity: record.Identity, originalSeal: record.OriginalSeal, readPool: pool}
	ticket.self = ticket
	s.frames = append(s.frames, ticket)
	s.oldPool = pool
	return ticket, nil
}
func (s *SQLHistoricalCASSpool) Seal(ctx context.Context) error {
	if s == nil {
		return ErrSQLHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || s.sealed || len(s.frames) == 0 {
		return ErrSQLHistoricalCASSpool
	}
	s.sealed = true
	return nil
}
func (s *SQLHistoricalCASSpool) load(ctx context.Context, t *SQLHistoricalCASSpoolTicket) (sqlSpoolPlan, error) {
	var r sqlSpoolPlan
	if t == nil || t.self != t || t.owner != s || t.readPool != s.oldPool || s.get(ctx, t.record, &r) != nil || r.Version != 1 || r.Identity != t.identity || r.OriginalSeal != t.originalSeal || len(r.Attachments) > 512 || len(r.Groups) > 512 {
		return r, ErrSQLHistoricalCASSpool
	}
	return r, nil
}
func (s *SQLHistoricalCASSpool) imageWithActualPredecessors(ctx context.Context, original sqlHistoricalCASImage) (sqlHistoricalCASImage, error) {
	v := casCloneImage(original)
	type position struct {
		table string
		id    uint64
		index int
	}
	refs := map[sqlSpoolRef][]position{}
	for _, table := range batchBusinessTables {
		for i, row := range v.rows[table] {
			id, e := sqlHistoricalUint(row, "id")
			if e != nil {
				return v, e
			}
			key := sqlSpoolKey(table, id)
			hash := sqlSpoolRowSHA(row)
			if s.originals[key] != hash {
				return v, ErrSQLHistoricalBatchConflict
			}
			if d, ok := s.deltas[key]; ok {
				if d.OriginalSHA != hash {
					return v, ErrSQLHistoricalBatchConflict
				}
				refs[d.Expected] = append(refs[d.Expected], position{table, id, i})
			}
		}
	}
	// Each predecessor frame is decoded once and released before the next one;
	// never retain a cache of hundreds of complete historical owner images.
	for ref, positions := range refs {
		var disk sqlSpoolImage
		if s.get(ctx, ref, &disk) != nil {
			return v, ErrSQLHistoricalCASSpool
		}
		image := sqlSpoolImageIn(disk)
		for _, p := range positions {
			next, e := casRow(image, p.table, p.id)
			if e != nil {
				return v, e
			}
			v.rows[p.table][p.index] = next
		}
	}
	return v, nil
}
func sqlSpoolReconstruct(r sqlSpoolPlan, before sqlHistoricalCASImage) (*SQLHistoricalBatchCASPlan, error) {
	p := &SQLHistoricalBatchCASPlan{oldTransaction: sqlResponsibilityTransaction{r.Old[0], r.Old[1], r.Old[2]}, identity: r.Identity, server: r.Server, database: r.Database, request: r.Request, limits: r.Limits, before: before, attachments: r.Attachments, missingOriginalRunIDs: r.Missing}
	for _, g := range r.Groups {
		row, e := casRow(before, g.Table, g.ID)
		if e != nil {
			return nil, e
		}
		stored, e := historicalDecode(row[g.Column])
		if e != nil {
			return nil, e
		}
		for _, entry := range g.Entries {
			stored, e = stored.Append(entry)
			if e != nil {
				return nil, e
			}
		}
		p.groups = append(p.groups, sqlHistoricalCASGroup{g.Table, g.Column, g.ID, g.Entries, stored})
	}
	return p, nil
}

// ApplyTicket borrows ONE actual new RW Tx for the entire spool. Only actual
// successful earlier statement expected rows can become an evidence predecessor.
// Any error poisons this instance: host rollback is required; no automatic retry.
func (s *SQLHistoricalCASSpool) ApplyTicket(ctx context.Context, t *SQLHistoricalCASSpoolTicket) (result error) {
	if s == nil {
		return ErrSQLHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if result != nil {
			s.poisoned = true
		}
	}()
	if s.valid(ctx) != nil || !s.sealed || s.applied >= len(s.frames) || s.frames[s.applied] != t || t.applied || sqlHistoricalEndedPool(ctx, s.oldPool) != nil {
		return ErrSQLHistoricalCASSpool
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	actual, err := casActualRW(tx)
	if err != nil {
		return err
	}
	if tx.Statement.ConnPool == s.oldPool {
		return ErrSQLHistoricalCASSpool
	}
	if s.writePool != nil && (tx.Statement.ConnPool != s.writePool || actual != s.writeTransaction) {
		return ErrSQLHistoricalCASSpool
	}
	r, err := s.load(ctx, t)
	if err != nil {
		return err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || server != r.Server || database != r.Database {
		return ErrSQLHistoricalFactsIdentity
	}
	before, err := s.imageWithActualPredecessors(ctx, sqlSpoolImageIn(r.Before))
	if err != nil {
		return err
	}
	p, err := sqlSpoolReconstruct(r, before)
	if err != nil {
		return err
	}
	var expected sqlHistoricalCASImage
	if r.ReadOnly {
		expected, err = p.capture(tx, true)
		if err != nil || !reflect.DeepEqual(expected, before) {
			return ErrSQLHistoricalBatchConflict
		}
		if len(r.Missing) > 0 && casMissingOriginalRuns(tx, r.Missing, true) != nil {
			return ErrSQLHistoricalBatchConflict
		}
	} else {
		statement, e := p.Apply(ctx)
		if e != nil {
			return e
		}
		if statement.plan != p || statement.transaction != actual {
			return ErrSQLHistoricalCASSpool
		}
		expected = statement.expected
	}
	ref, err := s.put(ctx, sqlSpoolImageOut(expected))
	if err != nil {
		return err
	}
	// Only dedicated evidence targets become deltas. This never permits changes
	// to ordinary business columns, standard slots, NULL, schema or timestamp.
	for _, g := range p.groups {
		old, e := casRow(sqlSpoolImageIn(r.Before), g.table, g.id)
		if e != nil {
			return e
		}
		next, e := casRow(expected, g.table, g.id)
		if e != nil {
			return e
		}
		check := historicalSQLRow{}
		for k, v := range next {
			check[k] = v
		}
		check[g.column] = old[g.column]
		if !reflect.DeepEqual(check, old) {
			return ErrSQLHistoricalBatchConflict
		}
		key := sqlSpoolKey(g.table, g.id)
		s.deltas[key] = sqlSpoolDelta{sqlSpoolRowSHA(old), ref, g.table, g.id}
	}
	s.writePool, s.writeTransaction = tx.Statement.ConnPool, actual
	t.applied = true
	s.applied++
	return nil
}
func (s *SQLHistoricalCASSpool) FinishApply(ctx context.Context) (*SQLHistoricalCASSpoolStatement, error) {
	if s == nil {
		return nil, ErrSQLHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || !s.sealed || s.applied != len(s.frames) || s.writePool == nil || s.finalSeal != "" {
		return nil, ErrSQLHistoricalCASSpool
	}
	tx, e := historicalTx(ctx)
	if e != nil || tx.Statement.ConnPool != s.writePool {
		return nil, ErrSQLHistoricalCASSpool
	}
	actual, e := casActualRW(tx)
	if e != nil || actual != s.writeTransaction {
		return nil, ErrSQLHistoricalCASSpool
	}
	s.finalSeal = s.digest()
	statement := &SQLHistoricalCASSpoolStatement{owner: s, count: s.applied, seal: s.finalSeal}
	statement.self = statement
	return statement, nil
}
func (s *SQLHistoricalCASSpool) digest() string {
	parts := []string{"sql-prepared-spool-actual-statement/v1", strconv.FormatInt(s.end, 10), strconv.Itoa(s.applied), sqlHistoricalProvenancePoolToken(s.oldPool), sqlHistoricalProvenancePoolToken(s.writePool)}
	for _, t := range s.frames {
		parts = append(parts, t.record.SHA256, t.originalSeal)
	}
	keys := make([]string, 0, len(s.deltas))
	for k := range s.deltas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := s.deltas[k]
		parts = append(parts, k, d.OriginalSHA, d.Expected.SHA256)
	}
	return cycleKeyDigest(parts)
}
func (s *SQLHistoricalCASSpool) VerifyTicket(ctx context.Context, statement *SQLHistoricalCASSpoolStatement, t *SQLHistoricalCASSpoolTicket, fresh *SQLHistoricalOwnerBatch) (SQLHistoricalCASSpoolReadback, error) {
	var report SQLHistoricalCASSpoolReadback
	if s == nil {
		return report, ErrSQLHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || statement == nil || statement.self != statement || statement.owner != s || statement.count != len(s.frames) || s.finalSeal == "" || statement.seal != s.finalSeal || s.applied != len(s.frames) || fresh == nil || fresh.ValidateBorrowedSnapshot(ctx) != nil || sqlHistoricalEndedPool(ctx, s.oldPool) != nil || sqlHistoricalEndedPool(ctx, s.writePool) != nil {
		return report, ErrSQLHistoricalCASSpool
	}
	tx, e := historicalTx(ctx)
	if e != nil || tx.Statement.ConnPool == s.oldPool || tx.Statement.ConnPool == s.writePool {
		return report, ErrSQLHistoricalCASSpool
	}
	r, e := s.load(ctx, t)
	if e != nil || !t.applied || fresh.report.DatabaseIdentitySHA256 != r.Identity || !reflect.DeepEqual(fresh.request, r.Request) {
		return report, ErrSQLHistoricalCASSpool
	}
	expected, e := s.imageWithActualPredecessors(ctx, sqlSpoolImageIn(r.Before))
	if e != nil {
		return report, e
	}
	head, _, _, e := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if e != nil {
		return report, e
	}
	observed := casCloneImage(sqlHistoricalCASImage{rows: fresh.rows, schema: fresh.schema, columns: fresh.columns})
	observed.rows["cas_migration_head"] = head
	if len(r.Missing) > 0 && casMissingOriginalRuns(tx, r.Missing, false) != nil {
		return report, ErrSQLHistoricalBatchConflict
	}
	if !reflect.DeepEqual(expected, observed) || fresh.ValidateBorrowedSnapshot(ctx) != nil {
		return report, ErrSQLHistoricalBatchConflict
	}
	report.ExpectedRowsSHA256, report.IndependentRawReadbackMatched = casImageHash(expected), true
	return report, nil
}

// Defensive selector access for the real higher-level spool producer. The DTO
// cannot create a ticket, provenance, statement or closure capability.
func SQLHistoricalBatchSpoolRequest(b *SQLHistoricalOwnerBatch) SQLHistoricalOwnerBatchRequest {
	if b == nil {
		return SQLHistoricalOwnerBatchRequest{}
	}
	return SQLHistoricalOwnerBatchRequest{AssessmentIDs: append([]uint64(nil), b.request.AssessmentIDs...), AnswerSheetIDs: append([]uint64(nil), b.request.AnswerSheetIDs...)}
}

func (*SQLHistoricalCASSpool) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpool) UnmarshalJSON([]byte) error { return ErrSQLHistoricalFactsSerialization }
func (*SQLHistoricalCASSpool) UnmarshalBSON([]byte) error { return ErrSQLHistoricalFactsSerialization }
func (*SQLHistoricalCASSpoolTicket) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolTicket) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolTicket) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolStatement) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolStatement) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASSpoolStatement) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (s *SQLHistoricalCASSpool) GoString() string { return s.String() }
func (*SQLHistoricalCASSpoolTicket) String() string {
	return "private SQL disk ticket; no import or authority"
}
func (t *SQLHistoricalCASSpoolTicket) GoString() string { return t.String() }
func (*SQLHistoricalCASSpoolStatement) String() string {
	return "private SQL spool actual statement; host commit unproven"
}
func (t *SQLHistoricalCASSpoolStatement) GoString() string { return t.String() }
