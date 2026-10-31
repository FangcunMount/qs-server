// Reuse the existing application snapshot without exporting identities or errors.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/mysqltls"
	mysql "github.com/go-sql-driver/mysql"
)

func main() {
	report := map[string]any{"read_only": true, "mutation": false, "available": false}
	defer func() { _ = json.NewEncoder(os.Stdout).Encode(report) }()
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr = "tcp", os.Getenv("QS_APISERVER_MYSQL_HOST")
	cfg.User, cfg.Passwd = os.Getenv("QS_APISERVER_MYSQL_USERNAME"), os.Getenv("QS_APISERVER_MYSQL_PASSWORD")
	cfg.DBName, cfg.ParseTime, cfg.Loc = os.Getenv("QS_APISERVER_MYSQL_DATABASE"), true, time.UTC
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 2*time.Second, 2*time.Second, 2*time.Second
	if cfg.Addr == "" || cfg.User == "" || cfg.DBName == "" || os.Getenv("QS_APISERVER_MYSQL_TLS_MODE") != "VERIFY_IDENTITY" {
		report["category"] = "configuration_unavailable"
		return
	}
	connector, err := mysqltls.NewConnector(cfg)
	if err != nil {
		report["category"] = "tls_configuration"
		return
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	_, err = store.MessagingSnapshot(ctx, db)
	report["elapsed_ms"] = time.Since(started).Milliseconds()
	switch {
	case err == nil:
		report["available"], report["category"] = true, "available"
	case errors.Is(err, store.ErrMessagingLedgerIntegrity):
		report["category"] = "ledger_evidence_integrity"
		classifyEvidence(db, report)
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		report["category"] = "observation_budget_exhausted"
	default:
		var driverError *mysql.MySQLError
		if errors.As(err, &driverError) {
			report["category"], report["mysql_error_number"] = "mysql_error", driverError.Number
		} else {
			report["category"] = "connection_or_query_error"
		}
	}
}

// Count only the evidence categories from the same join and scope as the
// existing collector. A timestamp cohort is not proof of historical completion.
func classifyEvidence(db *sql.DB, report map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		report["classification_available"] = false
		return
	}
	defer func() { _ = tx.Rollback() }()
	var total, noArchive, noExecution, noWitness, noAnchor, executionState, witnessState, reference, recent uint64
	err = tx.QueryRowContext(ctx, `SELECT /*+ MAX_EXECUTION_TIME(1000) */ COUNT(*),
 COALESCE(SUM(a.message_id IS NULL),0),
 COALESCE(SUM(e.execution_key IS NULL),0),
 COALESCE(SUM(w.branch_key IS NULL),0),
 COALESCE(SUM(a.message_id IS NOT NULL AND (a.execution_key IS NULL OR a.branch_key IS NULL)),0),
 COALESCE(SUM(e.execution_key IS NOT NULL AND e.state<>'committed'),0),
 COALESCE(SUM(w.branch_key IS NOT NULL AND w.state<>'committed'),0),
 COALESCE(SUM(w.reference<>a.reference),0),
 COALESCE(SUM(o.created_at>='2026-10-28 00:00:00' AND
 (a.message_id IS NULL OR e.execution_key IS NULL OR w.branch_key IS NULL OR
 e.state<>'committed' OR w.state<>'committed' OR w.reference<>a.reference)),0)
 FROM ai_messaging_operations o
 LEFT JOIN ai_messaging_archive a ON a.producer='qs-server' AND a.destination='qs-ai'
 AND a.message_id=CAST(o.command_id AS BINARY)
 LEFT JOIN rm_tx_execution e ON e.execution_key=a.execution_key AND e.owner='qs.mysql'
 LEFT JOIN rm_tx_witness w ON w.execution_key=a.execution_key AND w.branch_key=a.branch_key
 WHERE o.retired=FALSE AND o.kind IN (1,2,3,4,5)`).Scan(&total, &noArchive, &noExecution, &noWitness, &noAnchor, &executionState, &witnessState, &reference, &recent)
	if err != nil {
		report["classification_available"] = false
		return
	}
	report["classification_available"] = true
	report["evidence_counts"] = map[string]uint64{
		"active_operations": total, "archive_missing": noArchive,
		"execution_missing": noExecution, "witness_missing": noWitness,
		"archive_present_without_anchor": noAnchor, "execution_not_committed": executionState,
		"witness_not_committed": witnessState, "reference_mismatch": reference,
		"unproven_created_on_or_after_20261028": recent,
	}
	// Historical audit only: old publication rows never become current backlog.
	var missing, legacyExact, legacyConfirmed, accepted, rejected, held, unsettled uint64
	err = tx.QueryRowContext(ctx, `SELECT /*+ MAX_EXECUTION_TIME(1000) */ COUNT(*),
 COALESCE(SUM(b.message_id IS NOT NULL AND b.body_sha256=o.body_sha256 AND b.kind=o.kind),0),
 COALESCE(SUM(b.message_id IS NOT NULL AND b.body_sha256=o.body_sha256 AND b.kind=o.kind AND b.stage='confirmed'),0),
 COALESCE(SUM(o.decision='accepted'),0), COALESCE(SUM(o.decision='rejected'),0),
 COALESCE(SUM(o.decision='held'),0), COALESCE(SUM(o.decision NOT IN ('accepted','rejected','held')),0)
 FROM ai_messaging_operations o
 LEFT JOIN ai_messaging_archive a ON a.producer='qs-server' AND a.destination='qs-ai'
 AND a.message_id=CAST(o.command_id AS BINARY)
 LEFT JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai'
 AND b.message_id=CAST(o.command_id AS BINARY)
 WHERE o.retired=FALSE AND o.kind IN (1,2,3,4,5) AND a.message_id IS NULL`).Scan(&missing, &legacyExact, &legacyConfirmed, &accepted, &rejected, &held, &unsettled)
	if err != nil {
		report["historical_classification_available"] = false
		var driverError *mysql.MySQLError
		if errors.As(err, &driverError) {
			report["historical_mysql_error_number"] = driverError.Number
		}
		return
	}
	report["historical_classification_available"] = true
	report["historical_evidence_counts"] = map[string]uint64{
		"archive_missing": missing, "matching_legacy_publication": legacyExact,
		"matching_legacy_publication_confirmed": legacyConfirmed,
		"admission_accepted":                    accepted, "admission_rejected": rejected,
		"admission_held": held, "admission_unsettled": unsettled,
	}
	classifyHistoricalFacts(ctx, tx, report)
}

