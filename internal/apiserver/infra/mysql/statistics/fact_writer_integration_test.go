//go:build integration

package statistics

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestFactWriterBatchAgainstMySQL(t *testing.T) {
	dsn := os.Getenv("QS_STATISTICS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("QS_STATISTICS_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var database string
	if err := db.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
		t.Fatal(err)
	}
	if database != "qs_statistics_test" {
		t.Fatalf("refusing integration test against database %q", database)
	}

	if err := db.Exec("DROP TABLE IF EXISTS statistics_access_fact").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TABLE IF EXISTS statistics_access_fact").Error })
	if err := db.Exec(`CREATE TABLE statistics_access_fact (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		org_id BIGINT NOT NULL,
		fact_key VARCHAR(255) NOT NULL,
		core_hash CHAR(64) NOT NULL,
		fact_type VARCHAR(64) NOT NULL,
		occurred_at DATETIME(3) NOT NULL,
		stat_date DATE NOT NULL,
		source_type VARCHAR(64) NOT NULL,
		source_ref VARCHAR(128) NOT NULL,
		schema_version INT UNSIGNED NOT NULL DEFAULT 1,
		created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		PRIMARY KEY (id),
		UNIQUE KEY uk_statistics_access_fact_key (fact_key)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error; err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	facts := make([]map[string]any, 0, collectorBatchSize)
	for index := 0; index < collectorBatchSize; index++ {
		facts = append(facts, testAccessFact("fact-"+strconv.Itoa(index), at))
	}
	writer := factWriter{db: db}
	startedAt := time.Now()
	dispositions, err := writer.writeBatch(context.Background(), "statistics_access_fact", facts, false)
	if err != nil {
		t.Fatal(err)
	}
	for index, disposition := range dispositions {
		if disposition != factWriteInserted {
			t.Fatalf("insert disposition[%d]=%v", index, disposition)
		}
	}
	t.Logf("inserted %d facts in %s", len(facts), time.Since(startedAt))

	startedAt = time.Now()
	dispositions, err = writer.writeBatch(context.Background(), "statistics_access_fact", facts, true)
	if err != nil {
		t.Fatal(err)
	}
	for index, disposition := range dispositions {
		if disposition != factWriteExisting {
			t.Fatalf("validate disposition[%d]=%v", index, disposition)
		}
	}
	t.Logf("validated %d facts in %s", len(facts), time.Since(startedAt))

	conflicting := testAccessFact("fact-2", at.Add(time.Second))
	dispositions, err = writer.writeBatch(context.Background(), "statistics_access_fact", []map[string]any{conflicting}, false)
	if err == nil || dispositions[0] != factWriteConflict {
		t.Fatalf("conflict dispositions=%v err=%v", dispositions, err)
	}
}
