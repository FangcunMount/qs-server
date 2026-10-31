// A bounded read-only inventory. Publication state is not proof of business closure.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	mysql "github.com/go-sql-driver/mysql"
)

var sourceSHA = "development"

type object struct {
	Name, State, Closed string
}

func objects(owner string) (string, []object, error) {
	switch owner {
	case "qs.mysql":
		return "QS_APISERVER_MYSQL_", []object{{"rm_outbox", "state", "published"}, {"ai_messaging_outbox", "stage", "confirmed"}}, nil
	case "iam.mysql":
		return "IAM_APISERVER_MYSQL_", []object{{"domain_event_outbox", "status", "published"}, {"rm_outbox", "state", "published"}}, nil
	default:
		return "", nil, errors.New("unsupported owner")
	}
}

type tableReport struct {
	Name        string  `json:"name"`
	Available   bool    `json:"available"`
	Rows        *uint64 `json:"rows"`
	Closed      *uint64 `json:"publication_closed_rows"`
	Unclosed    *uint64 `json:"other_or_null_state_rows"`
	NullStates  *uint64 `json:"null_state_rows"`
	SchemaHash  string  `json:"show_create_table_sha256"`
	ClosedState string  `json:"publication_closed_state"`
}

type report struct {
	SourceSHA           string        `json:"source_sha"`
	Owner               string        `json:"owner"`
	ObservedAt          string        `json:"observed_at"`
	ReadOnly            bool          `json:"read_only"`
	Mutation            bool          `json:"mutation"`
	Complete            bool          `json:"inventory_complete"`
	DropAuthorized      bool          `json:"drop_authorized"`
	ExportVerified      bool          `json:"original_body_export_verified"`
	BusinessVerified    bool          `json:"business_responsibility_closed"`
	WritersExcluded     bool          `json:"writers_excluded"`
	TransactionReleased bool          `json:"transaction_released"`
	PoolReleased        bool          `json:"pool_released"`
	Binding             string        `json:"database_binding_sha256"`
	Version             string        `json:"mysql_version"`
	SchemaHead          *uint64       `json:"schema_head"`
	SchemaDirty         *bool         `json:"schema_dirty"`
	Tables              []tableReport `json:"tables"`
	Boundary            string        `json:"boundary"`
}

func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func collect(ctx context.Context, db *sql.DB, owner, endpoint string) (r report, err error) {
	_, tables, err := objects(owner)
	r = report{SourceSHA: sourceSHA, Owner: owner, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ReadOnly: true,
		Tables: []tableReport{}, Boundary: "Stored metadata only. Exact publication-state bytes are not business ACK, reference closure, writer exclusion, backup or deletion approval."}
	if err != nil {
		return r, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return r, err
	}
	defer func() {
		if releaseErr := tx.Rollback(); releaseErr != nil {
			r.Complete = false
			err = errors.Join(err, errors.New("read-only transaction release unavailable"))
		} else {
			r.TransactionReleased = true
		}
	}()
	var name, uuid string
	if err = tx.QueryRowContext(ctx, "SELECT DATABASE(),VERSION(),@@server_uuid").Scan(&name, &r.Version, &uuid); err != nil || name == "" || uuid == "" {
		return r, errors.New("database identity unavailable")
	}
	r.Binding = hash(endpoint + "\x00" + name + "\x00" + uuid)
	var headRows, head uint64
	var dirty bool
	if err = tx.QueryRowContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*),MIN(version),COALESCE(MAX(dirty),1) FROM schema_migrations").Scan(&headRows, &head, &dirty); err != nil || headRows != 1 || head == 0 {
		return r, errors.New("migration head unavailable or ambiguous")
	}
	r.SchemaHead, r.SchemaDirty = &head, &dirty
	if dirty {
		return r, errors.New("migration head is dirty")
	}
	for _, table := range tables {
		t := tableReport{Name: table.Name, ClosedState: table.Closed}
		r.Tables = append(r.Tables, t)
		target := &r.Tables[len(r.Tables)-1]
		var present int
		if err = tx.QueryRowContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=? AND table_type='BASE TABLE'", table.Name).Scan(&present); err != nil || present != 1 {
			return r, errors.New("required retained table unavailable")
		}
		var actual, ddl string
		if err = tx.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table.Name+"`").Scan(&actual, &ddl); err != nil || actual != table.Name || ddl == "" {
			return r, errors.New("table structure unavailable")
		}
		target.SchemaHash = hash(ddl)
		var total, closed, other, null uint64
		query := "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*), COALESCE(SUM(BINARY `" + table.State + "` = ?),0), COALESCE(SUM(`" + table.State + "` IS NULL OR BINARY `" + table.State + "` <> ?),0), COALESCE(SUM(`" + table.State + "` IS NULL),0) FROM `" + table.Name + "`"
		if err = tx.QueryRowContext(ctx, query, table.Closed, table.Closed).Scan(&total, &closed, &other, &null); err != nil {
			return r, errors.New("retained state inventory unavailable")
		}
		if closed+other != total || null > other {
			return r, errors.New("inconsistent state partition")
		}
		target.Available, target.Rows, target.Closed, target.Unclosed, target.NullStates = true, &total, &closed, &other, &null
	}
	r.Complete = true
	return r, nil
}

func run() (result report, resultErr error) {
	if len(os.Args) != 2 || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sourceSHA) {
		return report{}, errors.New("exact source and one fixed owner are required")
	}
	owner := os.Args[1]
	prefix, _, err := objects(owner)
	if err != nil {
		return report{}, err
	}
	get := func(key string) string { return os.Getenv(prefix + key) }
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = "tcp", get("HOST"), get("USERNAME"), get("PASSWORD"), get("DATABASE")
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 2*time.Second, 3*time.Second, 3*time.Second
	if cfg.Addr == "" || cfg.User == "" || cfg.DBName == "" || get("TLS_MODE") != "VERIFY_IDENTITY" {
		return report{}, errors.New("explicit owner TLS binding required")
	}
	if err = database.ConfigureMySQLTLS(cfg, get("TLS_MODE"), get("TLS_CA_FILE")); err != nil {
		return report{}, errors.New("owner TLS configuration unavailable")
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return report{}, errors.New("owner connector unavailable")
	}
	db := sql.OpenDB(connector)
	defer func() {
		if releaseErr := db.Close(); releaseErr != nil {
			result.Complete = false
			resultErr = errors.Join(resultErr, errors.New("owned pool release unavailable"))
		} else {
			result.PoolReleased = true
		}
	}()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return collect(ctx, db, owner, cfg.Addr)
}

func main() {
	r, err := run()
	if err != nil {
		r.Complete = false
		_ = json.NewEncoder(os.Stdout).Encode(r)
		_, _ = os.Stderr.WriteString("Read-only inventory unavailable; no deletion is authorized\n")
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
