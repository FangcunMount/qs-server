//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/go-sql-driver/mysql"
)

type collationShape struct {
	Table              string `json:"table"`
	RequestedCollation string `json:"requested_collation"`
	ActualCollation    string `json:"actual_table_collation"`
	ActualFile         string `json:"actual_file"`
	ActualSHA          string `json:"actual_ddl_sha256"`
	ActualBytes        int    `json:"actual_ddl_bytes"`
	ExpectedFile       string `json:"expected_file"`
	ExpectedSHA        string `json:"expected_ddl_sha256"`
	ExpectedBytes      int    `json:"expected_ddl_bytes"`
	Comparison         string `json:"comparison_only_not_restore_eligibility"`
}

type collationShapeResult struct {
	Kind                   string           `json:"kind"`
	ToolSource             string           `json:"compiled_source"`
	SQLVersion             string           `json:"actual_mysql_version"`
	SQLIdentityHash        string           `json:"synthetic_mysql_server_uuid_sha256"`
	Original075Hash        string           `json:"original075_sha256"`
	Original078Hash        string           `json:"original078_sha256"`
	Shapes                 []collationShape `json:"shapes"`
	MetadataComplete       bool             `json:"all_15_metadata_reads_complete"`
	SchemasDropped         bool             `json:"owned_schemas_dropped"`
	SQLConnectionClosed    bool             `json:"sql_connection_closed"`
	SQLPoolClosed          bool             `json:"sql_pool_closed"`
	PrivateDirectoryClosed bool             `json:"private_directory_closed"`
	ResultFileHash         string           `json:"result_file_sha256"`
	Complete               bool             `json:"diagnostic_complete"`
	RestoreQualified       bool             `json:"restore_qualified"`
	ErrorStage             string           `json:"error_stage"`
	ErrorMetadata          json.RawMessage  `json:"safe_error_metadata,omitempty"`
}

