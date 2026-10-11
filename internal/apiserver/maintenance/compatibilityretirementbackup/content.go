package compatibilityretirementbackup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type countHash struct {
	h     hash.Hash
	count int64
}

func newCountHash() *countHash { return &countHash{h: sha256.New()} }
func (h *countHash) Write(b []byte) (int, error) {
	h.count += int64(len(b))
	if h.count > 4<<30 {
		return 0, ErrSource
	}
	return h.h.Write(b)
}
func (h *countHash) digest() string { return hex.EncodeToString(h.h.Sum(nil)) }
func copySource(ctx context.Context, dir string, index int, input io.Reader, s SourceSnapshot, columns retirement.SQLColumns) (Asset, error) {
	asset := Asset{Filename: sourceNames[index]}
	if readerMissing(input) {
		return asset, ErrSource
	}
	f, e := newPrivateFile(filepath.Join(dir, asset.Filename))
	if e != nil {
		return asset, e
	}
	h := newCountHash()
	r, e := newRawReader(ctx, io.TeeReader(input, io.MultiWriter(f, h)), index, s, columns)
	if e == nil {
		for {
			_, e = r.next()
			if e != nil {
				break
			}
		}
	}
	se, ce := f.Sync(), f.Close()
	if e != io.EOF || se != nil || ce != nil || syncDirectory(dir) != nil {
		return asset, ErrSource
	}
	asset.SHA256, asset.Bytes = h.digest(), h.count
	return asset, nil
}
func verifySQLContent(ctx context.Context, db sqlReader, structure SQLStructure, s SourceSnapshot, index int) error {
	columns := structure.Columns
	if !supportedColumns(index, columns) {
		return ErrStructure
	}
	projections := make([]string, len(columns))
	for i, c := range columns {
		projections[i] = "CAST(" + quote(*c[0]) + " AS BINARY)"
	}
	pk := *columns[0][0]
	var last any
	h := sha256.New()
	frame(h, []byte(jsonSHA(columns)), false)
	var count, size uint64
	for {
		if ctx.Err() != nil {
			return ErrBudget
		}
		q, c := context.WithTimeout(ctx, 30*time.Second)
		query := "SELECT " + strings.Join(projections, ",") + " FROM " + quote(s.Name) + " FORCE INDEX (PRIMARY)"
		var args []any
		if last != nil {
			query += " WHERE " + quote(pk) + " > ?"
			args = append(args, last)
		}
		query += " ORDER BY " + quote(pk) + " LIMIT 1000"
		rows, e := db.QueryContext(q, query, args...)
		if e != nil {
			c()
			return ErrRead
		}
		page := 0
		for rows.Next() {
			raw := make([]sql.RawBytes, len(columns))
			scan := make([]any, len(raw))
			for i := range raw {
				scan[i] = &raw[i]
			}
			if rows.Scan(scan...) != nil {
				_ = rows.Close()
				c()
				return ErrRead
			}
			var rowSize uint64
			for _, v := range raw {
				frame(h, v, v == nil)
				rowSize += uint64(len(v))
			}
			if rowSize > retirement.MaxSourceRowBytes || count >= s.Records || size > s.Bytes || rowSize > s.Bytes-size {
				_ = rows.Close()
				c()
				return ErrContent
			}
			size += rowSize
			count++
			page++
			if index == 0 {
				id, e := strconv.ParseUint(string(raw[0]), 10, 64)
				if e != nil || id == 0 {
					_ = rows.Close()
					c()
					return ErrContent
				}
				last = id
			} else {
				if len(raw[0]) == 0 {
					_ = rows.Close()
					c()
					return ErrContent
				}
				last = string(raw[0])
			}
		}
		readErr, closeErr := rows.Err(), rows.Close()
		c()
		if readErr != nil || closeErr != nil {
			return ErrRead
		}
		if page < 1000 {
			break
		}
	}
	if count != s.Records || size != s.Bytes || hex.EncodeToString(h.Sum(nil)) != s.DataHash {
		return ErrContent
	}
	return nil
}
func verifyMongoContent(ctx context.Context, db *mongo.Database, s SourceSnapshot) error {
	if e := verifyMongoIDTypes(ctx, db, s); e != nil {
		return e
	}
	h := sha256.New()
	var count, size uint64
	var last bson.RawValue
	haveLast := false
	for {
		if ctx.Err() != nil {
			return ErrBudget
		}
		q, c := context.WithTimeout(ctx, 30*time.Second)
		filter := bson.D{}
		if haveLast {
			filter = append(filter, bson.E{Key: "_id", Value: bson.D{{Key: "$gt", Value: last}}})
		}
		cur, e := db.Collection(s.Name).Find(q, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}).SetLimit(1000).SetBatchSize(1000))
		if e != nil {
			c()
			return ErrRead
		}
		page := 0
		for cur.Next(q) {
			raw := append(bson.Raw(nil), cur.Current...)
			id := raw.Lookup("_id")
			if raw.Validate() != nil || pkKind(id) != s.Boundary.PKType || count >= s.Records || size > s.Bytes || uint64(len(raw)) > s.Bytes-size {
				_ = cur.Close(q)
				c()
				return ErrContent
			}
			if haveLast {
				cmp, e := comparePK(last, id)
				if e != nil || cmp >= 0 {
					_ = cur.Close(q)
					c()
					return ErrContent
				}
			}
			last = bson.RawValue{Type: id.Type, Value: append([]byte(nil), id.Value...)}
			haveLast = true
			frame(h, raw, false)
			count++
			size += uint64(len(raw))
			page++
		}
		re, ce := cur.Err(), cur.Close(q)
		c()
		if re != nil || ce != nil {
			return ErrRead
		}
		if page < 1000 {
			break
		}
	}
	if count != s.Records || size != s.Bytes || hex.EncodeToString(h.Sum(nil)) != s.DataHash {
		return ErrContent
	}
	return verifyMongoIDTypes(ctx, db, s)
}

