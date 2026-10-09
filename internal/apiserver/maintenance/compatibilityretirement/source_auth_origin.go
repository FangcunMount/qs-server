package retirement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"reflect"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

const (
	ErrSourceOrigin      SourceError = "source_origin_rejected"
	ErrSourceOriginFresh SourceError = "source_origin_independent_epoch_required"
)

type SourceOriginLimits struct {
	PageRows                  int
	MaxRows, MaxBytes         uint64
	MaxDuration, QueryTimeout time.Duration
}

func DefaultSourceOriginLimits() SourceOriginLimits {
	return SourceOriginLimits{PageRows: 512, MaxRows: MaxSourceIndexReservation / sourceIndexReservationPerRow, MaxBytes: 4 * MaxSourceBytes, MaxDuration: 30 * time.Minute, QueryTimeout: 15 * time.Second}
}
func (l SourceOriginLimits) valid() bool {
	return l.PageRows > 0 && l.PageRows <= 4096 && l.MaxRows > 0 && l.MaxRows <= MaxSourceIndexReservation/sourceIndexReservationPerRow && l.MaxBytes > 0 && l.MaxBytes <= 4*MaxSourceBytes && l.MaxDuration > 0 && l.MaxDuration <= time.Hour && l.QueryTimeout > 0 && l.QueryTimeout <= time.Minute
}

// Original SourceAuth did not retain Boundary or full-file hashes. This new
// handle binds them from construction onward; it cannot repair that earlier
// continuity gap or turn supplied expectations into human/workflow approval.
type OriginCopyBinding struct {
	copies     *VerifiedSourceCopies
	expected   [4]SourceCopyExpectation
	fileHashes [4]string
	fileBytes  [4]uint64
	limits     SourceOriginLimits
	started    time.Time
	hash       string
}
type SourceOriginEpoch struct {
	binding                                                        *OriginCopyBinding
	sql                                                            *sqlevaluation.SQLHistoricalResponsibilityCycle
	mongo                                                          *MongoResponsibilitySnapshot
	sqlConnection                                                  any
	sqlCycleID, sqlIdentity, sqlHead, mongoIdentity, mongoMetadata string
	transaction                                                    mongoCycleTxn
	receipts                                                       [4]SourceCopyReceipt
	boundaries                                                     [4]SourceBoundary
	hash                                                           string
}
type SourceOriginRecheckProof struct {
	first, second *SourceOriginEpoch
	firstAnchor   *FreshRecheckAnchor
}
type SourceOriginReport struct {
	Protocol, BindingSHA256, FirstEpochSHA256, SecondEpochSHA256, SQLIdentitySHA256, MongoIdentitySHA256 string
	Sources                                                                                              [4]SourceCopyReceipt
	SourceFilesMatched, ActualOriginMatched, IndependentEpochRechecked                                   bool
	IndependentApprovalRequired, FirstAuthMetadataContinuityUnproven, HostProcessBudgetRequired          bool
	IndependentDatabaseSnapshots, WriterFenceRequired                                                    bool
	BusinessClosureVerified, CASAuthorized, DropReady                                                    bool
}