// This name deliberately does not match ^TestM4MaintenanceTablesNative. It
// observes empty synthetic schemas only and never calls capture or restore.
// A shape mismatch is evidence to save, not permission to change the qualifier.
func TestMaintenanceOriginalCollationShapeDiagnostic(t *testing.T) {
	if os.Getenv("QS_M4_D_OBJECT_NATIVE_REQUIRED") != "1" || os.Getenv("QS_M4_D_OBJECT_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" || !sourceRE.MatchString(version.GitCommit) || !originalResourceBytesValid() {
		t.Fatal("owned diagnostic prerequisites required")
	}
	t.Logf("actual compiled D shape tool=%s phase=before_dependency_binding", version.GitCommit)
	dir := os.Getenv("QS_M4_D_COLLATION_SHAPE_OUTPUT_DIR")
	resultPath := filepath.Join(dir, "shape-result.json")
	if !absolutePath(resultPath) {
		t.Fatal("private diagnostic output required")
	}
	parent, e := captureParent(resultPath)
	if e != nil {
		t.Fatal("private diagnostic directory unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	out := collationShapeResult{Kind: "m4_d_original_closed_collation_shape_diagnostic", ToolSource: version.GitCommit, Original075Hash: fingerprint(original075), Original078Hash: fingerprint(original078), ErrorStage: "not_started"}
	var pool *sql.DB
	var conn *sql.Conn
	var schemas []string
	defer func() {
		// Cleanup has its own short allowance; it does not extend metadata-read
		// eligibility or turn a failed read into a completed diagnostic.
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if conn != nil {
			out.SQLConnectionClosed = conn.Close() == nil
		}
		if pool != nil {
			out.SchemasDropped = true
			for _, name := range schemas {
				q, done := context.WithTimeout(cleanup, queryLimit)
				_, dropErr := pool.ExecContext(q, "DROP DATABASE IF EXISTS `"+name+"`")
				done()
				if dropErr != nil {
					out.SchemasDropped = false
				}
			}
			closed := pool.Close() == nil
			stats := pool.Stats()
			out.SQLPoolClosed = closed && stats.OpenConnections == 0 && stats.InUse == 0
		}
		// No body goes to stdout. The result file and both full DDL files per
		// shape remain private; the final log binds their hashes and real releases.
		out.MetadataComplete = len(out.Shapes) == len(scope)*len(originalCollations) && out.ErrorStage == "none"
		// This file is metadata captured before the held output-directory FD
		// closes. Only the final test log below attests that last release; there
		// is no self-referential file hash or prematurely completed receipt.
		raw, marshalErr := json.Marshal(struct {
			Kind             string           `json:"kind"`
			ToolSource       string           `json:"compiled_source"`
			SQLVersion       string           `json:"actual_mysql_version"`
			SQLIdentityHash  string           `json:"synthetic_mysql_server_uuid_sha256"`
			Original075Hash  string           `json:"original075_sha256"`
			Original078Hash  string           `json:"original078_sha256"`
			Shapes           []collationShape `json:"shapes"`
			MetadataComplete bool             `json:"all_15_metadata_reads_complete"`
			ErrorStage       string           `json:"error_stage"`
			ErrorMetadata    json.RawMessage  `json:"safe_error_metadata,omitempty"`
		}{"m4_d_saved_shape_metadata_before_directory_close", out.ToolSource, out.SQLVersion, out.SQLIdentityHash, out.Original075Hash, out.Original078Hash, out.Shapes, out.MetadataComplete, out.ErrorStage, out.ErrorMetadata})
		proof := receipt{}
		writeErr := marshalErr
		if writeErr == nil {
			writeErr = finishCapture(ctx, &proof, resultPath, &parent, raw, writeCaptureExclusive)
		}
		closeErr := parent.close()
		out.PrivateDirectoryClosed = closeErr == nil && proof.ReleaseError == ""
		out.ResultFileHash = proof.MaterialHash
		out.Complete = out.MetadataComplete && out.SchemasDropped && out.SQLConnectionClosed && out.SQLPoolClosed && out.PrivateDirectoryClosed && writeErr == nil && ctx.Err() == nil
		if !out.Complete && out.ErrorStage == "none" {
			out.ErrorStage = "metadata_or_release_incomplete"
		}
		final, encodeErr := json.Marshal(out)
		if encodeErr != nil {
			t.Error("safe diagnostic final receipt encoding failed")
		} else {
			t.Logf("owned_shape_receipt=%s", final)
		}
		if !out.Complete {
			t.Error("owned shape diagnostic incomplete; no restore qualification")
		}
	}()
	fail := func(stage string, cause error) {
		out.ErrorStage = stage
		out.ErrorMetadata = json.RawMessage(fixtureErrorJSON("unknown", cause))
	}
	cfg, e := mysql.ParseDSN(os.Getenv("QS_M4_D_OBJECT_NATIVE_MYSQL_DSN"))
	if e != nil || cfg.Net != "tcp" || cfg.DBName != "" || cfg.TLSConfig != "" {
		fail("owned_loopback_input_rejected", errors.New("unbound input"))
		return
	}
	host, port, e := net.SplitHostPort(cfg.Addr)
	n, pe := strconv.Atoi(port)
	if e != nil || pe != nil || host != "127.0.0.1" || n <= 1024 || n > 65535 || port == "3306" || strconv.Itoa(n) != port {
		fail("owned_loopback_input_rejected", errors.New("unbound endpoint"))
		return
	}
	cfg.MultiStatements, cfg.ParseTime = true, false
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = queryLimit, queryLimit, queryLimit
	if e := mysql.SetLogger(&mysql.NopLogger{}); e != nil {
		fail("private_driver_logging_boundary_failed", e)
		return
	}
	pool, e = sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		fail("owned_pool_open_failed", e)
		return
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	q, done := context.WithTimeout(ctx, queryLimit)
	conn, e = pool.Conn(q)
	done()
	if e != nil {
		fail("owned_connection_unavailable", e)
		return
	}
	var uuid string
	q, done = context.WithTimeout(ctx, queryLimit)
	e = conn.QueryRowContext(q, "SELECT VERSION(),@@server_uuid").Scan(&out.SQLVersion, &uuid)
	done()
	if e != nil || out.SQLVersion != "8.0.36" || !uuidRE.MatchString(uuid) {
		fail("actual_mysql_identity_rejected", e)
		return
	}
	out.SQLIdentityHash = fingerprint([]byte(uuid))
	t.Logf("actual D collation shape tool=%s mysql=%s resources075=%s resources078=%s", out.ToolSource, out.SQLVersion, out.Original075Hash, out.Original078Hash)
	for index, collation := range originalCollations {
		name := fmt.Sprintf("qs_m4_d_shape_%d_%d", time.Now().UnixNano(), index)
		if !nameRE.MatchString(name) {
			fail("owned_schema_name_rejected", nil)
			return
		}
		schemas = append(schemas, name)
		exec := func(statement string) error {
			query, finish := context.WithTimeout(ctx, queryLimit)
			defer finish()
			_, err := conn.ExecContext(query, statement)
			return err
		}
		if e := exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); e != nil {
			fail("owned_schema_create_failed", e)
			return
		}
		if e := exec("USE `" + name + "`"); e != nil {
			fail("owned_schema_selection_failed", e)
			return
		}
		for _, resource := range [][]byte{original075, original078} {
			if e := exec(string(resource)); e != nil {
				fail("fixed_original_075_078_create_failed", e)
				return
			}
		}
		for i, spec := range scope {
			if e := exec("ALTER TABLE `" + spec.name + "` CONVERT TO CHARACTER SET utf8mb4 COLLATE " + collation); e != nil {
				fail("fixed_collation_convert_failed", e)
				return
			}
			var selected, actual, actualCollation string
			q, done := context.WithTimeout(ctx, queryLimit)
			e := conn.QueryRowContext(q, "SHOW CREATE TABLE `"+spec.name+"`").Scan(&selected, &actual)
			done()
			if e != nil || selected != spec.name || len(actual) < 1 || len(actual) > maxDDLBytes {
				fail("fixed_show_create_unavailable_or_over_limit", e)
				return
			}
			q, done = context.WithTimeout(ctx, queryLimit)
			e = conn.QueryRowContext(q, "SELECT TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", spec.name).Scan(&actualCollation)
			done()
			if e != nil || actualCollation != collation {
				fail("actual_collation_metadata_changed", e)
				return
			}
			expected := originalDDL(i, collation)
			comparison := "other_unclassified"
			if actual == expected {
				comparison = "exact_current_template"
			} else if actual == strings.Replace(expected, " COLLATE="+collation, "", 1) {
				comparison = "exact_except_table_tail_collate_omitted"
			}
			actualFile := fmt.Sprintf("%s-%02d-actual.sql", collation, i)
			expectedFile := fmt.Sprintf("%s-%02d-expected.sql", collation, i)
			for file, body := range map[string]string{actualFile: actual, expectedFile: expected} {
				path := filepath.Join(dir, file)
				if checkCaptureParent(path, &parent) != nil || writeCaptureExclusive(&parent, path, []byte(body)) != nil || checkCaptureParent(path, &parent) != nil || ctx.Err() != nil {
					fail("private_shape_file_persistence_unproven", nil)
					return
				}
			}
			out.Shapes = append(out.Shapes, collationShape{spec.name, collation, actualCollation, actualFile, fingerprint([]byte(actual)), len(actual), expectedFile, fingerprint([]byte(expected)), len(expected), comparison})
		}
	}
	q, done = context.WithTimeout(ctx, queryLimit)
	var afterUUID, afterVersion string
	e = conn.QueryRowContext(q, "SELECT VERSION(),@@server_uuid").Scan(&afterVersion, &afterUUID)
	done()
	if e != nil || afterUUID != uuid || afterVersion != out.SQLVersion {
		fail("actual_mysql_identity_changed", e)
		return
	}
	out.ErrorStage = "none"
}