// openSource is private and owned by this backend. Hosts' original Inputs are
// never closed; only the backend's archive files opened here are closed.
func (a *Archive) openSource(ctx context.Context, index int) (*os.File, *rawReader, error) {
	f, e := openPrivateFile(filepath.Join(a.dir, sourceNames[index]))
	if e != nil {
		return nil, nil, e
	}
	columns := a.data.SQL[0].Columns
	if index < 3 {
		columns = a.data.SQL[index].Columns
	}
	r, e := a.sourceReader(ctx, f, index, columns)
	if e != nil {
		_ = f.Close()
		return nil, nil, e
	}
	return f, r, nil
}

func (a *Archive) sourceReader(ctx context.Context, in io.Reader, index int, columns retirement.SQLColumns) (*rawReader, error) {
	if a == nil || index < 0 || index > 3 {
		return nil, ErrSource
	}
	if a.data.Version == 2 {
		if validateStorageManifest(a.data) != nil {
			return nil, ErrApproval
		}
		return rawReaderForPass(ctx, in, index, a.data.Inventory.Targets[index], columns, 1)
	}
	return newRawReader(ctx, in, index, a.data.Inventory.Targets[index], columns)
}

func verifyMongoIDTypes(ctx context.Context, db *mongo.Database, s SourceSnapshot) error {
	q, c := context.WithTimeout(ctx, 30*time.Second)
	defer c()
	pipeline := mongo.Pipeline{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}}
	cur, e := db.Collection(s.Name).Aggregate(q, pipeline, options.Aggregate().SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		return ErrRead
	}
	var rows []struct {
		Kind string `bson:"_id"`
		N    int64  `bson:"n"`
	}
	e = cur.All(q, &rows)
	ce := cur.Close(q)
	if e != nil || ce != nil {
		return ErrRead
	}
	if s.Records == 0 {
		if len(rows) != 0 {
			return ErrContent
		}
		return nil
	}
	if len(rows) != 1 || rows[0].Kind != s.Boundary.PKType || rows[0].N < 0 || uint64(rows[0].N) != s.Records {
		return ErrContent
	}
	return nil
}

