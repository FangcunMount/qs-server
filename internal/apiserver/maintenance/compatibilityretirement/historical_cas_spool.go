package retirement

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
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

const ErrHistoricalCASSpool SourceError = "historical_cas_prepared_spool_rejected"

// An in-process opaque disk holder is made ONLY from real qualified/sealed
// pages. A file/DTO cannot be imported into it, including after a process crash.
// The host owns both private files and all borrowed database lifecycles.
type HistoricalCASSpool struct {
	mu                                 sync.Mutex
	self                               *HistoricalCASSpool
	file                               *os.File
	dev, ino                           uint64
	end, maxBytes, frameMax            int64
	poisoned, sealed                   bool
	binding                            HistoricalCoordinatorBinding
	copies                             [4]SourceCopyReceipt
	encoded                            [4]string
	indexSHA                           string
	oldSQLPool, writeSQLPool           gorm.ConnPool
	oldMongoSession, writeMongoSession mongo.Session
	oldMongoTxn, writeMongoTxn         mongoCycleTxn
	db                                 *mongo.Database
	expires                            time.Time
	sql                                *sqlevaluation.SQLHistoricalCASSpool
	sqlStatement                       *sqlevaluation.SQLHistoricalCASSpoolStatement
	frames                             []*HistoricalCASSpoolTicket
	seen                               map[verifiedSourceKey]bool
	originalMongo                      map[string]string
	mongoDeltas                        map[string]historicalSpoolMongoDelta
	applied                            int
	finalSeal                          string
	freshSQLPool                       gorm.ConnPool
	freshMongoTxn                      mongoCycleTxn
	verified                           map[int]bool
}
type HistoricalCASSpoolTicket struct {
	self     *HistoricalCASSpoolTicket
	owner    *HistoricalCASSpool
	position int
	record   historicalSpoolRef
	sql      *sqlevaluation.SQLHistoricalCASSpoolTicket
	entries  uint64
	applied  bool
}
type HistoricalCASSpoolApplied struct {
	self  *HistoricalCASSpoolApplied
	owner *HistoricalCASSpool
	seal  string
}
type HistoricalCASSpoolPageObservation struct {
	Protocol, SourceSHA, OperationID, PageSHA256, SQLFinalRowsSHA256, MongoFinalRowsSHA256                              string
	References                                                                                                          uint64
	IndependentRawReadbackMatched                                                                                       bool
	HostCommitVerified, WholeRetirementComplete, AICommandsPersisted, BusinessClosureVerified, CASAuthorized, DropReady bool
}
type historicalSpoolRef struct {
	Offset, Length int64
	SHA256         string
}
type historicalSpoolRawRows struct {
	Nil  bool
	Rows []bson.Raw
}
type historicalSpoolRawImage map[string]historicalSpoolRawRows
type historicalSpoolMongoDelta struct {
	OriginalSHA string
	Record      historicalSpoolRef
	Collection  string
	ID          uint64
}
type historicalSpoolGroup struct {
	Collection, Slot string
	ID               uint64
	PK               bson.RawValue
	Entries          []evidence.HistoricalReferenceEntryV1
	Set              *evidence.HistoricalReferenceSetV1
}
type historicalSpoolSource struct {
	ID       string
	Object   uint8
	PK       [32]byte
	FactsSHA [32]byte
}
type historicalSpoolFrame struct {
	Version                                                  int
	PageSHA, OriginSHA, AISHA                                string
	Sequence, Entries                                        uint64
	Sources                                                  []historicalSpoolSource
	SQLRequest                                               sqlevaluation.SQLHistoricalOwnerBatchRequest
	MongoReadOnly                                            bool
	Config                                                   MongoOwnerConfig
	Selection                                                [5]map[uint64]bool
	Limits                                                   MongoHistoricalOwnerBatchLimits
	Hints                                                    map[string]string
	Before                                                   historicalSpoolRawImage
	Groups                                                   []historicalSpoolGroup
	Attachments                                              []mongoCASAttachmentFactsDisk
	Identity, Metadata, OriginalSQLIdentity, OriginalSQLRows string
}
type mongoCASAttachmentFactsDisk struct {
	Collection, Slot string
	ID               uint64
	Entry            evidence.HistoricalReferenceEntryV1
	Content          evidence.Digest
}

