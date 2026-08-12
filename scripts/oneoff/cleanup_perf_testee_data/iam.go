package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

const deleteIAMBatchProfileLinksSQL = `DELETE l FROM profile_links l JOIN tmp_cleanup_profile_batch_ids x ON x.id = l.profile_id`
const deleteIAMBatchProfilesSQL = `DELETE p FROM profiles p JOIN tmp_cleanup_profile_batch_ids x ON x.id = p.id`

type qsProfileScopeCounts struct {
	existingTestees     int
	distinctProfiles    int
	invalidMappings     int
	nonTargetReferences int
}

type iamScopeCounts struct {
	profiles        int
	links           int
	invalidProfiles int
	invalidLinks    int
	missingUsers    int
}

func openIAMMySQL(ctx context.Context, dsn string) (*sql.DB, *sql.Conn, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	if _, err := conn.ExecContext(ctx, "SET NAMES utf8mb4"); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, nil, err
	}
	return db, conn, nil
}

func prepareQSProfileScope(ctx context.Context, conn *sql.Conn, profileIDs []uint64) error {
	if _, err := conn.ExecContext(ctx, `CREATE TEMPORARY TABLE tmp_cleanup_profile_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`); err != nil {
		return err
	}
	if err := bulkInsertUint64IDs(ctx, conn, "tmp_cleanup_profile_ids", profileIDs); err != nil {
		return fmt.Errorf("insert profile ids: %w", err)
	}
	return nil
}