// storageSQLUpper reads only the primary key; no second original body pass is
// performed. Both observations use the same host-owned SQL transaction.
func storageSQLUpper(ctx context.Context, db sqlReader, index int, structure SQLStructure, s SourceSnapshot) ([]byte, error) {
	rows, e := readSQL(ctx, db, "SELECT CAST("+quote(*structure.Columns[0][0])+" AS BINARY) FROM "+quote(s.Name)+" FORCE INDEX (PRIMARY) ORDER BY "+quote(*structure.Columns[0][0])+" DESC LIMIT 1")
	if e != nil {
		return nil, e
	}
	if s.Boundary.Empty {
		if len(rows) != 0 {
			return nil, ErrContent
		}
		return nil, nil
	}
	upper, e := base64Raw(s.Boundary.UpperToken)
	if e != nil || len(rows) != 1 || len(rows[0]) != 1 || rows[0][0] == nil || !bytes.Equal([]byte(*rows[0][0]), upper) {
		return nil, ErrContent
	}
	if index == 0 {
		id, e := strconv.ParseUint(string(upper), 10, 64)
		if e != nil || id == 0 || strconv.FormatUint(id, 10) != string(upper) {
			return nil, ErrContent
		}
	} else if len(upper) == 0 {
		return nil, ErrContent
	}
	return upper, nil
}
func storageSQLRows(ctx context.Context, db sqlReader, index int, structure SQLStructure, s SourceSnapshot, out io.Writer) (SourceSnapshot, error) {
	if !supportedColumns(index, structure.Columns) {
		return s, ErrStructure
	}
	upper, e := storageSQLUpper(ctx, db, index, structure, s)
	if e != nil {
		return s, e
	}
	enc := json.NewEncoder(out)
	if enc.Encode(sqlHeader{Protocol: retirement.SQLSourceProtocol, Columns: structure.Columns, Boundary: s.Boundary}) != nil {
		return s, ErrPrivate
	}
	projections := make([]string, len(structure.Columns))
	for i, col := range structure.Columns {
		projections[i] = "CAST(" + quote(*col[0]) + " AS BINARY)"
	}
	query := "SELECT " + strings.Join(projections, ",") + " FROM " + quote(s.Name) + " FORCE INDEX (PRIMARY)"
	var args []any
	if !s.Boundary.Empty {
		query += " WHERE " + quote(*structure.Columns[0][0]) + " <= ?"
		args = append(args, string(upper))
	}
	query += " ORDER BY " + quote(*structure.Columns[0][0])
	rows, e := db.QueryContext(ctx, query, args...)
	if e != nil {
		return s, ErrRead
	}
	defer rows.Close()
	h := sha256.New()
	frame(h, []byte(jsonSHA(structure.Columns)), false)
	var last []byte
	var lastID uint64
	s.Records, s.Bytes, s.Complete = 0, 0, false
	for rows.Next() {
		if ctx.Err() != nil {
			return s, ErrBudget
		}
		raw := make([]sql.RawBytes, len(structure.Columns))
		scan := make([]any, len(raw))
		for i := range raw {
			scan[i] = &raw[i]
		}
		if rows.Scan(scan...) != nil {
			return s, ErrRead
		}
		if len(raw[0]) == 0 || s.Boundary.Empty {
			return s, ErrContent
		}
		if index == 0 {
			id, e := strconv.ParseUint(string(raw[0]), 10, 64)
			if e != nil || id == 0 || strconv.FormatUint(id, 10) != string(raw[0]) || id <= lastID {
				return s, ErrContent
			}
			lastID = id
		} else if last != nil && bytes.Compare(raw[0], last) <= 0 {
			return s, ErrContent
		}
		last = append(last[:0], raw[0]...)
		line := make([]*string, len(raw))
		var size uint64
		for i, cell := range raw {
			if cell == nil {
				if *structure.Columns[i][2] != "YES" {
					return s, ErrContent
				}
			} else {
				encoded := base64.StdEncoding.EncodeToString(cell)
				line[i] = &encoded
				size += uint64(len(cell))
			}
			frame(h, cell, cell == nil)
		}
		if size > retirement.MaxSourceRowBytes || s.Records >= retirement.MaxSourceRecords || size > retirement.MaxSourceBytes-s.Bytes {
			return s, ErrContent
		}
		if enc.Encode(line) != nil {
			return s, ErrPrivate
		}
		s.Records++
		s.Bytes += size
	}
	if rows.Err() != nil || rows.Close() != nil || ctx.Err() != nil {
		return s, ErrRead
	}
	if (!s.Boundary.Empty && !bytes.Equal(last, upper)) || s.Boundary.Empty != (s.Records == 0) {
		return s, ErrContent
	}
	if _, e = storageSQLUpper(ctx, db, index, structure, s); e != nil {
		return s, e
	}
	s.DataHash, s.Complete, s.Pages = hex.EncodeToString(h.Sum(nil)), true, 1
	return s, nil
}
func storageMongoUpper(ctx context.Context, db *mongo.Database, s SourceSnapshot) (bson.RawValue, error) {
	q, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var raw bson.Raw
	e := db.Collection(s.Name).FindOne(q, bson.D{}, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: -1}}).SetHint("_id_").SetCollation(&options.Collation{Locale: "simple"})).Decode(&raw)
	if s.Boundary.Empty {
		if e != mongo.ErrNoDocuments {
			return bson.RawValue{}, ErrContent
		}
		return bson.RawValue{}, nil
	}
	upper, parseErr := base64Raw(s.Boundary.UpperToken)
	approved := bson.Raw(upper)
	es, elementsErr := approved.Elements()
	if e != nil || parseErr != nil || approved.Validate() != nil || elementsErr != nil || len(es) != 1 || es[0].Key() != "_id" {
		return bson.RawValue{}, ErrContent
	}
	id := raw.Lookup("_id")
	want := es[0].Value()
	cmp, compareErr := comparePK(id, want)
	if compareErr != nil || cmp != 0 || pkKind(id) != s.Boundary.PKType {
		return bson.RawValue{}, ErrContent
	}
	return bson.RawValue{Type: id.Type, Value: append([]byte(nil), id.Value...)}, nil
}
func storageMongoRows(ctx context.Context, db *mongo.Database, s SourceSnapshot, out io.Writer) (SourceSnapshot, error) {
	upper, e := storageMongoUpper(ctx, db, s)
	if e != nil {
		return s, e
	}
	filter := bson.D{}
	if !s.Boundary.Empty {
		filter = bson.D{{Key: "_id", Value: bson.D{{Key: "$lte", Value: upper}}}}
	}
	cur, e := db.Collection(s.Name).Find(ctx, filter, options.Find().SetHint("_id_").SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}).SetBatchSize(1024))
	if e != nil {
		return s, ErrRead
	}
	defer cur.Close(ctx)
	h := sha256.New()
	var last bson.RawValue
	s.Records, s.Bytes, s.Complete = 0, 0, false
	for cur.Next(ctx) {
		if ctx.Err() != nil {
			return s, ErrBudget
		}
		raw := cur.Current
		if raw.Validate() != nil || len(raw) > 16<<20 {
			return s, ErrContent
		}
		es, e := raw.Elements()
		if e != nil {
			return s, ErrContent
		}
		seen := map[string]bool{}
		for _, element := range es {
			if seen[element.Key()] || !utf8.ValidString(element.Key()) {
				return s, ErrContent
			}
			seen[element.Key()] = true
		}
		id := raw.Lookup("_id")
		cmp, compareErr := comparePK(id, upper)
		if s.Boundary.Empty || pkKind(id) != s.Boundary.PKType || compareErr != nil || cmp > 0 {
			return s, ErrContent
		}
		if s.Records != 0 {
			cmp, e = comparePK(last, id)
			if e != nil || cmp >= 0 {
				return s, ErrContent
			}
		}
		last = bson.RawValue{Type: id.Type, Value: append([]byte(nil), id.Value...)}
		if s.Records >= retirement.MaxSourceRecords || uint64(len(raw)) > retirement.MaxSourceBytes-s.Bytes {
			return s, ErrContent
		}
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
		if _, e = out.Write(length[:]); e != nil {
			return s, ErrPrivate
		}
		if _, e = out.Write(raw); e != nil {
			return s, ErrPrivate
		}
		frame(h, raw, false)
		s.Records++
		s.Bytes += uint64(len(raw))
	}
	if cur.Err() != nil || cur.Close(ctx) != nil || ctx.Err() != nil {
		return s, ErrRead
	}
	if s.Boundary.Empty != (s.Records == 0) {
		return s, ErrContent
	}
	if s.Records != 0 {
		cmp, e := comparePK(last, upper)
		if e != nil || cmp != 0 {
			return s, ErrContent
		}
	}
	if _, e = storageMongoUpper(ctx, db, s); e != nil {
		return s, e
	}
	s.DataHash, s.Complete, s.Pages = hex.EncodeToString(h.Sum(nil)), true, 1
	return s, nil
}
func captureStorageFile(ctx context.Context, dir string, index int, read func(io.Writer) (SourceSnapshot, error)) (SourceSnapshot, Asset, error) {
	asset := Asset{Filename: sourceNames[index]}
	f, e := newPrivateFile(filepath.Join(dir, asset.Filename))
	if e != nil {
		return SourceSnapshot{}, asset, e
	}
	h := newCountHash()
	writer := bufio.NewWriterSize(io.MultiWriter(f, h), 1<<20)
	s, e := read(writer)
	flushErr, syncErr, closeErr := writer.Flush(), f.Sync(), f.Close()
	if e != nil {
		return s, asset, e
	}
	if flushErr != nil || syncErr != nil || closeErr != nil || syncDirectory(dir) != nil || ctx.Err() != nil {
		return s, asset, ErrPrivate
	}
	asset.SHA256, asset.Bytes = h.digest(), h.count
	return s, asset, nil
}
func captureStorageSQL(ctx context.Context, db sqlReader, dir string, index int, structure SQLStructure, s SourceSnapshot) (SourceSnapshot, Asset, error) {
	return captureStorageFile(ctx, dir, index, func(out io.Writer) (SourceSnapshot, error) { return storageSQLRows(ctx, db, index, structure, s, out) })
}
func captureStorageMongo(ctx context.Context, db *mongo.Database, dir string, s SourceSnapshot) (SourceSnapshot, Asset, error) {
	return captureStorageFile(ctx, dir, 3, func(out io.Writer) (SourceSnapshot, error) { return storageMongoRows(ctx, db, s, out) })
}
