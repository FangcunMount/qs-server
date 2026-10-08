package compatibilityretirementbackup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	r, e := newRawReader(ctx, f, index, a.data.Inventory.Targets[index], columns)
	if e != nil {
		_ = f.Close()
		return nil, nil, e
	}
	return f, r, nil
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
