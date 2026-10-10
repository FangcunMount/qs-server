package retirement

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"sort"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	ErrMongoSnapshotInput       SourceError = "mongo_snapshot_input_epoch_rejected"
	mongoSnapshotInputPageBytes             = 16 << 20
)

// This budget belongs to a NEW input-only read, never an extension of an
// existing transaction, owner page or global responsibility qualification.
type MongoSnapshotInputLimits struct {
	Scan        MongoResponsibilityLimits
	MaxDuration time.Duration
}

func (l MongoSnapshotInputLimits) valid() bool {
	return l.Scan.valid() && l.MaxDuration > 0 && l.MaxDuration <= 30*time.Minute
}

// MongoSnapshotInputEpoch is not a MongoResponsibilitySnapshot. The host
// supplies and owns a real snapshot-only Session and an empty private file.
// Neither this input nor any frozen page is accepted by the existing CAS,
// origin, transaction, global responsibility, fence or DROP APIs.
type MongoSnapshotInputEpoch struct {
	self      *MongoSnapshotInputEpoch
	db        *mongo.Database
	config    MongoOwnerConfig
	session   mongo.Session
	sessionID bson.Raw
	snapshot  primitive.Timestamp
	started   time.Time
	limits    MongoSnapshotInputLimits
	file      *os.File
	dev, ino  uint64
	end       int64
	pages     []historicalSpoolRef
	metadata  mongoCycleMetadata
	report    MongoResponsibilityCycleReport
	complete  bool
	poisoned  bool
}

type mongoSnapshotInputFrame struct {
	Version    int
	SessionID  bson.Raw
	Snapshot   primitive.Timestamp
	Metadata   string
	Collection string
	Boundary   mongoCycleBoundaryDisk
	Rows       []bson.Raw
	EOF        bool
}

type mongoCycleBoundaryDisk struct {
	Report       MongoResponsibilityCollectionReport
	Lower, Upper bson.RawValue
}

type MongoSnapshotInputSummary struct {
	Protocol, IdentitySHA256, MetadataSHA256, SnapshotSHA256, NativeEpochSHA256 string
	Rows, Bytes, Pages                                                          uint64
	CompleteInput, CASAuthorized, DropReady                                     bool
	Collections                                                                 []MongoResponsibilityCollectionReport
}

type MongoSnapshotInputPage struct {
	self  *MongoSnapshotInputPage
	owner *MongoSnapshotInputEpoch
	index int
	frame mongoSnapshotInputFrame
}

func (*MongoSnapshotInputEpoch) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoSnapshotInputEpoch) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoSnapshotInputPage) MarshalJSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*MongoSnapshotInputPage) MarshalBSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*MongoSnapshotInputEpoch) String() string {
	return "private Mongo snapshot input; no write authority"
}
func (*MongoSnapshotInputPage) String() string {
	return "private frozen Mongo input page; no write authority"
}

func snapshotInputNative(ctx context.Context, requireTime bool) (mongo.Session, bson.Raw, primitive.Timestamp, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, nil, primitive.Timestamp{}, ErrMongoSnapshotInput
	}
	s := mongo.SessionFromContext(ctx)
	x, ok := s.(mongo.XSession) //nolint:staticcheck // actual pinned driver snapshot state, not an imported DTO
	if !ok || x.ClientSession() == nil {
		return nil, nil, primitive.Timestamp{}, ErrMongoSnapshotInput
	}
	native := x.ClientSession()
	if native.Terminated || native.IsImplicit || !native.Snapshot || native.Consistent || native.TransactionRunning() || native.Committing || native.Aborting {
		return nil, nil, primitive.Timestamp{}, ErrMongoSnapshotInput
	}
	id := s.ID()
	fields, err := exactBSONFields(id)
	if err != nil || len(fields) != 1 || fields["id"].Type != bson.TypeBinary {
		return nil, nil, primitive.Timestamp{}, ErrMongoSnapshotInput
	}
	subtype, value := fields["id"].Binary()
	if subtype != 4 || len(value) != 16 {
		return nil, nil, primitive.Timestamp{}, ErrMongoSnapshotInput
	}
	var at primitive.Timestamp
	if native.SnapshotTime != nil {
		at = *native.SnapshotTime
	}
	if requireTime && at.T == 0 {
		return nil, nil, primitive.Timestamp{}, ErrMongoSnapshotInput
	}
	return s, append(bson.Raw(nil), id...), at, nil
}

