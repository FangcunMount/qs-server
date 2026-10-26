//go:build integration

package aibridge

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func TestMQRuntimeMissingTechnicalCoverageRefusesBeforeTransport(t *testing.T) {
	cfg, err := mysqlDriver.ParseDSN(os.Getenv("QS_AI_MQ_TEST_DSN"))
	if err != nil || cfg.DBName == "" || strings.ContainsAny(cfg.DBName, "`\\") {
		t.Fatal("required safe disposable test schema", err)
	}
	source, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	sourceSchema := cfg.DBName
	name := "mq_preflight_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = source.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := source.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error(err)
		}
	}()
	for _, table := range []string{"ai_messaging_admission", "ai_messaging_aggregates", "ai_messaging_evaluation_states", "ai_messaging_inbox", "ai_messaging_failures", "ai_messaging_outbox", "ai_messaging_operations", "ai_messaging_quarantine", "ai_messaging_observations"} {
		query := "CREATE TABLE `" + name + "`." + table + " LIKE `" + sourceSchema + "`." + table
		if table == "ai_messaging_operations" {
			// LIKE renames CHECK constraints. Preserve the exact registered
			// retirement constraint so this fixture isolates observation coverage.
			var actual, definition string
			if err = source.QueryRow("SHOW CREATE TABLE `"+sourceSchema+"`.ai_messaging_operations").Scan(&actual, &definition); err != nil {
				t.Fatal(err)
			}
			prefix := "CREATE TABLE `ai_messaging_operations`"
			if actual != table || !strings.HasPrefix(definition, prefix) {
				t.Fatal("unexpected operation fixture definition")
			}
			query = strings.Replace(definition, prefix, "CREATE TABLE `"+name+"`.`ai_messaging_operations`", 1)
		}
		if _, err = source.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = source.Exec("INSERT INTO `" + name + "`.ai_messaging_admission SELECT * FROM `" + sourceSchema + "`.ai_messaging_admission"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec("INSERT INTO `" + name + "`.ai_messaging_observations SELECT * FROM `" + sourceSchema + "`.ai_messaging_observations WHERE kind<>'duplicate_event'"); err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runtime := &MessagingRuntime{DB: db}
	err = runtime.Start(t.Context())
	if err == nil || err.Error() != "AI MQ required technical observation schema unavailable" || runtime.rocket != nil || runtime.handlerCancel != nil || runtime.started {
		t.Fatal("partial schema consumed/connected transport", err)
	}
	if _, err = source.Exec("INSERT INTO `" + name + "`.ai_messaging_observations SELECT * FROM `" + sourceSchema + "`.ai_messaging_observations WHERE kind='duplicate_event'"); err != nil {
		t.Fatal(err)
	}
	if err = runtime.preflightStorage(t.Context()); err != nil {
		t.Fatal("complete schema refused", err)
	}
	if _, err = db.Exec("ALTER TABLE ai_messaging_operations ALTER INDEX idx_ai_messaging_operations_request_stats INVISIBLE"); err != nil {
		t.Fatal(err)
	}
	err = runtime.Start(t.Context())
	if err == nil || err.Error() != "AI MQ required operation statistics index mismatch" || runtime.rocket != nil || runtime.handlerCancel != nil || runtime.started {
		t.Fatal("unusable operation index reached transport", err)
	}
	if _, err = db.Exec("ALTER TABLE ai_messaging_operations ALTER INDEX idx_ai_messaging_operations_request_stats VISIBLE"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("ALTER TABLE ai_messaging_operations ALTER CHECK chk_ai_messaging_operations_retirement NOT ENFORCED"); err != nil {
		t.Fatal(err)
	}
	err = runtime.Start(t.Context())
	if err == nil || err.Error() != "AI MQ required retirement constraint unavailable" || runtime.rocket != nil || runtime.handlerCancel != nil || runtime.started {
		t.Fatal("unenforced retirement constraint reached transport", err)
	}
	if _, err = db.Exec("ALTER TABLE ai_messaging_operations ALTER CHECK chk_ai_messaging_operations_retirement ENFORCED"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DROP TABLE ai_messaging_observations"); err != nil {
		t.Fatal(err)
	}
	err = runtime.Start(t.Context())
	if err == nil || err.Error() != "AI MQ required schema unavailable" || runtime.handlerCancel != nil {
		t.Fatal("missing table exposed details/created clients", err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal("borrowed pool closed", err)
	}
}