func validateQSProfileScope(ctx context.Context, conn *sql.Conn, allowMissing, requireNoExistingTestees bool, expectedTestees, expectedProfiles int) error {
	var counts qsProfileScopeCounts
	queries := []struct {
		target *int
		query  string
	}{
		{&counts.existingTestees, `SELECT COUNT(*) FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
		{&counts.distinctProfiles, `SELECT COUNT(DISTINCT t.profile_id) FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
		{&counts.invalidMappings, `SELECT COUNT(*)
FROM testee t
JOIN tmp_cleanup_testee_ids x ON x.id = t.id
LEFT JOIN tmp_cleanup_profile_ids p ON p.id = t.profile_id
WHERE t.profile_id IS NULL OR p.id IS NULL`},
		{&counts.nonTargetReferences, `SELECT COUNT(*)
FROM testee t
JOIN tmp_cleanup_profile_ids p ON p.id = t.profile_id
LEFT JOIN tmp_cleanup_testee_ids x ON x.id = t.id
WHERE x.id IS NULL`},
	}
	for _, item := range queries {
		if err := conn.QueryRowContext(ctx, item.query).Scan(item.target); err != nil {
			return err
		}
	}
	return validateQSProfileScopeCounts(counts, allowMissing, requireNoExistingTestees, expectedTestees, expectedProfiles)
}

func validateQSProfileScopeCounts(counts qsProfileScopeCounts, allowMissing, requireNoExistingTestees bool, expectedTestees, expectedProfiles int) error {
	if counts.invalidMappings != 0 {
		return fmt.Errorf("%d selected testee(s) do not map to the explicit IAM profile scope", counts.invalidMappings)
	}
	if requireNoExistingTestees && counts.existingTestees != 0 {
		return fmt.Errorf("IAM cleanup requires every selected QS testee to be absent; still present=%d", counts.existingTestees)
	}
	if counts.nonTargetReferences != 0 {
		return fmt.Errorf("%d non-target QS testee row(s) still reference selected IAM profiles", counts.nonTargetReferences)
	}
	if counts.distinctProfiles != counts.existingTestees {
		return fmt.Errorf("selected QS rows are not one-testee-to-one-profile: testees=%d distinct_profiles=%d", counts.existingTestees, counts.distinctProfiles)
	}
	if !allowMissing && (counts.existingTestees != expectedTestees || counts.distinctProfiles != expectedProfiles || expectedTestees != expectedProfiles) {
		return fmt.Errorf("QS/IAM explicit scope mismatch: expected_testees=%d existing_testees=%d expected_profiles=%d mapped_profiles=%d", expectedTestees, counts.existingTestees, expectedProfiles, counts.distinctProfiles)
	}
	if allowMissing && counts.distinctProfiles > expectedProfiles {
		return fmt.Errorf("QS profile scope exceeds explicit IAM scope: mapped_profiles=%d expected_profiles=%d", counts.distinctProfiles, expectedProfiles)
	}
	return nil
}

func prepareIAMScope(ctx context.Context, conn *sql.Conn, profileIDs []uint64, allowMissing bool) error {
	if _, err := conn.ExecContext(ctx, `CREATE TEMPORARY TABLE tmp_cleanup_profile_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`); err != nil {
		return err
	}
	if err := bulkInsertUint64IDs(ctx, conn, "tmp_cleanup_profile_ids", profileIDs); err != nil {
		return fmt.Errorf("insert iam profile ids: %w", err)
	}
	return validateIAMScope(ctx, conn, len(profileIDs), allowMissing)
}

func validateIAMScope(ctx context.Context, conn *sql.Conn, expected int, allowMissing bool) error {
	var counts iamScopeCounts
	queries := []struct {
		target *int
		query  string
	}{
		{&counts.profiles, `SELECT COUNT(*) FROM profiles p JOIN tmp_cleanup_profile_ids x ON x.id = p.id`},
		{&counts.links, `SELECT COUNT(*) FROM profile_links l JOIN tmp_cleanup_profile_ids x ON x.id = l.profile_id`},
		{&counts.invalidProfiles, `SELECT COUNT(*) FROM profiles p JOIN tmp_cleanup_profile_ids x ON x.id = p.id WHERE p.deleted_at IS NOT NULL`},
		{&counts.invalidLinks, `SELECT COUNT(*)
FROM profile_links l
JOIN tmp_cleanup_profile_ids x ON x.id = l.profile_id
WHERE l.type <> 'relation' OR l.revoked_at IS NOT NULL OR l.deleted_at IS NOT NULL`},
		{&counts.missingUsers, `SELECT COUNT(*)
FROM profile_links l
JOIN tmp_cleanup_profile_ids x ON x.id = l.profile_id
LEFT JOIN users u ON u.id = l.user_id AND u.deleted_at IS NULL
WHERE u.id IS NULL`},
	}
	for _, item := range queries {
		if err := conn.QueryRowContext(ctx, item.query).Scan(item.target); err != nil {
			return err
		}
	}
	return validateIAMScopeCounts(counts, expected, allowMissing)
}

func validateIAMScopeCounts(counts iamScopeCounts, expected int, allowMissing bool) error {
	if !allowMissing && (counts.profiles != expected || counts.links != expected) {
		return fmt.Errorf("IAM scope is not one-profile-to-one-link: expected=%d profiles=%d profile_links=%d", expected, counts.profiles, counts.links)
	}
	if allowMissing && (counts.profiles > expected || counts.links != counts.profiles) {
		return fmt.Errorf("remaining IAM scope is not one-profile-to-one-link: original=%d profiles=%d profile_links=%d", expected, counts.profiles, counts.links)
	}
	if counts.invalidProfiles != 0 || counts.invalidLinks != 0 || counts.missingUsers != 0 {
		return fmt.Errorf("IAM scope guard failed: deleted_profiles=%d non_active_relation_links=%d missing_active_users=%d", counts.invalidProfiles, counts.invalidLinks, counts.missingUsers)
	}
	return nil
}

func countIAMRows(ctx context.Context, conn *sql.Conn) ([]namedCount, error) {
	items := []mysqlCountItem{
		{name: "profiles", query: `SELECT COUNT(*) FROM profiles p JOIN tmp_cleanup_profile_ids x ON x.id = p.id`},
		{name: "profile_links", query: `SELECT COUNT(*) FROM profile_links l JOIN tmp_cleanup_profile_ids x ON x.id = l.profile_id`},
	}
	out := make([]namedCount, 0, len(items))
	for _, item := range items {
		var count int64
		if err := conn.QueryRowContext(ctx, item.query).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		out = append(out, namedCount{Name: item.name, Count: count})
	}
	return out, nil
}

func backupIAMRows(ctx context.Context, conn *sql.Conn, suffix string, expected int) error {
	items := iamBackupItems()
	for _, item := range items {
		backupTable, err := mysqlBackupTableName(item.table, suffix)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE `%s`", backupTable, item.table)); err != nil {
			return fmt.Errorf("create IAM backup table %s: %w", backupTable, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("INSERT IGNORE INTO `%s` %s", backupTable, item.selectSQL)); err != nil {
			return fmt.Errorf("insert IAM backup table %s: %w", backupTable, err)
		}
		var count int
		if err := conn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`", backupTable)).Scan(&count); err != nil {
			return fmt.Errorf("count IAM backup table %s: %w", backupTable, err)
		}
		if count != expected {
			return fmt.Errorf("IAM backup table %s has unexpected rows: expected=%d actual=%d", backupTable, expected, count)
		}
	}
	return nil
}

func iamBackupItems() []mysqlBackupItem {
	return []mysqlBackupItem{
		{table: "profiles", selectSQL: `SELECT p.* FROM profiles p JOIN tmp_cleanup_profile_ids x ON x.id = p.id`},
		{table: "profile_links", selectSQL: `SELECT l.* FROM profile_links l JOIN tmp_cleanup_profile_ids x ON x.id = l.profile_id`},
	}
}

func deleteIAMRows(ctx context.Context, conn *sql.Conn, lockWaitTimeoutSec, maxRetries, batchSize int, allowMissing bool) ([]namedCount, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("IAM delete batch size must be positive")
	}
	if lockWaitTimeoutSec > 0 {
		if _, err := conn.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout = ?", lockWaitTimeoutSec); err != nil {
			return nil, fmt.Errorf("set IAM innodb_lock_wait_timeout: %w", err)
		}
	}
	var original int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM tmp_cleanup_profile_ids`).Scan(&original); err != nil {
		return nil, err
	}
	if original == 0 {
		return nil, fmt.Errorf("IAM profile scope is empty")
	}
	if err := validateIAMScope(ctx, conn, original, allowMissing); err != nil {
		return nil, fmt.Errorf("revalidate IAM scope before delete: %w", err)
	}
	var expected int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles p JOIN tmp_cleanup_profile_ids x ON x.id = p.id`).Scan(&expected); err != nil {
		return nil, err
	}
	if expected == 0 {
		if allowMissing {
			return []namedCount{{Name: "profile_links", Count: 0}, {Name: "profiles", Count: 0}}, nil
		}
		return nil, fmt.Errorf("IAM profile scope has no existing rows")
	}
	if _, err := conn.ExecContext(ctx, `CREATE TEMPORARY TABLE tmp_cleanup_profile_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`); err != nil {
		return nil, fmt.Errorf("create IAM profile batch table: %w", err)
	}

	var deleted int64
	for deleted < int64(expected) {
		batchDeleted, err := deleteIAMProfileBatchWithRetry(ctx, conn, batchSize, maxRetries)
		if err != nil {
			return nil, err
		}
		if batchDeleted == 0 {
			return nil, fmt.Errorf("IAM delete stopped early: expected=%d deleted=%d", expected, deleted)
		}
		deleted += batchDeleted
		log.Printf("iam delete progress: deleted_profiles=%d/%d batch_size=%d", deleted, expected, batchSize)
	}
	if deleted != int64(expected) {
		return nil, fmt.Errorf("IAM delete total mismatch: expected=%d deleted=%d", expected, deleted)
	}

	var remainingProfiles, remainingLinks int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles p JOIN tmp_cleanup_profile_ids x ON x.id = p.id`).Scan(&remainingProfiles); err != nil {
		return nil, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_links l JOIN tmp_cleanup_profile_ids x ON x.id = l.profile_id`).Scan(&remainingLinks); err != nil {
		return nil, err
	}
	if remainingProfiles != 0 || remainingLinks != 0 {
		return nil, fmt.Errorf("IAM delete verification failed: remaining_profiles=%d remaining_profile_links=%d", remainingProfiles, remainingLinks)
	}
	return []namedCount{{Name: "profile_links", Count: deleted}, {Name: "profiles", Count: deleted}}, nil
}

func deleteIAMProfileBatchWithRetry(ctx context.Context, conn *sql.Conn, batchSize, maxRetries int) (int64, error) {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		deleted, err := deleteIAMProfileBatchOnce(ctx, conn, batchSize)
		if err == nil {
			return deleted, nil
		}
		lastErr = err
		if !isMySQLLockError(err) || attempt == maxRetries {
			return 0, err
		}
		wait := time.Duration(attempt) * 5 * time.Second
		log.Printf("iam delete lock contention: attempt=%d/%d retry_in=%s err=%v", attempt, maxRetries, wait, err)
		if err := sleepWithContext(ctx, wait); err != nil {
			return 0, err
		}
	}
	return 0, lastErr
}

func deleteIAMProfileBatchOnce(ctx context.Context, conn *sql.Conn, batchSize int) (int64, error) {
	if _, err := conn.ExecContext(ctx, `DELETE FROM tmp_cleanup_profile_batch_ids`); err != nil {
		return 0, fmt.Errorf("clear IAM profile batch: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(cause error) (int64, error) {
		_ = tx.Rollback()
		return 0, cause
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO tmp_cleanup_profile_batch_ids (id)
SELECT scope.id
FROM tmp_cleanup_profile_ids scope
JOIN profiles p ON p.id = scope.id
ORDER BY scope.id
LIMIT ?`, batchSize)
	if err != nil {
		return rollback(err)
	}
	expected, err := result.RowsAffected()
	if err != nil {
		return rollback(err)
	}
	if expected == 0 {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	if err := lockAndValidateIAMBatch(ctx, tx, expected); err != nil {
		return rollback(err)
	}
	linkResult, err := tx.ExecContext(ctx, deleteIAMBatchProfileLinksSQL)
	if err != nil {
		return rollback(err)
	}
	linkDeleted, _ := linkResult.RowsAffected()
	profileResult, err := tx.ExecContext(ctx, deleteIAMBatchProfilesSQL)
	if err != nil {
		return rollback(err)
	}
	profileDeleted, _ := profileResult.RowsAffected()
	if linkDeleted != expected || profileDeleted != expected {
		return rollback(fmt.Errorf("IAM delete count mismatch: expected=%d profile_links=%d profiles=%d", expected, linkDeleted, profileDeleted))
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return profileDeleted, nil
}

func lockAndValidateIAMBatch(ctx context.Context, tx *sql.Tx, expected int64) error {
	profileRows, err := tx.QueryContext(ctx, `SELECT p.id, p.deleted_at IS NOT NULL
FROM profiles p
JOIN tmp_cleanup_profile_batch_ids batch ON batch.id = p.id
ORDER BY p.id
FOR UPDATE`)
	if err != nil {
		return err
	}
	var profiles int64
	for profileRows.Next() {
		var id uint64
		var deleted int
		if err := profileRows.Scan(&id, &deleted); err != nil {
			_ = profileRows.Close()
			return err
		}
		if deleted != 0 {
			_ = profileRows.Close()
			return fmt.Errorf("IAM profile %d became deleted before cleanup", id)
		}
		profiles++
	}
	if err := profileRows.Err(); err != nil {
		_ = profileRows.Close()
		return err
	}
	if err := profileRows.Close(); err != nil {
		return err
	}
	if profiles != expected {
		return fmt.Errorf("IAM batch profile count mismatch: expected=%d profiles=%d", expected, profiles)
	}

	linkRows, err := tx.QueryContext(ctx, `SELECT l.id, l.type, l.revoked_at IS NOT NULL, l.deleted_at IS NOT NULL
FROM profile_links l
JOIN tmp_cleanup_profile_batch_ids batch ON batch.id = l.profile_id
ORDER BY l.profile_id, l.id
FOR UPDATE`)
	if err != nil {
		return err
	}
	var links int64
	for linkRows.Next() {
		var id uint64
		var linkType string
		var revoked, deleted int
		if err := linkRows.Scan(&id, &linkType, &revoked, &deleted); err != nil {
			_ = linkRows.Close()
			return err
		}
		if linkType != "relation" || revoked != 0 || deleted != 0 {
			_ = linkRows.Close()
			return fmt.Errorf("IAM profile link %d became inactive before cleanup", id)
		}
		links++
	}
	if err := linkRows.Err(); err != nil {
		_ = linkRows.Close()
		return err
	}
	if err := linkRows.Close(); err != nil {
		return err
	}
	if links != expected {
		return fmt.Errorf("IAM batch profile link count mismatch: expected=%d profile_links=%d", expected, links)
	}
	return nil
}