func (*OriginCopyBinding) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*OriginCopyBinding) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*OriginCopyBinding) String() string {
	return "private origin copy binding; independent approval required"
}
func (b *OriginCopyBinding) GoString() string           { return b.String() }
func (*SourceOriginEpoch) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SourceOriginEpoch) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SourceOriginEpoch) String() string {
	return "private actual paired source epoch; fresh recheck required"
}
func (e *SourceOriginEpoch) GoString() string                  { return e.String() }
func (*SourceOriginRecheckProof) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SourceOriginRecheckProof) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SourceOriginRecheckProof) String() string {
	return "private origin recheck; business and approval unproven"
}
func (p *SourceOriginRecheckProof) GoString() string { return p.String() }
func originReport() SourceOriginReport {
	return SourceOriginReport{Protocol: "actual-four-source-origin/v1", IndependentApprovalRequired: true, FirstAuthMetadataContinuityUnproven: true, HostProcessBudgetRequired: true, IndependentDatabaseSnapshots: true, WriterFenceRequired: true}
}
func (p *SourceOriginRecheckProof) Report() SourceOriginReport {
	r := originReport()
	if p == nil || p.second == nil {
		return r
	}
	if p.firstAnchor != nil {
		if !p.firstAnchor.intact() {
			return r
		}
		r.BindingSHA256 = p.firstAnchor.binding.hash
		r.FirstEpochSHA256 = p.firstAnchor.originalEpochHash
	} else {
		if p.first == nil {
			return r
		}
		r.BindingSHA256 = p.first.binding.hash
		r.FirstEpochSHA256 = p.first.hash
	}
	r.SecondEpochSHA256 = p.second.hash
	r.SQLIdentitySHA256 = p.second.sqlIdentity
	r.MongoIdentitySHA256 = p.second.mongoIdentity
	r.Sources = p.second.receipts
	r.SourceFilesMatched = true
	r.ActualOriginMatched = true
	r.IndependentEpochRechecked = true
	return r
}
func sourceOriginDigest(v any) (string, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return "", ErrSourceOrigin
	}
	return sourceSHA(raw), nil
}
func (b *OriginCopyBinding) alive(ctx context.Context) error {
	if b == nil || ctx == nil || b.copies == nil || !b.copies.complete || !b.limits.valid() || time.Since(b.started) > b.limits.MaxDuration {
		return ErrSourceOrigin
	}
	if ctx.Err() != nil {
		return ErrSourceIncomplete
	}
	return nil
}
func sourceOriginObserve(copies *VerifiedSourceCopies, value any, sourceDatabase, object, pk string) error {
	key, e := sourceAuthKey(sourceDatabase, object, pk)
	if e != nil {
		return e
	}
	old, ok := copies.rows[key]
	if !ok {
		return ErrSourceAuthentication
	}
	h, e := privateFactsSHA(value)
	if e != nil || old.facts != h {
		return ErrSourceAuthentication
	}
	return nil
}

type sourceOriginFileHash struct {
	h hash.Hash
	n uint64
}

func (h *sourceOriginFileHash) Write(raw []byte) (int, error) {
	if uint64(len(raw)) > 2*MaxSourceBytes+(64<<20)-h.n {
		return 0, ErrSourceBounds
	}
	n, e := h.h.Write(raw)
	h.n += uint64(n)
	return n, e
}