// No identities or payloads leave the process. Receipt admission, projection,
// and original transaction evidence remain separate observations.
func classifyHistoricalFacts(ctx context.Context, tx *sql.Tx, report map[string]any) {
	rows, err := tx.QueryContext(ctx, `SELECT /*+ MAX_EXECUTION_TIME(1000) */
 SHA2(CONCAT_WS('|',o.command_id,o.kind,o.body_sha256,o.organization_id,
 o.resource_id,o.aggregate_key,o.aggregate_sequence,o.decision,o.code,
 COALESCE(o.receipt_id,''),COALESCE(SHA2(o.receipt,256),''),o.created_at,
 COALESCE(CAST(o.decided_at AS CHAR),'')),256),o.kind,o.decision,
 o.receipt_id IS NOT NULL AND o.decided_at IS NOT NULL AND o.receipt IS NOT NULL AND OCTET_LENGTH(o.receipt)>0,
 COALESCE(i.message_id IS NOT NULL AND i.kind=6 AND i.aggregate_key=o.aggregate_key
 AND i.body_sha256=SHA2(o.receipt,256) AND i.body=o.receipt AND i.outcome='stored',FALSE),
 COALESCE(r.organization_id=o.organization_id AND r.session_id IS NOT NULL AND r.projection IS NOT NULL AND r.version>0,FALSE),
 COALESCE(r.organization_id=o.organization_id AND r.status IN ('completed','cancelled') AND r.version>0
 AND r.projection IS NOT NULL AND EXISTS(SELECT 1 FROM ai_bridge_events v WHERE v.request_id=r.request_id AND v.version=r.version),FALSE),
 COALESCE(es.organization_id=o.organization_id AND es.version>0,FALSE)
 FROM ai_messaging_operations o
 LEFT JOIN ai_messaging_archive a ON a.producer='qs-server' AND a.destination='qs-ai' AND a.message_id=CAST(o.command_id AS BINARY)
 LEFT JOIN ai_messaging_inbox i ON i.producer='qs-ai' AND i.message_id=o.receipt_id
 LEFT JOIN ai_bridge_requests r ON r.request_id=o.aggregate_key
 LEFT JOIN ai_messaging_evaluation_states es ON es.run_id=o.resource_id
 WHERE o.retired=FALSE AND o.kind IN (1,2,3,4,5) AND a.message_id IS NULL
 ORDER BY o.command_id LIMIT 1001`)
	if err != nil {
		report["historical_fact_audit_available"] = false
		var driverError *mysql.MySQLError
		if errors.As(err, &driverError) {
			report["historical_fact_mysql_error_number"] = driverError.Number
		}
		return
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]uint64{}
	groups := map[int32]map[string]uint64{}
	digest := sha256.New()
	for rows.Next() {
		var hash, decision string
		var kind int32
		var receipt, inbox, projection, terminal, evaluation bool
		if err = rows.Scan(&hash, &kind, &decision, &receipt, &inbox, &projection, &terminal, &evaluation); err != nil || counts["rows"] >= 1000 {
			report["historical_fact_audit_available"] = false
			return
		}
		_, _ = digest.Write([]byte(hash + "\n"))
		counts["rows"]++
		for name, ok := range map[string]bool{"receipt_metadata_present": receipt, "receipt_matches_stored_inbox": inbox,
			"workflow_projection_present": projection, "terminal_workflow_projection_with_event_version": terminal,
			"evaluation_state_present": evaluation} {
			if ok {
				counts[name]++
			}
		}
		if groups[kind] == nil {
			groups[kind] = map[string]uint64{}
		}
		if decision != "accepted" && decision != "rejected" && decision != "held" && decision != "" {
			decision = "unknown"
		}
		groups[kind][decision]++
	}
	if rows.Err() != nil {
		report["historical_fact_audit_available"] = false
		return
	}
	report["historical_fact_audit_available"] = true
	report["historical_fact_counts"] = counts
	report["historical_kind_decisions"] = groups
	report["historical_cohort_sha256"] = hex.EncodeToString(digest.Sum(nil))
}
