package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// The fixed scope is the modern personnel/store facts affected by the retired
// maintenance tools. This is deliberately not a whole-database inventory.
var retainedScope = [...]tableSpec{
	{"operators", []column{{"id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"user_id", "bigint", false}, {"roles", "json", false}, {"authz_policy_version", "bigint", false}, {"authz_projected_at", "datetime(3)", true}, {"authz_projection_pending", "tinyint(1)", false}, {"name", "varchar(100)", false}, {"email", "varchar(255)", true}, {"phone", "varchar(20)", true}, {"is_active", "tinyint(1)", false}, {"created_at", "datetime", false}, {"updated_at", "datetime", false}, {"deleted_at", "datetime", true}, {"created_by", "bigint unsigned", false}, {"updated_by", "bigint unsigned", false}, {"deleted_by", "bigint unsigned", false}, {"version", "int unsigned", false}}, []string{"id"}, false},
	{"testee", []column{{"id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"profile_id", "bigint unsigned", true}, {"name", "varchar(100)", false}, {"gender", "tinyint", false}, {"birthday", "date", true}, {"source", "varchar(50)", false}, {"is_key_focus", "tinyint(1)", false}, {"total_assessments", "int", false}, {"last_assessment_at", "datetime", true}, {"last_risk_level", "varchar(50)", true}, {"created_at", "datetime", false}, {"updated_at", "datetime", false}, {"deleted_at", "datetime", true}, {"created_by", "bigint unsigned", false}, {"updated_by", "bigint unsigned", false}, {"deleted_by", "bigint unsigned", false}, {"version", "int unsigned", false}, {"store_id", "bigint unsigned", true}, {"store_version", "int unsigned", false}}, []string{"id"}, false},
	{"clinician", []column{{"id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"name", "varchar(100)", false}, {"department", "varchar(100)", true}, {"title", "varchar(100)", true}, {"clinician_type", "varchar(50)", false}, {"employee_code", "varchar(50)", true}, {"is_active", "tinyint(1)", false}, {"created_at", "datetime", false}, {"updated_at", "datetime", false}, {"deleted_at", "datetime", true}, {"created_by", "bigint unsigned", false}, {"updated_by", "bigint unsigned", false}, {"deleted_by", "bigint unsigned", false}, {"version", "int unsigned", false}, {"store_id", "bigint unsigned", true}}, []string{"id"}, false},
	{"operator_retirement_tasks", []column{{"operator_id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"user_id", "bigint", false}, {"stage", "varchar(16)", false}, {"payload", "json", false}, {"before_state", "json", false}, {"updated_at", "datetime(3)", false}}, []string{"operator_id"}, false},
	{"operator_recovery_archives", []column{{"request_id", "varchar(64)", false}, {"operator_id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"user_id", "bigint", false}, {"retirement_request_id", "varchar(64)", false}, {"recovery_payload", "longtext", false}, {"retirement_payload", "longtext", false}, {"retirement_before_state", "longtext", false}, {"operator_before_state", "longtext", false}, {"checksum", "char(64)", false}, {"created_at", "datetime(3)", false}}, []string{"request_id"}, true},
	{"actor_stores", []column{{"id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"code", "varchar(32)", false}, {"name", "varchar(100)", false}, {"address", "varchar(255)", false}, {"is_active", "tinyint(1)", false}, {"version", "int unsigned", false}, {"created_at", "datetime(6)", false}, {"updated_at", "datetime(6)", false}, {"created_by", "bigint", false}, {"updated_by", "bigint", false}}, []string{"id"}, false},
	{"clinician_store_history", []column{{"id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"clinician_id", "bigint unsigned", false}, {"from_store_id", "bigint unsigned", true}, {"to_store_id", "bigint unsigned", false}, {"kind", "varchar(16)", false}, {"actor_id", "bigint", false}, {"created_at", "datetime(6)", false}, {"reason", "varchar(500)", false}, {"request_id", "varchar(64)", false}, {"invalidated_count", "bigint", false}, {"version", "int unsigned", false}}, []string{"id"}, false},
	{"testee_store_history", []column{{"id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"testee_id", "bigint unsigned", false}, {"from_store_id", "bigint unsigned", true}, {"to_store_id", "bigint unsigned", false}, {"kind", "varchar(24)", false}, {"actor_id", "bigint", false}, {"entry_id", "bigint unsigned", true}, {"clinician_id", "bigint unsigned", true}, {"created_at", "datetime(6)", false}, {"reason", "varchar(500)", false}, {"request_id", "varchar(64)", false}, {"version", "int unsigned", false}}, []string{"id"}, false},
}

type retainedRowProof struct {
	Key  []string `json:"primary_key"`
	Hash string   `json:"server_row_sha256"`
}
type retainedTableProof struct {
	Table            string             `json:"table"`
	DDL              string             `json:"original_ddl"`
	Columns          []column           `json:"columns"`
	Keys             []string           `json:"primary_key_columns"`
	Rows             []retainedRowProof `json:"rows"`
	Pages            int                `json:"pages_including_empty_eof"`
	EOF              bool               `json:"mandatory_empty_eof"`
	EncodedRowsBytes int                `json:"encoded_rows_bytes"`
}

func validateRetained(tables [8]retainedTableProof, totalRows, totalBytes *int) error {
	for i, t := range tables {
		s := retainedScope[i]
		if t.Table != s.name || t.DDL == "" || len(t.DDL) > maxDDLBytes || !sameColumns(t.Columns, s.columns) || !sameStrings(t.Keys, s.keys) || !t.EOF || len(t.Rows) > maxRows || t.Pages != (len(t.Rows)+pageSize-1)/pageSize+1 {
			return errors.New("retained_material_shape_or_eof_rejected")
		}
		used := 0
		for n, row := range t.Rows {
			if !validKey(s, row.Key) || !nonzeroHash(row.Hash) || n > 0 && !keyLess(s, t.Rows[n-1].Key, row.Key) {
				return errors.New("retained_material_row_rejected")
			}
			raw, e := json.Marshal(row)
			if e != nil || accountRow(len(raw)+1, n, &used, totalRows, totalBytes) != nil {
				return errors.New("retained_material_limit_rejected")
			}
		}
		if used != t.EncodedRowsBytes {
			return errors.New("retained_material_length_rejected")
		}
	}
	return nil
}

func retainedColumns(ctx context.Context, r sqlReader, s tableSpec) ([]column, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,GENERATION_EXPRESSION FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", s.name)
	if e != nil {
		return nil, errors.New("retained_columns_unavailable")
	}
	var got []column
	for rows.Next() {
		var c column
		var nullable, generated string
		if rows.Scan(&c.Name, &c.Type, &nullable, &generated) != nil || nullable != "YES" && nullable != "NO" || generated != "" || len(got) >= len(s.columns) {
			_ = rows.Close()
			return nil, errors.New("retained_columns_rejected")
		}
		c.Nullable = nullable == "YES"
		got = append(got, c)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !sameColumns(got, s.columns) {
		return nil, errors.New("retained_columns_or_release_rejected")
	}
	if e := requirePrimaryKey(ctx, r, s); e != nil {
		return nil, e
	}
	return got, nil
}

func retainedPageSQL(s tableSpec, hasLast bool) string {
	key := "`" + s.keys[0] + "`"
	expr, order, where := "CAST("+key+" AS CHAR)", key, "1=1"
	if s.binaryFirst {
		expr, order = "HEX(CAST("+key+" AS BINARY))", "CAST("+key+" AS BINARY)"
		if hasLast {
			where = order + ">UNHEX(?)"
		}
	} else if hasLast {
		where = key + ">CAST(? AS DECIMAL(20,0))"
	}
	var lengths, allowed []string
	for _, c := range s.columns {
		length := "COALESCE(OCTET_LENGTH(CAST(`" + c.Name + "` AS BINARY)),0)"
		lengths = append(lengths, length)
		allowed = append(allowed, length+"<="+strconv.Itoa(maxValueBytes))
	}
	allowed = append(allowed, "("+strings.Join(lengths, "+")+")<="+strconv.Itoa(maxRowBytes))
	return "SELECT /*+ MAX_EXECUTION_TIME(5000) */ " + expr + ",CASE WHEN " + strings.Join(allowed, " AND ") + " THEN " + retainedHashSQL(s.columns) + " ELSE NULL END FROM `" + s.name + "` FORCE INDEX(PRIMARY) WHERE " + where + " ORDER BY " + order + " LIMIT " + strconv.Itoa(pageSize)
}

func scanRetained(ctx context.Context, r sqlReader, i int, totalRows, totalBytes *int) (retainedTableProof, error) {
	s := retainedScope[i]
	out := retainedTableProof{Table: s.name, Columns: s.columns, Keys: s.keys}
	var e error
	if _, e = retainedColumns(ctx, r, s); e != nil {
		return out, e
	}
	if out.DDL, e = ddl(ctx, r, s.name); e != nil {
		return out, e
	}
	var last []string
	for {
		q, cancel := context.WithTimeout(ctx, queryLimit)
		rows, e := r.QueryContext(q, retainedPageSQL(s, last != nil), pageArgs(s, last)...)
		if e != nil {
			cancel()
			return out, errors.New("retained_page_unavailable")
		}
		count := 0
		for rows.Next() {
			row := retainedRowProof{Key: make([]string, 1)}
			var hash sql.NullString
			if rows.Scan(&row.Key[0], &hash) != nil || !hash.Valid || !nonzeroHash(hash.String) || !validKey(s, row.Key) || last != nil && !keyLess(s, last, row.Key) {
				_ = rows.Close()
				cancel()
				return out, errors.New("retained_row_limit_order_or_hash_rejected")
			}
			row.Hash = hash.String
			raw, e := json.Marshal(row)
			if e != nil || accountRow(len(raw)+1, len(out.Rows), &out.EncodedRowsBytes, totalRows, totalBytes) != nil {
				_ = rows.Close()
				cancel()
				return out, errors.New("retained_material_limit_before_eof")
			}
			out.Rows = append(out.Rows, row)
			last = row.Key
			count++
		}
		re, ce := rows.Err(), rows.Close()
		cancel()
		if re != nil || ce != nil {
			return out, errors.New("retained_page_release_unproven")
		}
		out.Pages++
		if count == 0 {
			out.EOF = true
			break
		}
	}
	if e := completeRangeCount(ctx, r, s.name, uint64(len(out.Rows))); e != nil {
		return out, e
	}
	return out, nil
}

func sameRetained(a, b [8]retainedTableProof) bool { return reflect.DeepEqual(a, b) }