func (e *MongoSnapshotInputEpoch) validFile(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || e == nil || e.self != e || e.file == nil || e.poisoned {
		return ErrMongoSnapshotInput
	}
	info, err := e.file.Stat()
	if err != nil {
		return ErrMongoSnapshotInput
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || uint64(st.Dev) != e.dev || uint64(st.Ino) != e.ino || info.Size() != e.end {
		return ErrMongoSnapshotInput
	}
	return nil
}

// ValidateBorrowedInputEpoch checks an actual server-selected snapshot time.
// It cannot validate a transaction or keep any previous qualification alive.
func (e *MongoSnapshotInputEpoch) ValidateBorrowedInputEpoch(ctx context.Context) error {
	if e.validFile(ctx) != nil || !time.Now().Before(e.started.Add(e.limits.MaxDuration)) {
		return ErrMongoSnapshotInput
	}
	s, id, at, err := snapshotInputNative(ctx, true)
	if err != nil || s != e.session || !bytes.Equal(id, e.sessionID) || at != e.snapshot {
		return ErrMongoSnapshotInput
	}
	return nil
}

// PrepareMongoSnapshotInputEpoch reads complete native pages in one real
// nontransaction snapshot. Unsupported server/read/permission/history-window
// errors fail normally. The host must not retry by replacing the session/time.
func PrepareMongoSnapshotInputEpoch(parent context.Context, db *mongo.Database, config MongoOwnerConfig, limits MongoSnapshotInputLimits, file *os.File) (result *MongoSnapshotInputEpoch, err error) {
	if parent == nil || db == nil || file == nil || !limits.valid() || !evidence.ValidSHA256(config.ExpectedIdentityHash) || config.ExpectedMigrationVersion <= 0 {
		return nil, ErrMongoSnapshotInput
	}
	s, id, _, err := snapshotInputNative(parent, false)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, ErrMongoSnapshotInput
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || info.Size() != 0 {
		return nil, ErrMongoSnapshotInput
	}
	e := &MongoSnapshotInputEpoch{db: db, config: config, session: s, sessionID: id, limits: limits, started: time.Now(), file: file, dev: uint64(st.Dev), ino: uint64(st.Ino)}
	e.self = e
	defer func() {
		if err != nil {
			e.poisoned = true
		}
	}()
	scope, cancel := context.WithDeadline(parent, e.started.Add(limits.MaxDuration))
	defer cancel()
	ctx := mongo.NewSessionContext(scope, s)
	// Metadata commands remain outside the session. The original metadata
	// reader's schema-head Find is the first real snapshot response, whose
	// atClusterTime the pinned driver retains in SnapshotTime.
	e.metadata, err = observeMongoCycleMetadata(ctx, db, config)
	if err != nil {
		return nil, err
	}
	_, _, e.snapshot, err = snapshotInputNative(ctx, true)
	if err != nil || e.ValidateBorrowedInputEpoch(ctx) != nil {
		return nil, ErrMongoSnapshotInput
	}
	// This private collector is never complete and never escapes as a global
	// responsibility capability. Only its native paging/classification code
	// is reused, with the distinct input validator at every page boundary.
	collector := &MongoResponsibilitySnapshot{db: db, config: config, limits: limits.Scan, metadata: e.metadata, boundaries: map[string]mongoCycleBoundary{}, byEvent: map[string][]int{}, bySheet: map[string][]int{}, byAssessment: map[string][]int{}, byGeneration: map[string][]int{}, byOutcome: map[string][]int{}}
	collector.graph.initialize()
	collector.report = MongoResponsibilityCycleReport{Protocol: "mongo-snapshot-input/v1", IdentitySHA256: e.metadata.identity, MetadataSHA256: e.metadata.hash, MigrationVersion: config.ExpectedMigrationVersion, ClassCounts: map[string]uint64{}}
	for range e.metadata.unknown {
		collector.gap("unknown_catalog_namespace_coverage_required")
	}
	for _, name := range mongoCycleCollections {
		var b mongoCycleBoundary
		b, err = collector.scanCollectionWithInput(ctx, name, nil, true, e.ValidateBorrowedInputEpoch, e.freezePage)
		if err != nil {
			return nil, err
		}
		collector.boundaries[name] = b
		collector.report.Collections = append(collector.report.Collections, b.report)
	}
	if err = e.ValidateBorrowedInputEpoch(ctx); err != nil {
		return nil, err
	}
	end, err := observeMongoCycleMetadata(ctx, db, config)
	if err != nil {
		return nil, err
	}
	if end.hash != e.metadata.hash {
		return nil, ErrMongoCycleConflict
	}
	if err = collector.classifyGraph(); err != nil {
		return nil, err
	}
	collector.report.SnapshotSHA256 = collector.snapshotHash()
	sort.Strings(collector.report.BlockingReasons)
	sort.Strings(collector.report.CoverageGaps)
	if err = e.ValidateBorrowedInputEpoch(ctx); err != nil {
		return nil, err
	}
	e.report, e.complete = collector.report, true
	return e, nil
}

func (e *MongoSnapshotInputEpoch) freezePage(ctx context.Context, boundary mongoCycleBoundary, rows []bson.Raw, eof bool) error {
	if e.ValidateBorrowedInputEpoch(ctx) != nil || len(e.pages) >= int(e.limits.Scan.MaxPages)+len(mongoCycleCollections) {
		return ErrMongoSnapshotInput
	}
	f := mongoSnapshotInputFrame{Version: 1, SessionID: e.sessionID, Snapshot: e.snapshot, Metadata: e.metadata.hash, Collection: boundary.report.Collection, Boundary: mongoCycleBoundaryDisk{boundary.report, boundary.lower, boundary.upper}, Rows: rows, EOF: eof}
	raw, err := historicalSpoolEncode(f)
	if err != nil || len(raw) > mongoSnapshotInputPageBytes+(1<<20) || int64(len(raw)) > int64(e.limits.Scan.MaxBytes)+(64<<20)-e.end {
		return ErrMongoSnapshotInput
	}
	ref := historicalSpoolRef{e.end, int64(len(raw)), historicalSpoolSHA(raw)}
	n, err := e.file.WriteAt(raw, e.end)
	if err != nil || n != len(raw) {
		e.poisoned = true
		return ErrMongoSnapshotInput
	}
	e.end += int64(len(raw))
	if e.file.Sync() != nil || e.ValidateBorrowedInputEpoch(ctx) != nil {
		e.poisoned = true
		return ErrMongoSnapshotInput
	}
	e.pages = append(e.pages, ref)
	return nil
}

func (e *MongoSnapshotInputEpoch) Summary() MongoSnapshotInputSummary {
	if e == nil || e.self != e || !e.complete || e.poisoned {
		return MongoSnapshotInputSummary{Protocol: "mongo-snapshot-input/v1"}
	}
	var at [8]byte
	binary.BigEndian.PutUint32(at[:4], e.snapshot.T)
	binary.BigEndian.PutUint32(at[4:], e.snapshot.I)
	return MongoSnapshotInputSummary{Protocol: "mongo-snapshot-input/v1", IdentitySHA256: e.report.IdentitySHA256, MetadataSHA256: e.report.MetadataSHA256, SnapshotSHA256: e.report.SnapshotSHA256, NativeEpochSHA256: mongoOwnerHashParts(string(e.sessionID), string(at[:])), Rows: e.report.Rows, Bytes: e.report.Bytes, Pages: uint64(len(e.pages)), CompleteInput: true, Collections: append([]MongoResponsibilityCollectionReport(nil), e.report.Collections...)}
}

// ReadFrozenPage checks exact original private bytes after the read session
// has ended. Its result remains INPUT ONLY and carries no renewable lifetime.
func (e *MongoSnapshotInputEpoch) ReadFrozenPage(ctx context.Context, index int) (*MongoSnapshotInputPage, error) {
	if e.validFile(ctx) != nil || !e.complete || index < 0 || index >= len(e.pages) {
		return nil, ErrMongoSnapshotInput
	}
	r := e.pages[index]
	if r.Offset < 0 || r.Length <= 0 || r.Length > mongoSnapshotInputPageBytes+(1<<20) || r.Offset > e.end-r.Length {
		return nil, ErrMongoSnapshotInput
	}
	raw := make([]byte, r.Length)
	n, err := e.file.ReadAt(raw, r.Offset)
	var frame mongoSnapshotInputFrame
	if err != nil || n != len(raw) || historicalSpoolSHA(raw) != r.SHA256 || historicalSpoolDecode(raw, &frame) != nil || frame.Version != 1 || !bytes.Equal(frame.SessionID, e.sessionID) || frame.Snapshot != e.snapshot || frame.Metadata != e.metadata.hash || frame.Collection != frame.Boundary.Report.Collection {
		return nil, ErrMongoSnapshotInput
	}
	p := &MongoSnapshotInputPage{owner: e, index: index, frame: frame}
	p.self = p
	return p, nil
}

func (p *MongoSnapshotInputPage) VisitRows(ctx context.Context, visit func(string, bson.Raw) error) error {
	if p == nil || p.self != p || p.owner == nil || visit == nil || ctx == nil || ctx.Err() != nil {
		return ErrMongoSnapshotInput
	}
	actual, err := p.owner.ReadFrozenPage(ctx, p.index)
	if err != nil {
		return err
	}
	for _, raw := range actual.frame.Rows {
		if ctx.Err() != nil || visit(actual.frame.Collection, append(bson.Raw(nil), raw...)) != nil {
			return ErrMongoSnapshotInput
		}
	}
	return nil
}

// Both inputs must come from genuinely different native session instances.
// A server-selected majority snapshot time may remain equal without writes,
// and the driver may reuse an ended server-session UUID from its pool. Neither
// fact replaces the native session instance or permits an older snapshot.
func (e *MongoSnapshotInputEpoch) CompareFreshInput(ctx context.Context, fresh *MongoSnapshotInputEpoch) error {
	if e.validFile(ctx) != nil || fresh == nil || fresh.validFile(ctx) != nil || !e.complete || !fresh.complete || e == fresh || e.session == fresh.session || fresh.snapshot.T < e.snapshot.T || fresh.snapshot.T == e.snapshot.T && fresh.snapshot.I < e.snapshot.I || e.report.SnapshotSHA256 != fresh.report.SnapshotSHA256 || e.metadata.hash != fresh.metadata.hash {
		return ErrMongoSnapshotInput
	}
	// Cached observations do not certify that the original frozen bytes are
	// still intact. Recheck their existing private frame hashes, without a
	// database rescan or any renewed read/write qualification.
	for _, input := range []*MongoSnapshotInputEpoch{e, fresh} {
		for index := range input.pages {
			if _, err := input.ReadFrozenPage(ctx, index); err != nil {
				return err
			}
		}
	}
	return nil
}