func (*HistoricalCASSpool) MarshalJSON() ([]byte, error)        { return nil, ErrSourceSerialization }
func (*HistoricalCASSpoolTicket) MarshalJSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*HistoricalCASSpoolApplied) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASSpool) String() string {
	return "private actual prepared-page spool; production/AI/fence/DROP unqualified"
}
func historicalSpoolSHA(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func historicalSpoolEncode(v any) ([]byte, error) {
	var b bytes.Buffer
	if gob.NewEncoder(&b).Encode(v) != nil {
		return nil, ErrHistoricalCASSpool
	}
	return b.Bytes(), nil
}
func historicalSpoolDecode(raw []byte, v any) error {
	d := gob.NewDecoder(bytes.NewReader(raw))
	if d.Decode(v) != nil {
		return ErrHistoricalCASSpool
	}
	var extra any
	if !errors.Is(d.Decode(&extra), io.EOF) {
		return ErrHistoricalCASSpool
	}
	return nil
}

// Gob collapses empty slices, so physical nil/empty collection baselines are
// encoded explicitly. Ordered BSON bytes are never decoded/reserialized here.
func historicalSpoolRawOut(data map[string][]bson.Raw) historicalSpoolRawImage {
	out := historicalSpoolRawImage{}
	for name, rows := range data {
		v := historicalSpoolRawRows{Nil: rows == nil, Rows: make([]bson.Raw, len(rows))}
		for i, raw := range rows {
			v.Rows[i] = append(bson.Raw(nil), raw...)
		}
		out[name] = v
	}
	return out
}
func historicalSpoolRawIn(data historicalSpoolRawImage) map[string][]bson.Raw {
	out := map[string][]bson.Raw{}
	for name, v := range data {
		if v.Nil {
			out[name] = nil
			continue
		}
		rows := make([]bson.Raw, len(v.Rows))
		for i, raw := range v.Rows {
			rows[i] = append(bson.Raw(nil), raw...)
		}
		out[name] = rows
	}
	return out
}
func historicalSpoolSelectionOut(v mongoBatchSelection) [5]map[uint64]bool {
	return [5]map[uint64]bool{v.sheets, v.outcomes, v.generations, v.artifacts, v.runs}
}
func historicalSpoolSelectionIn(v [5]map[uint64]bool) mongoBatchSelection {
	return mongoBatchSelection{v[0], v[1], v[2], v[3], v[4]}
}

// There is no root/production permission here. This proves only exact private
// temporary file ownership. The caller must open with NOFOLLOW/NONBLOCK/O_EXCL.
func NewHistoricalCASSpool(ctx context.Context, file, sqlFile *os.File, maxBytes, frameMax int64) (*HistoricalCASSpool, error) {
	if ctx == nil || ctx.Err() != nil || file == nil || sqlFile == nil || file == sqlFile || maxBytes <= 0 || maxBytes > 64<<30 || frameMax <= 0 || frameMax > 512<<20 || frameMax > maxBytes {
		return nil, ErrHistoricalCASSpool
	}
	info, err := file.Stat()
	if err != nil {
		return nil, ErrHistoricalCASSpool
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() != 0 {
		return nil, ErrHistoricalCASSpool
	}
	other, e := sqlFile.Stat()
	if e != nil || os.SameFile(info, other) {
		return nil, ErrHistoricalCASSpool
	}
	sqlSpool, err := sqlevaluation.NewSQLHistoricalCASSpool(sqlFile, maxBytes, frameMax)
	if err != nil {
		return nil, err
	}
	s := &HistoricalCASSpool{file: file, dev: uint64(st.Dev), ino: uint64(st.Ino), maxBytes: maxBytes, frameMax: frameMax, sql: sqlSpool, seen: map[verifiedSourceKey]bool{}, originalMongo: map[string]string{}, mongoDeltas: map[string]historicalSpoolMongoDelta{}, verified: map[int]bool{}}
	s.self = s
	return s, nil
}
func (s *HistoricalCASSpool) valid(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || s == nil || s.self != s || s.file == nil || s.poisoned || (!s.expires.IsZero() && !time.Now().Before(s.expires)) {
		return ErrHistoricalCASSpool
	}
	info, e := s.file.Stat()
	if e != nil {
		return ErrHistoricalCASSpool
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || uint64(st.Dev) != s.dev || uint64(st.Ino) != s.ino || info.Size() != s.end {
		return ErrHistoricalCASSpool
	}
	return nil
}
func (s *HistoricalCASSpool) put(ctx context.Context, v any) (historicalSpoolRef, error) {
	if s.valid(ctx) != nil {
		return historicalSpoolRef{}, ErrHistoricalCASSpool
	}
	raw, e := historicalSpoolEncode(v)
	if e != nil || int64(len(raw)) > s.frameMax || int64(len(raw)) > s.maxBytes-s.end {
		return historicalSpoolRef{}, ErrHistoricalCASSpool
	}
	r := historicalSpoolRef{s.end, int64(len(raw)), historicalSpoolSHA(raw)}
	n, e := s.file.WriteAt(raw, s.end)
	if e != nil || n != len(raw) || s.file.Sync() != nil {
		s.poisoned = true
		return historicalSpoolRef{}, ErrHistoricalCASSpool
	}
	s.end += int64(n)
	if s.valid(ctx) != nil {
		s.poisoned = true
		return historicalSpoolRef{}, ErrHistoricalCASSpool
	}
	return r, nil
}
func (s *HistoricalCASSpool) get(ctx context.Context, r historicalSpoolRef, v any) error {
	if s.valid(ctx) != nil || r.Offset < 0 || r.Length <= 0 || r.Length > s.frameMax || r.Offset > s.end-r.Length {
		return ErrHistoricalCASSpool
	}
	raw := make([]byte, r.Length)
	n, e := s.file.ReadAt(raw, r.Offset)
	if e != nil || n != len(raw) || historicalSpoolSHA(raw) != r.SHA256 {
		return ErrHistoricalCASSpool
	}
	return historicalSpoolDecode(raw, v)
}

// Append is the ONLY frame producer. The genuine qualified factory/seal and
// joint's original actual rows remain mandatory; there is no FromJSON method.
func (s *HistoricalCASSpool) Append(ctx context.Context, joint *WholeSourceJointPage, p *HistoricalCASPersistencePage) (ticket *HistoricalCASSpoolTicket, result error) {
	if s == nil {
		return nil, ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if result != nil {
			s.poisoned = true
		}
	}()
	if s.valid(ctx) != nil || s.sealed || joint == nil || joint.owner == nil || joint.sql == nil || joint.mongo == nil || p == nil || p.self != p || p.seal != p.digest() || p.alive(ctx) != nil || joint.ValidateBorrowedSnapshot(ctx) != nil || !joint.consumed || joint.candidateCount <= 0 || joint.candidateCount > 512 || p.entries != uint64(joint.candidateCount) || p.pageSHA != joint.candidateSHA || p.binding != joint.owner.binding || len(s.frames) >= 2_000_000 {
		return nil, ErrHistoricalCASSpool
	}
	c := joint.owner
	c.mu.Lock()
	coverage := c.coverage && !c.failed && c.authenticated != nil && c.authenticated.complete && c.receipts == p.copies
	c.mu.Unlock()
	if !coverage {
		return nil, ErrCoordinatorIncomplete
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx.Statement == nil || tx.Statement.ConnPool != p.oldSQLPool || mongo.SessionFromContext(ctx) != p.oldMongoSession {
		return nil, ErrHistoricalCASSpool
	}
	actual, e := mongoCycleTransaction(ctx, p.db)
	if e != nil || actual.number != p.oldMongoTxn.number || !bytes.Equal(actual.session, p.oldMongoTxn.session) {
		return nil, ErrHistoricalCASSpool
	}
	if len(s.frames) > 0 && (s.indexSHA != joint.index.indexSHA || s.encoded != joint.index.encodedSHA || s.binding != p.binding || s.copies != p.copies || s.oldSQLPool != p.oldSQLPool || s.db != p.db || s.oldMongoSession != p.oldMongoSession || s.oldMongoTxn.number != p.oldMongoTxn.number || !bytes.Equal(s.oldMongoTxn.session, p.oldMongoTxn.session)) {
		return nil, ErrHistoricalCASSpool
	}
	b := joint.mongo
	f := historicalSpoolFrame{Version: 1, PageSHA: p.pageSHA, OriginSHA: p.originSHA, AISHA: p.aiSHA, Sequence: p.sequence, Entries: p.entries, Config: b.global.config, Selection: historicalSpoolSelectionOut(b.selection), Limits: b.limits, Before: historicalSpoolRawOut(b.data), Identity: b.global.metadata.identity, Metadata: b.global.metadata.hash, OriginalSQLIdentity: b.sql.Report().DatabaseIdentitySHA256, OriginalSQLRows: b.sql.Report().BusinessRowsSHA256, SQLRequest: joint.sql.factsRequest(), Hints: map[string]string{}}
	for _, pair := range mongoCASSelections(b.selection) {
		index, err := b.index(pair.name, pair.field, pair.field == "domain_id")
		if err != nil {
			return nil, err
		}
		f.Hints[pair.name+":"+pair.field] = index
	}
	for _, handle := range b.sourceHandles {
		facts, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		hash, he := privateFactsSHA(facts)
		if err != nil || he != nil {
			return nil, ErrSourceAuthentication
		}
		f.Sources = append(f.Sources, historicalSpoolSource{facts.EventID, key.object, key.pk, hash})
	}
	if len(f.Sources) == 0 || len(f.Sources) > 512 {
		return nil, ErrHistoricalCASSpool
	}
	// Coverage uses current, not the related-owner expansion: no double count.
	for _, handle := range joint.current {
		facts, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		if err != nil || s.seen[key] || len(s.seen) >= 2_000_000 {
			return nil, ErrSourceAuthentication
		}
		s.seen[key] = true
	}
	if p.mongo != nil {
		m := p.mongo
		f.Before = historicalSpoolRawOut(m.before)
		f.Hints = m.hints
		f.Limits = m.limits
		for _, g := range m.groups {
			f.Groups = append(f.Groups, historicalSpoolGroup{g.collection, g.slot, g.id, g.pk, g.entries, g.set})
		}
		for _, a := range m.attachments {
			f.Attachments = append(f.Attachments, mongoCASAttachmentFactsDisk{a.collection, a.slot, a.id, a.entry, a.content})
		}
	} else {
		f.MongoReadOnly = true
	}
	beforeRaw := historicalSpoolRawIn(f.Before)
	for _, name := range mongoBatchBusinessCollections {
		for _, raw := range beforeRaw[name] {
			id, e := historicalSpoolMongoID(raw)
			if e != nil {
				return nil, e
			}
			key := mongoCASKey(name, id)
			hash := historicalSpoolSHA(raw)
			if len(s.originalMongo) >= 2_000_000 && s.originalMongo[key] == "" {
				return nil, ErrHistoricalCASSpool
			}
			if old, ok := s.originalMongo[key]; ok && old != hash {
				return nil, ErrMongoBatchConflict
			}
			s.originalMongo[key] = hash
		}
	}
	sqlTicket, e := s.sql.Append(ctx, p.sql, p.unchangedSQL)
	if e != nil {
		return nil, e
	}
	ref, e := s.put(ctx, f)
	if e != nil {
		return nil, e
	}
	if len(s.frames) == 0 {
		s.binding, s.copies, s.oldSQLPool, s.oldMongoSession, s.oldMongoTxn, s.db = p.binding, p.copies, p.oldSQLPool, p.oldMongoSession, p.oldMongoTxn, p.db
		s.indexSHA, s.encoded = joint.index.indexSHA, joint.index.encodedSHA
		s.expires = p.expires
	} else if p.expires.Before(s.expires) {
		s.expires = p.expires
	}
	if s.valid(ctx) != nil || p.alive(ctx) != nil || joint.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrHistoricalCASSpool
	}
	t := &HistoricalCASSpoolTicket{owner: s, position: len(s.frames), record: ref, sql: sqlTicket, entries: p.entries}
	t.self = t
	s.frames = append(s.frames, t)
	return t, nil
}

// factsRequest is read from the actual private SQL batch's Report selectors;
// the public request is merely a selector and never write authorization.
func (b *SQLBusinessOwnerBatch) factsRequest() sqlevaluation.SQLHistoricalOwnerBatchRequest {
	return sqlevaluation.SQLHistoricalBatchSpoolRequest(b.facts)
}
func (s *HistoricalCASSpool) Seal(ctx context.Context, c *HistoricalCoordinator) error {
	if s == nil || c == nil {
		return ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || s.sealed || len(s.frames) == 0 {
		return ErrHistoricalCASSpool
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alive(ctx) != nil || !c.coverage || c.failed || c.receipts != s.copies || c.binding != s.binding || len(c.remaining) != 0 || c.authenticated == nil || !c.authenticated.complete {
		return ErrCoordinatorIncomplete
	}
	var counts [4]uint64
	for key := range s.seen {
		if key.object != 0 && key.object != 3 {
			return ErrSourceAuthentication
		}
		counts[key.object]++
	}
	if counts[0] != s.copies[0].Records || counts[3] != s.copies[3].Records {
		return ErrCoordinatorIncomplete
	}
	if e := s.sql.Seal(ctx); e != nil {
		return e
	}
	s.sealed = true
	return nil
}
func (s *HistoricalCASSpool) load(ctx context.Context, t *HistoricalCASSpoolTicket) (historicalSpoolFrame, error) {
	var f historicalSpoolFrame
	if t == nil || t.self != t || t.owner != s || t.position < 0 || t.position >= len(s.frames) || s.frames[t.position] != t || s.get(ctx, t.record, &f) != nil || f.Version != 1 || f.Entries != t.entries || f.Entries == 0 || f.Entries > 512 || len(f.Sources) == 0 || len(f.Sources) > 512 {
		return f, ErrHistoricalCASSpool
	}
	return f, nil
}
func (s *HistoricalCASSpool) mongoPredecessors(ctx context.Context, data map[string][]bson.Raw) (map[string][]bson.Raw, error) {
	out := mongoCASCloneData(data)
	type position struct {
		name  string
		id    uint64
		index int
	}
	refs := map[historicalSpoolRef][]position{}
	for _, name := range mongoBatchBusinessCollections {
		for i, raw := range out[name] {
			id, e := historicalSpoolMongoID(raw)
			if e != nil {
				return nil, e
			}
			key := mongoCASKey(name, id)
			hash := historicalSpoolSHA(raw)
			if s.originalMongo[key] != hash {
				return nil, ErrMongoBatchConflict
			}
			if d, ok := s.mongoDeltas[key]; ok {
				if d.OriginalSHA != hash {
					return nil, ErrMongoBatchConflict
				}
				refs[d.Record] = append(refs[d.Record], position{name, id, i})
			}
		}
	}
	for ref, positions := range refs {
		var disk historicalSpoolRawImage
		if s.get(ctx, ref, &disk) != nil {
			return nil, ErrHistoricalCASSpool
		}
		image := historicalSpoolRawIn(disk)
		for _, p := range positions {
			next, e := mongoCASRow(image, p.name, p.id)
			if e != nil {
				return nil, e
			}
			out[p.name][p.index] = next
		}
	}
	mongoCASSort(out)
	return out, nil
}
func (s *HistoricalCASSpool) mongoPlan(f historicalSpoolFrame, before map[string][]bson.Raw) (*MongoHistoricalBatchCASPlan, error) {
	p := &MongoHistoricalBatchCASPlan{db: s.db, config: f.Config, oldTxn: s.oldMongoTxn, metadataHash: f.Metadata, identity: f.Identity, originalSQLIdentity: f.OriginalSQLIdentity, originalSQLRows: f.OriginalSQLRows, originalSQLConnection: s.oldSQLPool, selection: historicalSpoolSelectionIn(f.Selection), limits: f.Limits, hints: f.Hints, expires: s.expires, before: before}
	for _, g := range f.Groups {
		raw, e := mongoCASRow(before, g.Collection, g.ID)
		if e != nil {
			return nil, e
		}
		stored, e := mongoCASSet(raw, g.Slot)
		if e != nil {
			return nil, e
		}
		for _, entry := range g.Entries {
			stored, e = stored.Append(entry)
			if e != nil {
				return nil, e
			}
		}
		p.groups = append(p.groups, mongoCASGroup{g.Collection, g.Slot, g.ID, g.PK, stored, g.Entries})
	}
	for _, a := range f.Attachments {
		p.attachments = append(p.attachments, mongoCASAttachmentFacts{a.Collection, a.Slot, a.ID, a.Entry, a.Content})
	}
	return p, nil
}

// The HOST must abort both transactions on any Apply error. This holder is
// poisoned and cannot retry. All pages use the identical genuine RW epoch.
func (s *HistoricalCASSpool) ApplyTicket(ctx context.Context, t *HistoricalCASSpoolTicket) (result error) {
	if s == nil {
		return ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if result != nil {
			s.poisoned = true
		}
	}()
	if s.valid(ctx) != nil || !s.sealed || s.applied >= len(s.frames) || s.frames[s.applied] != t || t.applied || casPersistenceSQLPoolEnded(ctx, s.oldSQLPool) != nil || casPersistenceMongoEpochEnded(s.oldMongoSession, s.oldMongoTxn) != nil {
		return ErrHistoricalCASSpool
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx.Statement == nil || tx.Statement.ConnPool == s.oldSQLPool {
		return ErrHistoricalCASSpool
	}
	session := mongo.SessionFromContext(ctx)
	actual, e := mongoCycleTransaction(ctx, s.db)
	if e != nil || session == nil || actual.number == s.oldMongoTxn.number && bytes.Equal(actual.session, s.oldMongoTxn.session) {
		return ErrHistoricalCASSpool
	}
	if s.writeSQLPool != nil && (s.writeSQLPool != tx.Statement.ConnPool || s.writeMongoSession != session || s.writeMongoTxn.number != actual.number || !bytes.Equal(s.writeMongoTxn.session, actual.session)) {
		return ErrHistoricalCASSpool
	}
	f, e := s.load(ctx, t)
	if e != nil {
		return e
	}
	before, e := s.mongoPredecessors(ctx, historicalSpoolRawIn(f.Before))
	if e != nil {
		return e
	}
	plan, e := s.mongoPlan(f, before)
	if e != nil {
		return e
	}
	if e = s.sql.ApplyTicket(ctx, t.sql); e != nil {
		return e
	}
	var expected map[string][]bson.Raw
	if f.MongoReadOnly {
		meta, e := observeMongoCycleMetadata(ctx, s.db, f.Config)
		if e != nil || meta.hash != f.Metadata || meta.identity != f.Identity {
			return ErrMongoBatchConflict
		}
		expected, e = plan.capture(ctx)
		if e != nil || !reflect.DeepEqual(expected, before) {
			return ErrMongoBatchConflict
		}
	} else {
		statement, e := plan.Apply(ctx)
		if e != nil {
			return e
		}
		if statement.plan != plan || statement.transaction.number != actual.number || !bytes.Equal(statement.transaction.session, actual.session) {
			return ErrHistoricalCASSpool
		}
		expected = statement.expected
	}
	ref, e := s.put(ctx, historicalSpoolRawOut(expected))
	if e != nil {
		return e
	}
	originalRaw := historicalSpoolRawIn(f.Before)
	for _, g := range plan.groups {
		old, e := mongoCASRow(originalRaw, g.collection, g.id)
		if e != nil {
			return e
		}
		next, e := mongoCASRow(expected, g.collection, g.id)
		if e != nil {
			return e
		}
		// Compare every non-dedicated raw element exactly, including element order,
		// physical NULL, standard evidence, new fields and millisecond time.
		if !historicalSpoolMongoOnlySlot(old, next, g.slot) {
			return ErrMongoBatchConflict
		}
		key := mongoCASKey(g.collection, g.id)
		s.mongoDeltas[key] = historicalSpoolMongoDelta{historicalSpoolSHA(old), ref, g.collection, g.id}
	}
	t.applied = true
	s.applied++
	s.writeSQLPool, s.writeMongoSession, s.writeMongoTxn = tx.Statement.ConnPool, session, actual
	return nil
}
func historicalSpoolMongoOnlySlot(old, next bson.Raw, slot string) bool {
	strip := func(raw bson.Raw) (bson.Raw, error) {
		elements, e := raw.Elements()
		if e != nil {
			return nil, e
		}
		out := bson.D{}
		seen := false
		for _, element := range elements {
			if element.Key() == slot {
				if seen {
					return nil, ErrMongoBatchConflict
				}
				seen = true
				continue
			}
			out = append(out, bson.E{Key: element.Key(), Value: element.Value()})
		}
		return bson.Marshal(out)
	}
	a, e1 := strip(old)
	b, e2 := strip(next)
	return e1 == nil && e2 == nil && bytes.Equal(a, b)
}
func (s *HistoricalCASSpool) digest() string {
	keys := make([]string, 0, len(s.mongoDeltas))
	for k := range s.mongoDeltas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{"actual-whole-event-spool-apply/v1", s.binding.SourceSHA, s.binding.OperationID, strconv.Itoa(s.applied)}
	for _, t := range s.frames {
		parts = append(parts, t.record.SHA256)
	}
	for _, k := range keys {
		d := s.mongoDeltas[k]
		parts = append(parts, k, d.OriginalSHA, d.Record.SHA256)
	}
	return mongoOwnerHashParts(parts...)
}
func (s *HistoricalCASSpool) FinishApply(ctx context.Context) (*HistoricalCASSpoolApplied, error) {
	if s == nil {
		return nil, ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || s.applied != len(s.frames) || len(s.frames) == 0 || s.finalSeal != "" {
		return nil, ErrHistoricalCASSpool
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx.Statement.ConnPool != s.writeSQLPool || mongo.SessionFromContext(ctx) != s.writeMongoSession {
		return nil, ErrHistoricalCASSpool
	}
	actual, e := mongoCycleTransaction(ctx, s.db)
	if e != nil || actual.number != s.writeMongoTxn.number || !bytes.Equal(actual.session, s.writeMongoTxn.session) {
		return nil, ErrHistoricalCASSpool
	}
	statement, e := s.sql.FinishApply(ctx)
	if e != nil {
		return nil, e
	}
	s.sqlStatement = statement
	s.finalSeal = s.digest()
	p := &HistoricalCASSpoolApplied{owner: s, seal: s.finalSeal}
	p.self = p
	return p, nil
}

// This reuses the earliest REAL prepared-page expiry. It cannot replace it
// with now+budget, prolong an origin proof, or authorize a production host.
func (s *HistoricalCASSpool) InheritedContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	if s == nil {
		return nil, nil, ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(parent) != nil || !s.sealed || s.expires.IsZero() {
		return nil, nil, ErrHistoricalCASSpool
	}
	ctx, cancel := context.WithDeadline(parent, s.expires)
	return ctx, cancel, nil
}
func (s *HistoricalCASSpool) Tickets() ([]*HistoricalCASSpoolTicket, error) {
	if s == nil {
		return nil, ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.self != s || !s.sealed || s.poisoned {
		return nil, ErrHistoricalCASSpool
	}
	return append([]*HistoricalCASSpoolTicket(nil), s.frames...), nil
}
func (t *HistoricalCASSpoolTicket) Diagnostic() (int, uint64) {
	if t == nil || t.self != t {
		return -1, 0
	}
	return t.position, t.entries
}
func historicalSpoolFreshIndex(ctx context.Context, index *WholeSourceJointIndex, binding HistoricalCoordinatorBinding) error {
	if index == nil || index.owner == nil {
		return ErrSourceAuthentication
	}
	c := index.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alive(ctx) != nil || c.failed || c.binding != binding || c.authenticated == nil || index.auth != c.authenticated || !index.complete || !index.auth.complete {
		return ErrSourceAuthentication
	}
	return nil
}

// Third independently authenticated copies/index are real capabilities. The
// source handles are reread from ORIGINAL ReaderAt frames, not supplied DTOs.
func (s *HistoricalCASSpool) VerifyTicket(ctx context.Context, applied *HistoricalCASSpoolApplied, t *HistoricalCASSpoolTicket, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, index *WholeSourceJointIndex) (HistoricalCASSpoolPageObservation, error) {
	var report HistoricalCASSpoolPageObservation
	if s == nil {
		return report, ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || applied == nil || applied.self != applied || applied.owner != s || s.finalSeal == "" || applied.seal != s.finalSeal || s.applied != len(s.frames) || current == nil || global == nil || index == nil || historicalSpoolFreshIndex(ctx, index, s.binding) != nil || !index.complete || index.auth == nil || !index.auth.complete || index.indexSHA != s.indexSHA || index.encodedSHA != s.encoded || index.receipts != s.copies || current.ValidateBorrowedSnapshot(ctx) != nil || global.ValidateBorrowedSnapshot(ctx) != nil || casPersistenceSQLPoolEnded(ctx, s.oldSQLPool) != nil || casPersistenceSQLPoolEnded(ctx, s.writeSQLPool) != nil || casPersistenceMongoEpochEnded(s.oldMongoSession, s.oldMongoTxn) != nil || casPersistenceMongoEpochEnded(s.writeMongoSession, s.writeMongoTxn) != nil {
		return report, ErrHistoricalCASSpool
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx.Statement.ConnPool == s.oldSQLPool || tx.Statement.ConnPool == s.writeSQLPool {
		return report, ErrHistoricalCASSpool
	}
	actual, e := mongoCycleTransaction(ctx, global.db)
	if e != nil || actual.number == s.oldMongoTxn.number && bytes.Equal(actual.session, s.oldMongoTxn.session) || actual.number == s.writeMongoTxn.number && bytes.Equal(actual.session, s.writeMongoTxn.session) {
		return report, ErrHistoricalCASSpool
	}
	if s.freshSQLPool != nil && (s.freshSQLPool != tx.Statement.ConnPool || s.freshMongoTxn.number != actual.number || !bytes.Equal(s.freshMongoTxn.session, actual.session)) {
		return report, ErrHistoricalCASSpool
	}
	f, e := s.load(ctx, t)
	if e != nil || !t.applied {
		return report, ErrHistoricalCASSpool
	}
	sources := make([]*VerifiedSourceEvent, 0, len(f.Sources))
	for _, item := range f.Sources {
		handle, e := index.event(ctx, item.ID)
		if e != nil {
			return report, e
		}
		facts, e := handle.Facts()
		if e != nil {
			return report, e
		}
		key, e := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		hash, he := privateFactsSHA(facts)
		if e != nil || he != nil || key != (verifiedSourceKey{item.Object, item.PK}) || hash != item.FactsSHA {
			return report, ErrSourceAuthentication
		}
		sources = append(sources, handle)
	}
	sqlBatch, e := PrepareSQLBusinessOwnerBatch(ctx, current, f.SQLRequest, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
	if e != nil {
		return report, e
	}
	sqlReport, e := s.sql.VerifyTicket(ctx, s.sqlStatement, t.sql, sqlBatch.facts)
	if e != nil {
		return report, e
	}
	limits := f.Limits
	limits.MaxSources = 512
	mongoBatch, e := PrepareMongoHistoricalOwnerBatch(ctx, global, sqlBatch.facts, sources, limits)
	if e != nil {
		return report, e
	}
	expected, e := s.mongoPredecessors(ctx, historicalSpoolRawIn(f.Before))
	if e != nil {
		return report, e
	}
	if global.db != s.db || global.metadata.hash != f.Metadata || global.metadata.identity != f.Identity || !reflect.DeepEqual(mongoBatch.selection, historicalSpoolSelectionIn(f.Selection)) || !reflect.DeepEqual(mongoBatch.data, expected) || mongoBatch.ValidateBorrowedSnapshot(ctx) != nil || historicalSpoolFreshIndex(ctx, index, s.binding) != nil {
		return report, ErrMongoBatchConflict
	}
	report = HistoricalCASSpoolPageObservation{Protocol: "limited-historical-spool-page-readback/v1", SourceSHA: s.binding.SourceSHA, OperationID: s.binding.OperationID, PageSHA256: f.PageSHA, SQLFinalRowsSHA256: sqlReport.ExpectedRowsSHA256, MongoFinalRowsSHA256: mongoCASHash(expected, f.Metadata), References: f.Entries, IndependentRawReadbackMatched: true}
	s.freshSQLPool, s.freshMongoTxn = tx.Statement.ConnPool, actual
	s.verified[t.position] = true
	return report, nil
}

func historicalSpoolMongoID(raw bson.Raw) (uint64, error) {
	id, ok := mongoExactInteger(raw.Lookup("domain_id"))
	if !ok || id <= 0 {
		return 0, ErrMongoBatchCAS
	}
	return uint64(id), nil
}

// Selectors come from the genuine consumed-page seal, never a candidate DTO.
// They select a replay; they do not authorize it or extend its original budget.
func (a *WholeSourceJointReplayAnchor) Selectors() (uint64, int, error) {
	if a == nil || a.self != a || a.seal == "" || a.seal != a.digest() {
		return 0, 0, ErrWholeSourceJoint
	}
	return a.receipt.Sequence, a.candidateOffset, nil
}

// Final limited event observation is possible only after every genuine ticket
// was read in the SAME actual third scope. This is still not AI/closure/DROP.
func (s *HistoricalCASSpool) FinishReadback(ctx context.Context, applied *HistoricalCASSpoolApplied) error {
	if s == nil {
		return ErrHistoricalCASSpool
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid(ctx) != nil || applied == nil || applied.self != applied || applied.owner != s || s.finalSeal == "" || applied.seal != s.finalSeal || s.applied != len(s.frames) || len(s.verified) != len(s.frames) || s.freshSQLPool == nil {
		return ErrHistoricalCASSpool
	}
	for i := range s.frames {
		if !s.verified[i] {
			return ErrHistoricalCASSpool
		}
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx.Statement.ConnPool != s.freshSQLPool {
		return ErrHistoricalCASSpool
	}
	actual, e := mongoCycleTransaction(ctx, s.db)
	if e != nil || actual.number != s.freshMongoTxn.number || !bytes.Equal(actual.session, s.freshMongoTxn.session) {
		return ErrHistoricalCASSpool
	}
	return nil
}

func (*HistoricalCASSpool) MarshalBSON() ([]byte, error)        { return nil, ErrSourceSerialization }
func (*HistoricalCASSpool) UnmarshalJSON([]byte) error          { return ErrSourceSerialization }
func (*HistoricalCASSpool) UnmarshalBSON([]byte) error          { return ErrSourceSerialization }
func (*HistoricalCASSpoolTicket) MarshalBSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*HistoricalCASSpoolTicket) UnmarshalJSON([]byte) error    { return ErrSourceSerialization }
func (*HistoricalCASSpoolTicket) UnmarshalBSON([]byte) error    { return ErrSourceSerialization }
func (*HistoricalCASSpoolApplied) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASSpoolApplied) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*HistoricalCASSpoolApplied) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (s *HistoricalCASSpool) GoString() string                  { return s.String() }
func (*HistoricalCASSpoolTicket) String() string {
	return "private event-page disk ticket; no import or authority"
}
func (t *HistoricalCASSpoolTicket) GoString() string { return t.String() }
func (*HistoricalCASSpoolApplied) String() string {
	return "private event-spool statement; commit response unproven"
}
func (t *HistoricalCASSpoolApplied) GoString() string { return t.String() }