// Read borrowed original streams completely; no file, reader or connection is
// opened/closed here. The old opaque facts/receipts must match exactly.
func (b *OriginCopyBinding) verifyFiles(ctx context.Context, readers []io.Reader, checkHash bool) ([4]string, [4]uint64, error) {
	var hashes [4]string
	var sizes [4]uint64
	if len(readers) != 4 || b.alive(ctx) != nil {
		return hashes, sizes, ErrSourceOrigin
	}
	for i, input := range readers {
		if sourceReaderAbsent(input) {
			return hashes, sizes, ErrSourceAuthentication
		}
		h := &sourceOriginFileHash{h: sha256.New()}
		stream := io.TeeReader(input, h)
		expected := b.expected[i]
		var receipt SourceCopyReceipt
		if i == 0 || i == 3 {
			var next func() (*DecodedSourceEvent, error)
			var result func() SourceCopyReceipt
			if i == 0 {
				d, e := NewSQLSourceReader(stream, expected)
				if e != nil {
					return hashes, sizes, e
				}
				next, result = d.Next, d.Receipt
			} else {
				d, e := NewMongoSourceReader(stream, expected)
				if e != nil {
					return hashes, sizes, e
				}
				next, result = d.Next, d.Receipt
			}
			for {
				if e := b.alive(ctx); e != nil {
					return hashes, sizes, e
				}
				value, e := next()
				if e == io.EOF {
					break
				}
				if e != nil {
					return hashes, sizes, e
				}
				if e = sourceOriginObserve(b.copies, value, value.Source.Database, value.Source.Object, value.Source.PrimaryKeySHA256); e != nil {
					return hashes, sizes, e
				}
			}
			receipt = result()
		} else {
			d, e := NewAISQLSourceReader(stream, expected)
			if e != nil {
				return hashes, sizes, e
			}
			for {
				if e = b.alive(ctx); e != nil {
					return hashes, sizes, e
				}
				value, e := d.Next()
				if e == io.EOF {
					break
				}
				if e != nil {
					return hashes, sizes, e
				}
				if e = sourceOriginObserve(b.copies, value, value.Source.Database, value.Source.Object, value.Source.PrimaryKeySHA256); e != nil {
					return hashes, sizes, e
				}
			}
			receipt = d.Receipt()
		}
		if !receipt.Complete || receipt != b.copies.receipts[i] {
			return hashes, sizes, ErrSourceAuthentication
		}
		hashes[i], sizes[i] = hex.EncodeToString(h.h.Sum(nil)), h.n
		if checkHash && (hashes[i] != b.fileHashes[i] || sizes[i] != b.fileBytes[i]) {
			return hashes, sizes, ErrSourceAuthentication
		}
	}
	return hashes, sizes, b.alive(ctx)
}
func BindOriginCopies(ctx context.Context, copies *VerifiedSourceCopies, inputs []SourceCopyInput, limits SourceOriginLimits) (*OriginCopyBinding, error) {
	if ctx == nil || copies == nil || !copies.complete || len(inputs) != 4 || !limits.valid() {
		return nil, ErrSourceOrigin
	}
	b := &OriginCopyBinding{copies: copies, limits: limits, started: time.Now()}
	readers := make([]io.Reader, 4)
	var count, size uint64
	for i, input := range inputs {
		database, name := "mysql", "domain_event_outbox"
		if i == 1 {
			name = AIBridgeCommandSource
		}
		if i == 2 {
			name = AILegacyCommandSource
		}
		if i == 3 {
			database = "mongodb"
		}
		if input.Expected.Boundary.Database != database || input.Expected.Boundary.Name != name {
			return nil, ErrSourceAuthentication
		}
		b.expected[i] = input.Expected
		readers[i] = input.Input
		receipt := copies.receipts[i]
		if input.Expected.DataHash != receipt.DataHash || input.Expected.Records != receipt.Records || input.Expected.Bytes != receipt.Bytes {
			return nil, ErrSourceAuthentication
		}
		count += receipt.Records
		size += receipt.Bytes
	}
	if count > limits.MaxRows || size > limits.MaxBytes {
		return nil, ErrSourceBounds
	}
	var e error
	b.fileHashes, b.fileBytes, e = b.verifyFiles(ctx, readers, false)
	if e != nil {
		return nil, e
	}
	b.hash, e = sourceOriginDigest(struct {
		Expected [4]SourceCopyExpectation
		Hashes   [4]string
		Bytes    [4]uint64
	}{b.expected, b.fileHashes, b.fileBytes})
	if e != nil {
		return nil, e
	}
	return b, nil
}
func PrepareSourceOriginEpoch(ctx context.Context, b *OriginCopyBinding, sql *sqlevaluation.SQLHistoricalResponsibilityCycle, mgo *MongoResponsibilitySnapshot, readers []io.Reader) (*SourceOriginEpoch, error) {
	if b.alive(ctx) != nil || sql == nil || mgo == nil || !mgo.report.Complete || sql.Report().CompletedAt.IsZero() {
		return nil, ErrSourceOrigin
	}
	if e := sql.ValidateBorrowedSnapshot(ctx); e != nil {
		return nil, e
	}
	if e := mgo.ValidateBorrowedSnapshot(ctx); e != nil {
		return nil, e
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil {
		return nil, e
	}
	if _, _, e = b.verifyFiles(ctx, readers, true); e != nil {
		return nil, e
	}
	p := &SourceOriginEpoch{binding: b, sql: sql, mongo: mgo, sqlConnection: tx.Statement.ConnPool, sqlCycleID: sql.Report().CycleID, sqlIdentity: sql.Report().DatabaseIdentitySHA256, mongoIdentity: mgo.metadata.identity, transaction: mgo.txn, mongoMetadata: mgo.metadata.hash}
	p.sqlHead, e = sourceOriginSQLHead(ctx, tx, b.limits)
	if e != nil {
		return nil, e
	}
	for i := 0; i < 3; i++ {
		p.boundaries[i], p.receipts[i], e = sourceOriginReadSQL(ctx, tx, b, i)
		if e != nil {
			return nil, e
		}
	}
	p.boundaries[3], p.receipts[3], e = sourceOriginReadMongo(ctx, mgo, b)
	if e != nil {
		return nil, e
	}
	for i := 0; i < 4; i++ {
		if p.boundaries[i] != b.expected[i].Boundary || p.receipts[i] != b.copies.receipts[i] {
			return nil, ErrSourceOrigin
		}
	}
	if e = p.validate(ctx); e != nil {
		return nil, e
	}
	p.hash, e = sourceOriginDigest(struct {
		Binding, SQLCycle, SQLIdentity, SQLHead, MongoIdentity, MongoMetadata, MongoTransaction string
		Receipts                                                                                [4]SourceCopyReceipt
	}{b.hash, p.sqlCycleID, p.sqlIdentity, p.sqlHead, p.mongoIdentity, p.mongoMetadata, mongoOwnerHashParts("actual-mongo-session-txn/v1", string(p.transaction.session), originMongoNumber(p.transaction.number)), p.receipts})
	if e != nil {
		return nil, e
	}
	return p, nil
}
func (e *SourceOriginEpoch) validate(ctx context.Context) error {
	if e == nil || e.binding.alive(ctx) != nil {
		return ErrSourceOrigin
	}
	if err := e.sql.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	if err := e.mongo.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement.ConnPool != e.sqlConnection {
		return ErrSourceOriginFresh
	}
	head, err := sourceOriginSQLHead(ctx, tx, e.binding.limits)
	if err != nil || head != e.sqlHead {
		return ErrSourceOrigin
	}
	for i := 0; i < 3; i++ {
		boundary, _, _, err := sourceOriginSQLMetadata(ctx, tx, e.binding.expected[i], e.binding.limits)
		if err != nil || boundary != e.boundaries[i] {
			return ErrSourceOrigin
		}
	}
	metadata, err := observeMongoCycleMetadata(ctx, e.mongo.db, e.mongo.config)
	if err != nil || metadata.hash != e.mongoMetadata {
		return ErrSourceOrigin
	}
	return e.binding.alive(ctx)
}
func (e *SourceOriginEpoch) Recheck(ctx context.Context, sql *sqlevaluation.SQLHistoricalResponsibilityCycle, mgo *MongoResponsibilitySnapshot, readers []io.Reader) (*SourceOriginRecheckProof, error) {
	if e == nil || sql == nil || mgo == nil || e.binding.alive(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.Statement == nil || tx.Statement.ConnPool == e.sqlConnection || sql.Report().CycleID == e.sqlCycleID || (string(mgo.txn.session) == string(e.transaction.session) && mgo.txn.number == e.transaction.number) {
		return nil, ErrSourceOriginFresh
	}
	fresh, err := PrepareSourceOriginEpoch(ctx, e.binding, sql, mgo, readers)
	if err != nil {
		return nil, err
	}
	if fresh.sqlIdentity != e.sqlIdentity || fresh.sqlHead != e.sqlHead || fresh.mongoIdentity != e.mongoIdentity || fresh.mongoMetadata != e.mongoMetadata || !reflect.DeepEqual(fresh.boundaries, e.boundaries) || fresh.receipts != e.receipts {
		return nil, ErrSourceOrigin
	}
	return &SourceOriginRecheckProof{first: e, second: fresh}, nil
}
func originMongoNumber(n int64) string { return strconv.FormatInt(n, 10) }
