package retirement

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

var sourceOriginAutoIncrement = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)

func sourceOriginQuote(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func sourceOriginSQLQuery(ctx context.Context, tx *gorm.DB, limits SourceOriginLimits, query string, args ...any) (SQLColumns, error) {
	q, cancel := context.WithTimeout(ctx, limits.QueryTimeout)
	defer cancel()
	rows, e := tx.WithContext(q).Raw(query, args...).Rows()
	if e != nil {
		return nil, ErrSourceOrigin
	}
	defer func() { _ = rows.Close() }()
	var total uint64
	cols, e := rows.Columns()
	if e != nil || len(cols) > 64 {
		return nil, ErrSourceSchema
	}
	out := SQLColumns{}
	for rows.Next() {
		if len(out) >= 4096 {
			return nil, ErrSourceBounds
		}
		v := make([]sql.RawBytes, len(cols))
		dst := make([]any, len(v))
		for i := range v {
			dst[i] = &v[i]
		}
		if rows.Scan(dst...) != nil {
			return nil, ErrSourceOrigin
		}
		var rowBytes uint64
		for _, value := range v {
			rowBytes += uint64(len(value))
		}
		if rowBytes > MaxSourceRowBytes || total+rowBytes > 64<<20 {
			return nil, ErrSourceBounds
		}
		total += rowBytes
		r := make([]*string, len(v))
		for i, value := range v {
			if value != nil {
				s := string(value)
				r[i] = &s
			}
		}
		out = append(out, r)
	}
	if rows.Err() != nil || rows.Close() != nil {
		return nil, ErrSourceOrigin
	}
	return out, nil
}
func sourceOriginSQLValue(row []*string, i int) string {
	if i >= len(row) || row[i] == nil {
		return ""
	}
	return *row[i]
}
func sourceOriginSQLHead(ctx context.Context, tx *gorm.DB, l SourceOriginLimits) (string, error) {
	rows, e := sourceOriginSQLQuery(ctx, tx, l, "SELECT CAST(version AS BINARY),CAST(dirty AS BINARY) FROM schema_migrations ORDER BY version")
	if e != nil || len(rows) != 1 || sourceOriginSQLValue(rows[0], 1) != "0" {
		return "", ErrSourceOrigin
	}
	version, e := strconv.ParseUint(sourceOriginSQLValue(rows[0], 0), 10, 64)
	if e != nil || version == 0 {
		return "", ErrSourceOrigin
	}
	return sourceOriginDigest(rows)
}
func sourceOriginSQLMetadata(ctx context.Context, tx *gorm.DB, expected SourceCopyExpectation, l SourceOriginLimits) (SourceBoundary, SQLColumns, string, error) {
	name := expected.Boundary.Name
	if name != "domain_event_outbox" && name != AIBridgeCommandSource && name != AILegacyCommandSource {
		return SourceBoundary{}, nil, "", ErrSourceSchema
	}
	b := SourceBoundary{Database: "mysql", Name: name, Kind: "base_table", Present: true}
	kinds, e := sourceOriginSQLQuery(ctx, tx, l, "SELECT TABLE_TYPE FROM information_schema.tables WHERE TABLE_SCHEMA=DATABASE() AND BINARY TABLE_NAME=BINARY ?", name)
	if e != nil || len(kinds) != 1 || sourceOriginSQLValue(kinds[0], 0) != "BASE TABLE" {
		return b, nil, "", ErrSourceSchema
	}
	create, e := sourceOriginSQLQuery(ctx, tx, l, "SHOW CREATE TABLE "+sourceOriginQuote(name))
	if e != nil || len(create) != 1 || len(create[0]) != 2 || create[0][1] == nil {
		return b, nil, "", ErrSourceSchema
	}
	normalized := sourceOriginAutoIncrement.ReplaceAllString(*create[0][1], "")
	create[0][1] = &normalized
	b.SchemaHash, e = sourceOriginDigest(create)
	if e != nil {
		return b, nil, "", e
	}
	b.IdentityHash = mongoOwnerHashParts("mysql-object-v1", name, b.SchemaHash)
	cols, e := sourceOriginSQLQuery(ctx, tx, l, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE TABLE_SCHEMA=DATABASE() AND BINARY TABLE_NAME=BINARY ? ORDER BY ORDINAL_POSITION", name)
	if e != nil {
		return b, nil, "", e
	}
	pk, e := sourceOriginSQLQuery(ctx, tx, l, "SELECT c.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE,c.CHARACTER_SET_NAME,c.COLLATION_NAME FROM information_schema.statistics s JOIN information_schema.columns c ON c.TABLE_SCHEMA=s.TABLE_SCHEMA AND c.TABLE_NAME=s.TABLE_NAME AND c.COLUMN_NAME=s.COLUMN_NAME WHERE s.TABLE_SCHEMA=DATABASE() AND BINARY s.TABLE_NAME=BINARY ? AND s.INDEX_NAME='PRIMARY' ORDER BY s.SEQ_IN_INDEX", name)
	if e != nil || len(pk) != 1 {
		return b, nil, "", ErrSourceSchema
	}
	key := "command_id"
	b.PKType = "ascii_string"
	if name == "domain_event_outbox" {
		key = "id"
		b.PKType = "uint64"
		if sourceOriginSQLValue(pk[0], 1) != "bigint" || sourceOriginSQLValue(pk[0], 2) != "bigint unsigned" || len(cols) != len(sqlSourceColumns) {
			return b, nil, "", ErrSourceSchema
		}
		for i, c := range cols {
			want := sqlSourceColumns[i]
			if len(c) != 6 || sourceOriginSQLValue(c, 0) != want.name || sourceOriginSQLValue(c, 1) != want.kind || sourceOriginSQLValue(c, 2) != want.nullable || c[4] == nil {
				return b, nil, "", ErrSourceSchema
			}
		}
	} else if e = aiValidateColumns(name, cols); e != nil {
		return b, nil, "", e
	} else if sourceOriginSQLValue(pk[0], 3) != "ascii" || sourceOriginSQLValue(pk[0], 4) != "ascii_bin" {
		return b, nil, "", ErrSourceSchema
	}
	if sourceOriginSQLValue(pk[0], 0) != key {
		return b, nil, "", ErrSourceSchema
	}
	upper, e := sourceOriginSQLQuery(ctx, tx, l, "SELECT CAST("+sourceOriginQuote(key)+" AS BINARY) FROM "+sourceOriginQuote(name)+" FORCE INDEX (PRIMARY) ORDER BY "+sourceOriginQuote(key)+" DESC LIMIT 1")
	if e != nil {
		return b, nil, "", e
	}
	b.Empty = len(upper) == 0
	if len(upper) > 1 {
		return b, nil, "", ErrSourceSchema
	}
	if !b.Empty {
		if len(upper[0]) != 1 || upper[0][0] == nil {
			return b, nil, "", ErrSourceSchema
		}
		raw := []byte(*upper[0][0])
		if b.PKType == "uint64" {
			if _, e = positiveSQLID(raw); e != nil {
				return b, nil, "", e
			}
		} else if !aiOriginalUUID(string(raw)) {
			return b, nil, "", ErrSourceIdentity
		}
		b.UpperToken = base64.StdEncoding.EncodeToString(raw)
	}
	return b, cols, key, nil
}
func sourceOriginSQLToken(b SourceBoundary) (any, error) {
	raw, e := canonicalBase64(b.UpperToken)
	if e != nil {
		return nil, e
	}
	if b.PKType == "uint64" {
		return positiveSQLID(raw)
	}
	if !aiOriginalUUID(string(raw)) {
		return nil, ErrSourceIdentity
	}
	return string(raw), nil
}
func sourceOriginReadSQL(ctx context.Context, tx *gorm.DB, binding *OriginCopyBinding, index int) (SourceBoundary, SourceCopyReceipt, error) {
	return sourceOriginReadSQLWithInput(ctx, tx, binding, index, binding.alive, nil)
}

func sourceOriginReadSQLWithInput(ctx context.Context, tx *gorm.DB, binding *OriginCopyBinding, index int, guard func(context.Context) error, freeze sourceOriginInputSink) (SourceBoundary, SourceCopyReceipt, error) {
	if binding == nil || guard == nil || index < 0 || index > 2 || guard(ctx) != nil {
		return SourceBoundary{}, SourceCopyReceipt{}, ErrSourceOrigin
	}
	expected := binding.expected[index]
	boundary, cols, key, e := sourceOriginSQLMetadata(ctx, tx, expected, binding.limits)
	if e != nil || boundary != expected.Boundary {
		return boundary, SourceCopyReceipt{}, ErrSourceOrigin
	}
	columnsRaw, e := json.Marshal(cols)
	if e != nil {
		return boundary, SourceCopyReceipt{}, ErrSourceSchema
	}
	columnHash := sourceSHA(columnsRaw)
	acc := sourceAccumulator{expectation: expected, h: sha256.New()}
	sourceFrame(acc.h, []byte(columnHash), false)
	d := &SQLSourceReader{acc: acc, columns: cols, columnsHash: columnHash}
	ai := &AISQLSourceReader{acc: acc, columns: cols, columnsHash: columnHash}
	if !boundary.Empty {
		raw, _ := canonicalBase64(boundary.UpperToken)
		if index == 0 {
			d.upper, e = positiveSQLID(raw)
		} else {
			ai.upper = string(raw)
		}
		if e != nil {
			return boundary, SourceCopyReceipt{}, e
		}
	}
	receipt := func() SourceCopyReceipt {
		if index == 0 {
			return d.Receipt()
		}
		return ai.Receipt()
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = "CAST(" + sourceOriginQuote(*c[0]) + " AS BINARY)"
	}
	if !boundary.Empty {
		upper, e := sourceOriginSQLToken(boundary)
		if e != nil {
			return boundary, SourceCopyReceipt{}, e
		}
		last := ""
		for page := uint64(0); ; page++ {
			if e = guard(ctx); e != nil {
				return boundary, SourceCopyReceipt{}, e
			}
			if page > binding.limits.MaxRows/uint64(binding.limits.PageRows)+1 {
				return boundary, SourceCopyReceipt{}, ErrSourceBounds
			}
			predicate := sourceOriginQuote(key) + " <= ?"
			args := []any{upper}
			if last != "" {
				next := boundary
				next.UpperToken = last
				token, e := sourceOriginSQLToken(next)
				if e != nil {
					return boundary, SourceCopyReceipt{}, e
				}
				predicate += " AND " + sourceOriginQuote(key) + " > ?"
				args = append(args, token)
			}
			args = append(args, binding.limits.PageRows)
			rows, e := sourceOriginSQLQuery(ctx, tx, binding.limits, "SELECT "+strings.Join(names, ",")+" FROM "+sourceOriginQuote(boundary.Name)+" FORCE INDEX (PRIMARY) WHERE "+predicate+" ORDER BY "+sourceOriginQuote(key)+" LIMIT ?", args...)
			if e != nil {
				return boundary, SourceCopyReceipt{}, e
			}
			for _, row := range rows {
				if e = guard(ctx); e != nil {
					return boundary, SourceCopyReceipt{}, e
				}
				encoded := make([]*string, len(row))
				for i, c := range row {
					if c != nil {
						v := base64.StdEncoding.EncodeToString([]byte(*c))
						encoded[i] = &v
					}
				}
				line, e := json.Marshal(encoded)
				if e != nil {
					return boundary, SourceCopyReceipt{}, ErrSourceOrigin
				}
				if index == 0 {
					v, e := d.decodeLine(line)
					if e != nil {
						return boundary, SourceCopyReceipt{}, e
					}
					if e = sourceOriginObserve(binding.copies, v, v.Source.Database, v.Source.Object, v.Source.PrimaryKeySHA256); e != nil {
						return boundary, SourceCopyReceipt{}, e
					}
					last = v.PrimaryKeyToken
				} else {
					v, e := ai.decodeLine(line)
					if e != nil {
						return boundary, SourceCopyReceipt{}, e
					}
					if e = sourceOriginObserve(binding.copies, v, v.Source.Database, v.Source.Object, v.Source.PrimaryKeySHA256); e != nil {
						return boundary, SourceCopyReceipt{}, e
					}
					last = base64.StdEncoding.EncodeToString([]byte(v.CommandID))
				}
			}
			if freeze != nil {
				if e = freeze(ctx, sourceOriginInputFrame{Source: index, Boundary: boundary, Columns: cols, SQLRows: rows, EOF: len(rows) < binding.limits.PageRows}); e != nil {
					return boundary, SourceCopyReceipt{}, e
				}
			}
			if len(rows) < binding.limits.PageRows {
				break
			}
		}
	}
	if boundary.Empty && freeze != nil {
		if e = freeze(ctx, sourceOriginInputFrame{Source: index, Boundary: boundary, Columns: cols, EOF: true}); e != nil {
			return boundary, SourceCopyReceipt{}, e
		}
	}
	if index == 0 {
		e = d.acc.finish()
	} else {
		e = ai.acc.finish()
	}
	if e != io.EOF {
		return boundary, SourceCopyReceipt{}, e
	}
	return boundary, receipt(), nil
}
