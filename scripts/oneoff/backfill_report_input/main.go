// backfill_report_input upgrades legacy/v2 evaluation outcome report input to
// schema v3 when a minimal replay-safe snapshot can be derived (MC-R017 batch 3).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	_ "github.com/go-sql-driver/mysql"
)

type config struct {
	mysqlDSN string
	limit    int
	apply    bool
	audit    bool
	timeout  time.Duration
}

type candidate struct {
	ID              uint64
	ReportInputJSON sql.NullString
	ModelKind       string
	ModelSubKind    sql.NullString
	ModelAlgorithm  sql.NullString
	ModelCode       string
	ModelVersion    string
	ModelTitle      sql.NullString
	AlgorithmFamily sql.NullString
}

type result struct {
	Scanned   int `json:"scanned"`
	Upgraded  int `json:"upgraded"`
	Skipped   int `json:"skipped"`
	AuditFail int `json:"audit_fail"`
}

func main() {
	cfg := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	db, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = db.Close() }()
	stats, err := run(ctx, db, cfg)
	if err != nil {
		log.Fatalf("backfill report input failed: %v", err)
	}
	_ = json.NewEncoder(os.Stdout).Encode(stats)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.IntVar(&cfg.limit, "limit", 500, "max outcomes to scan")
	flag.BoolVar(&cfg.apply, "apply", false, "persist upgraded report input")
	flag.BoolVar(&cfg.audit, "audit-only", false, "audit decode only, do not upgrade")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, db *sql.DB, cfg config) (*result, error) {
	if cfg.mysqlDSN == "" {
		return nil, fmt.Errorf("mysql-dsn is required (or set MYSQL_DSN)")
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, report_input_json, model_kind, model_sub_kind, model_algorithm, model_code, model_version, model_title, algorithm_family
FROM evaluation_outcome
WHERE report_input_json IS NOT NULL AND report_input_json <> ''
ORDER BY id ASC
LIMIT ?`, cfg.limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	stats := &result{}
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.ID, &item.ReportInputJSON, &item.ModelKind, &item.ModelSubKind, &item.ModelAlgorithm, &item.ModelCode, &item.ModelVersion, &item.ModelTitle, &item.AlgorithmFamily); err != nil {
			return nil, err
		}
		stats.Scanned++
		raw := []byte(item.ReportInputJSON.String)
		modelRef := modelRefFromCandidate(item)
		if cfg.audit {
			if issues := evaluationinput.AuditReportInput(raw, modelRef); len(issues) > 0 {
				stats.AuditFail++
			}
			continue
		}
		family, err := algorithmFamilyFromCandidate(item)
		if err != nil {
			stats.Skipped++
			continue
		}
		upgrade, err := evaluationinput.TryUpgradeReportInputToV3(raw, modelRef, family)
		if err != nil {
			return nil, fmt.Errorf("outcome %d: %w", item.ID, err)
		}
		if upgrade.Skipped != "" || upgrade.ToSchema < evaluationinput.ReportInputSchemaV3 {
			stats.Skipped++
			continue
		}
		stats.Upgraded++
		if !cfg.apply {
			continue
		}
		if _, err := db.ExecContext(ctx, `UPDATE evaluation_outcome SET report_input_json = ? WHERE id = ?`, string(upgrade.Data), item.ID); err != nil {
			return nil, fmt.Errorf("update outcome %d: %w", item.ID, err)
		}
	}
	return stats, rows.Err()
}

func modelRefFromCandidate(item candidate) evaluationinput.ModelRef {
	ref := evaluationinput.ModelRef{
		Kind:    evaluationinput.EvaluationModelKind(item.ModelKind),
		Code:    item.ModelCode,
		Version: item.ModelVersion,
	}
	if item.ModelSubKind.Valid {
		ref.SubKind = item.ModelSubKind.String
	}
	if item.ModelAlgorithm.Valid {
		ref.Algorithm = item.ModelAlgorithm.String
	}
	if item.ModelTitle.Valid {
		ref.Title = item.ModelTitle.String
	}
	return ref
}

func algorithmFamilyFromCandidate(item candidate) (modelcatalog.AlgorithmFamily, error) {
	if item.AlgorithmFamily.Valid && item.AlgorithmFamily.String != "" {
		family := modelcatalog.AlgorithmFamily(item.AlgorithmFamily.String)
		if family.IsValid() {
			return family, nil
		}
	}
	subKind := modelcatalog.SubKindEmpty
	if item.ModelSubKind.Valid {
		subKind = modelcatalog.SubKind(item.ModelSubKind.String)
	}
	algorithm := modelcatalog.Algorithm("")
	if item.ModelAlgorithm.Valid {
		algorithm = modelcatalog.Algorithm(item.ModelAlgorithm.String)
	}
	family, ok := identity.AlgorithmFamilyFromIdentity(modelcatalog.Kind(item.ModelKind), subKind, algorithm)
	if !ok {
		return "", fmt.Errorf("unsupported model identity")
	}
	return family, nil
}
