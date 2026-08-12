package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const duplicateSelectionOrder = `e.outcome_count DESC, e.evaluated_count DESC, e.completed_tasks DESC,
           e.submitted_count DESC, e.assessment_count DESC, e.intake_count DESC,
           e.enrollment_count DESC, e.created_at, e.testee_id`

type config struct {
	mysqlDSN     string
	iamSchema    string
	clinicianID  uint64
	source       string
	createdFrom  string
	createdUntil string
	outputDir    string
	timeout      time.Duration
}

type duplicateRow struct {
	TesteeID                 uint64
	ProfileID                uint64
	ProfileLinkID            uint64
	UserID                   uint64
	CanonicalTestee          uint64
	CanonicalProfile         uint64
	OrgID                    int64
	Name                     string
	Gender                   int
	Birthday                 string
	Source                   string
	CreatedDate              string
	CreatedAt                string
	GroupSize                int
	DuplicateOrdinal         int
	OutcomeCount             int
	EvaluatedCount           int
	SubmittedCount           int
	AssessmentCount          int
	CompletedTasks           int
	IntakeCount              int
	EnrollmentCount          int
	CanonicalOutcomeCount    int
	CanonicalEvaluatedCount  int
	CanonicalSubmittedCount  int
	CanonicalAssessmentCount int
	CanonicalCompletedTasks  int
	CanonicalIntakeCount     int
	CanonicalEnrollmentCount int
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	db, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		log.Fatalf("begin read-only transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	anomalies, err := loadScopeAnomalies(ctx, tx, cfg)
	if err != nil {
		log.Fatalf("validate source scope: %v", err)
	}
	if len(anomalies) > 0 {
		log.Fatalf("source scope contains unsafe testee/profile/link rows; sample=%s", strings.Join(anomalies, "; "))
	}
	rows, err := loadDuplicateRows(ctx, tx, cfg)
	if err != nil {
		log.Fatalf("select duplicate scope: %v", err)
	}
	if len(rows) == 0 {
		if err := tx.Commit(); err != nil {
			log.Fatalf("commit empty read-only transaction: %v", err)
		}
		log.Print("no duplicate rows matched the explicit scope")
		return
	}
	if err := validateDuplicateRows(rows); err != nil {
		log.Fatalf("validate duplicate rows: %v", err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatalf("commit read-only transaction: %v", err)
	}
	if err := writeScopeFiles(cfg, rows); err != nil {
		log.Fatalf("write scope files: %v", err)
	}

	groups := map[uint64]int{}
	for _, row := range rows {
		groups[row.CanonicalTestee] = row.GroupSize
	}
	log.Printf("scope written: groups=%d duplicate_testees=%d output_dir=%s", len(groups), len(rows), cfg.outputDir)
}

func parseFlags() (config, error) {
	var cfg config
	var clinicianIDRaw string
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "QS MySQL DSN with read access to the IAM schema")
	flag.StringVar(&cfg.iamSchema, "iam-schema", "iam", "IAM database/schema name on the same MySQL server")
	flag.StringVar(&clinicianIDRaw, "clinician-id", "", "required clinician ID")
	flag.StringVar(&cfg.source, "source", "daily_simulation", "required testee source")
	flag.StringVar(&cfg.createdFrom, "created-from", "", "required inclusive date, YYYY-MM-DD")
	flag.StringVar(&cfg.createdUntil, "created-through", "", "required inclusive date, YYYY-MM-DD")
	flag.StringVar(&cfg.outputDir, "output-dir", "", "required output directory for manifest and explicit ID files")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "read-only selection timeout")
	flag.Parse()

	if strings.TrimSpace(cfg.mysqlDSN) == "" {
		return cfg, errors.New("--mysql-dsn is required")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(cfg.iamSchema) {
		return cfg, fmt.Errorf("unsafe --iam-schema %q", cfg.iamSchema)
	}
	if strings.TrimSpace(clinicianIDRaw) == "" {
		return cfg, errors.New("--clinician-id is required")
	}
	clinicianID, err := strconv.ParseUint(clinicianIDRaw, 10, 64)
	if err != nil || clinicianID == 0 {
		return cfg, fmt.Errorf("invalid --clinician-id %q", clinicianIDRaw)
	}
	cfg.clinicianID = clinicianID
	if strings.TrimSpace(cfg.source) == "" {
		return cfg, errors.New("--source must not be empty")
	}
	from, err := time.Parse("2006-01-02", cfg.createdFrom)
	if err != nil {
		return cfg, fmt.Errorf("invalid --created-from: %w", err)
	}
	through, err := time.Parse("2006-01-02", cfg.createdUntil)
	if err != nil {
		return cfg, fmt.Errorf("invalid --created-through: %w", err)
	}
	if through.Before(from) {
		return cfg, errors.New("--created-through must be on or after --created-from")
	}
	cfg.createdFrom = from.Format("2006-01-02") + " 00:00:00"
	cfg.createdUntil = through.AddDate(0, 0, 1).Format("2006-01-02") + " 00:00:00"
	if strings.TrimSpace(cfg.outputDir) == "" {
		return cfg, errors.New("--output-dir is required")
	}
	absOutputDir, err := filepath.Abs(cfg.outputDir)
	if err != nil {
		return cfg, fmt.Errorf("resolve --output-dir: %w", err)
	}
	cfg.outputDir = absOutputDir
	if cfg.timeout <= 0 {
		return cfg, errors.New("--timeout must be positive")
	}
	return cfg, nil
}

func loadScopeAnomalies(ctx context.Context, tx *sql.Tx, cfg config) ([]string, error) {
	rows, err := tx.QueryContext(ctx, scopeAnomaliesQuery(cfg.iamSchema), cfg.source, cfg.createdFrom, cfg.createdUntil, cfg.clinicianID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id uint64
		var reason string
		if err := rows.Scan(&id, &reason); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%d:%s", id, reason))
	}
	return out, rows.Err()
}

func scopeAnomaliesQuery(iamSchema string) string {
	return fmt.Sprintf(`WITH source_scope AS (
  SELECT t.*
  FROM testee t
  WHERE t.deleted_at IS NULL
    AND t.source = ?
    AND t.created_at >= ?
    AND t.created_at < ?
    AND EXISTS (
      SELECT 1 FROM clinician_relation r
      WHERE r.testee_id = t.id AND r.clinician_id = ?
        AND r.is_active = 1 AND r.deleted_at IS NULL
    )
), link_scope AS (
  SELECT links.profile_id,
	         MIN(id) AS profile_link_id,
	         MIN(user_id) AS user_id,
	         COUNT(*) AS total_link_count,
	         SUM(CASE WHEN revoked_at IS NULL AND deleted_at IS NULL THEN 1 ELSE 0 END) AS active_link_count,
	         SUM(CASE WHEN type = 'relation' THEN 1 ELSE 0 END) AS relation_link_count
  FROM %s.profile_links links
  JOIN (SELECT DISTINCT profile_id FROM source_scope) target ON target.profile_id = links.profile_id
  GROUP BY links.profile_id
	)
	SELECT t.id,
       CONCAT_WS(',',
         IF(p.id IS NULL, 'missing_profile', NULL),
         IF(p.deleted_at IS NOT NULL, 'deleted_profile', NULL),
         IF(NOT (p.name <=> t.name), 'name_mismatch', NULL),
         IF(NOT (p.gender <=> t.gender), 'gender_mismatch', NULL),
         IF(NOT (p.birthday <=> DATE_FORMAT(t.birthday, '%%Y-%%m-%%d')), 'birthday_mismatch', NULL),
         IF(COALESCE(ls.total_link_count, 0) <> 1, 'link_count', NULL),
         IF(COALESCE(ls.active_link_count, 0) <> 1, 'active_link_count', NULL),
         IF(COALESCE(ls.relation_link_count, 0) <> 1, 'relation_link_type', NULL),
         IF(u.id IS NULL OR u.deleted_at IS NOT NULL, 'missing_active_user', NULL)
       ) AS reason
FROM source_scope t
	LEFT JOIN %s.profiles p ON p.id = t.profile_id
	LEFT JOIN link_scope ls ON ls.profile_id = t.profile_id
	LEFT JOIN %s.users u ON u.id = ls.user_id
WHERE
	    p.id IS NULL OR p.deleted_at IS NOT NULL
	    OR NOT (p.name <=> t.name)
    OR NOT (p.gender <=> t.gender)
    OR NOT (p.birthday <=> DATE_FORMAT(t.birthday, '%%Y-%%m-%%d'))
    OR COALESCE(ls.total_link_count, 0) <> 1
    OR COALESCE(ls.active_link_count, 0) <> 1
    OR COALESCE(ls.relation_link_count, 0) <> 1
    OR u.id IS NULL OR u.deleted_at IS NOT NULL
	ORDER BY t.id
	LIMIT 20`, iamSchema, iamSchema, iamSchema)
}

func loadDuplicateRows(ctx context.Context, tx *sql.Tx, cfg config) ([]duplicateRow, error) {
	rows, err := tx.QueryContext(ctx, duplicateRowsQuery(cfg.iamSchema), cfg.source, cfg.createdFrom, cfg.createdUntil, cfg.clinicianID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []duplicateRow
	for rows.Next() {
		var row duplicateRow
		if err := rows.Scan(
			&row.TesteeID, &row.ProfileID, &row.ProfileLinkID, &row.UserID,
			&row.CanonicalTestee, &row.CanonicalProfile, &row.OrgID, &row.Name, &row.Gender,
			&row.Birthday, &row.Source, &row.CreatedDate, &row.CreatedAt, &row.GroupSize, &row.DuplicateOrdinal,
			&row.OutcomeCount, &row.EvaluatedCount, &row.SubmittedCount, &row.AssessmentCount,
			&row.CompletedTasks, &row.IntakeCount, &row.EnrollmentCount,
			&row.CanonicalOutcomeCount, &row.CanonicalEvaluatedCount, &row.CanonicalSubmittedCount,
			&row.CanonicalAssessmentCount, &row.CanonicalCompletedTasks, &row.CanonicalIntakeCount,
			&row.CanonicalEnrollmentCount,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func duplicateRowsQuery(iamSchema string) string {
	return fmt.Sprintf(`WITH source_scope AS (
  SELECT t.*
  FROM testee t
  WHERE t.deleted_at IS NULL
    AND t.source = ?
    AND t.created_at >= ?
    AND t.created_at < ?
    AND EXISTS (
      SELECT 1 FROM clinician_relation r
      WHERE r.testee_id = t.id AND r.clinician_id = ?
        AND r.is_active = 1 AND r.deleted_at IS NULL
    )
), link_scope AS (
  SELECT links.profile_id, MIN(links.id) AS profile_link_id, MIN(links.user_id) AS user_id
  FROM %s.profile_links links
  JOIN (SELECT DISTINCT profile_id FROM source_scope) target ON target.profile_id = links.profile_id
  GROUP BY links.profile_id
), base AS (
  SELECT t.id AS testee_id, t.profile_id, ls.profile_link_id, ls.user_id,
         t.org_id, t.name, t.gender,
         COALESCE(DATE_FORMAT(t.birthday, '%%Y-%%m-%%d'), '') AS birthday,
         t.source, DATE_FORMAT(t.created_at, '%%Y-%%m-%%d') AS created_date,
         DATE_FORMAT(t.created_at, '%%Y-%%m-%%d %%H:%%i:%%s') AS created_at
  FROM source_scope t
  JOIN %s.profiles p ON p.id = t.profile_id AND p.deleted_at IS NULL
  JOIN link_scope ls ON ls.profile_id = t.profile_id
  WHERE p.name <=> t.name
    AND p.gender <=> t.gender
    AND p.birthday <=> DATE_FORMAT(t.birthday, '%%Y-%%m-%%d')
), outcome_progress AS (
  SELECT o.testee_id, COUNT(*) AS outcome_count
  FROM evaluation_outcome o
  JOIN base b ON b.testee_id = o.testee_id
  GROUP BY o.testee_id
), assessment_progress AS (
  SELECT a.testee_id,
         SUM(a.status = 'evaluated') AS evaluated_count,
         SUM(a.status = 'submitted') AS submitted_count,
         COUNT(*) AS assessment_count
  FROM assessment a
  JOIN base b ON b.testee_id = a.testee_id
  WHERE a.deleted_at IS NULL
  GROUP BY a.testee_id
), task_progress AS (
  SELECT task.testee_id, COUNT(*) AS completed_tasks
  FROM assessment_task task
  JOIN base b ON b.testee_id = task.testee_id
  WHERE task.status = 'completed' AND task.deleted_at IS NULL
  GROUP BY task.testee_id
), intake_progress AS (
  SELECT intake.testee_id, COUNT(*) AS intake_count
  FROM assessment_entry_intake_log intake
  JOIN base b ON b.testee_id = intake.testee_id
  WHERE intake.deleted_at IS NULL
  GROUP BY intake.testee_id
), enrollment_progress AS (
  SELECT enrollment.testee_id, COUNT(*) AS enrollment_count
  FROM plan_enrollment enrollment
  JOIN base b ON b.testee_id = enrollment.testee_id
  WHERE enrollment.deleted_at IS NULL
  GROUP BY enrollment.testee_id
), eligible AS (
  SELECT b.*,
         COALESCE(o.outcome_count, 0) AS outcome_count,
         COALESCE(a.evaluated_count, 0) AS evaluated_count,
         COALESCE(a.submitted_count, 0) AS submitted_count,
         COALESCE(a.assessment_count, 0) AS assessment_count,
         COALESCE(task.completed_tasks, 0) AS completed_tasks,
         COALESCE(intake.intake_count, 0) AS intake_count,
         COALESCE(enrollment.enrollment_count, 0) AS enrollment_count
  FROM base b
  LEFT JOIN outcome_progress o ON o.testee_id = b.testee_id
  LEFT JOIN assessment_progress a ON a.testee_id = b.testee_id
  LEFT JOIN task_progress task ON task.testee_id = b.testee_id
  LEFT JOIN intake_progress intake ON intake.testee_id = b.testee_id
  LEFT JOIN enrollment_progress enrollment ON enrollment.testee_id = b.testee_id
), ranked AS (
	  SELECT e.*,
	         COUNT(*) OVER duplicate_group AS group_size,
	         ROW_NUMBER() OVER (duplicate_group ORDER BY
           %s
	         ) AS duplicate_ordinal
  FROM eligible e
  WINDOW duplicate_group AS (
    PARTITION BY e.user_id, e.created_date, e.org_id, e.name, e.gender, e.birthday, e.source
  )
), annotated AS (
  SELECT r.*,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.testee_id END) OVER duplicate_group AS canonical_testee_id,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.profile_id END) OVER duplicate_group AS canonical_profile_id,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.outcome_count END) OVER duplicate_group AS canonical_outcome_count,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.evaluated_count END) OVER duplicate_group AS canonical_evaluated_count,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.submitted_count END) OVER duplicate_group AS canonical_submitted_count,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.assessment_count END) OVER duplicate_group AS canonical_assessment_count,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.completed_tasks END) OVER duplicate_group AS canonical_completed_tasks,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.intake_count END) OVER duplicate_group AS canonical_intake_count,
	     MAX(CASE WHEN r.duplicate_ordinal = 1 THEN r.enrollment_count END) OVER duplicate_group AS canonical_enrollment_count
  FROM ranked r
  WINDOW duplicate_group AS (
    PARTITION BY r.user_id, r.created_date, r.org_id, r.name, r.gender, r.birthday, r.source
  )
)
SELECT testee_id, profile_id, profile_link_id, user_id,
       canonical_testee_id, canonical_profile_id, org_id, name, gender,
	   birthday, source, created_date, created_at, group_size, duplicate_ordinal,
	   outcome_count, evaluated_count, submitted_count, assessment_count,
	   completed_tasks, intake_count, enrollment_count,
	   canonical_outcome_count, canonical_evaluated_count, canonical_submitted_count,
	   canonical_assessment_count, canonical_completed_tasks, canonical_intake_count,
	   canonical_enrollment_count
	FROM annotated
	WHERE group_size > 1 AND duplicate_ordinal > 1
	ORDER BY created_date, user_id, name, duplicate_ordinal`, iamSchema, iamSchema, duplicateSelectionOrder)
}

func validateDuplicateRows(rows []duplicateRow) error {
	seenTestees := make(map[uint64]struct{}, len(rows))
	seenProfiles := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		if row.TesteeID == 0 || row.ProfileID == 0 || row.ProfileLinkID == 0 || row.UserID == 0 || row.CanonicalTestee == 0 || row.CanonicalProfile == 0 {
			return fmt.Errorf("zero identifier in duplicate row for testee %d", row.TesteeID)
		}
		if row.CanonicalTestee == row.TesteeID || row.CanonicalProfile == row.ProfileID {
			return fmt.Errorf("duplicate row %d aliases its canonical row", row.TesteeID)
		}
		if row.GroupSize < 2 || row.DuplicateOrdinal < 2 || row.DuplicateOrdinal > row.GroupSize {
			return fmt.Errorf("invalid duplicate rank for testee %d: ordinal=%d group_size=%d", row.TesteeID, row.DuplicateOrdinal, row.GroupSize)
		}
		if duplicateProgressExceedsCanonical(row) {
			return fmt.Errorf("duplicate testee %d has more downstream progress than canonical testee %d", row.TesteeID, row.CanonicalTestee)
		}
		if _, exists := seenTestees[row.TesteeID]; exists {
			return fmt.Errorf("duplicate testee id %d in scope", row.TesteeID)
		}
		seenTestees[row.TesteeID] = struct{}{}
		if _, exists := seenProfiles[row.ProfileID]; exists {
			return fmt.Errorf("profile id %d is shared by multiple selected testees", row.ProfileID)
		}
		seenProfiles[row.ProfileID] = struct{}{}
	}
	return nil
}

func duplicateProgressExceedsCanonical(row duplicateRow) bool {
	duplicate := []int{row.OutcomeCount, row.EvaluatedCount, row.CompletedTasks, row.SubmittedCount, row.AssessmentCount, row.IntakeCount, row.EnrollmentCount}
	canonical := []int{row.CanonicalOutcomeCount, row.CanonicalEvaluatedCount, row.CanonicalCompletedTasks, row.CanonicalSubmittedCount, row.CanonicalAssessmentCount, row.CanonicalIntakeCount, row.CanonicalEnrollmentCount}
	for i := range duplicate {
		if duplicate[i] == canonical[i] {
			continue
		}
		return duplicate[i] > canonical[i]
	}
	return false
}

func writeScopeFiles(cfg config, rows []duplicateRow) error {
	if err := os.MkdirAll(cfg.outputDir, 0o700); err != nil {
		return err
	}
	testeeIDs := make([]uint64, 0, len(rows))
	profileIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		testeeIDs = append(testeeIDs, row.TesteeID)
		profileIDs = append(profileIDs, row.ProfileID)
	}
	sort.Slice(testeeIDs, func(i, j int) bool { return testeeIDs[i] < testeeIDs[j] })
	sort.Slice(profileIDs, func(i, j int) bool { return profileIDs[i] < profileIDs[j] })

	manifest, err := encodeManifest(rows)
	if err != nil {
		return err
	}
	summary := buildSummary(cfg, rows)
	files := map[string][]byte{
		"manifest.csv":    manifest,
		"testee_ids.txt":  encodeIDs(testeeIDs),
		"profile_ids.txt": encodeIDs(profileIDs),
		"summary.txt":     []byte(summary),
	}
	for name := range files {
		path := filepath.Join(cfg.outputDir, name)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for name, data := range files {
		path := filepath.Join(cfg.outputDir, name)
		if err := writeExclusiveFile(path, data); err != nil {
			return err
		}
	}
	return nil
}

func writeExclusiveFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func buildSummary(cfg config, rows []duplicateRow) string {
	groups := map[uint64]struct{}{}
	dateGroups := map[string]map[uint64]struct{}{}
	dateDuplicates := map[string]int{}
	for _, row := range rows {
		groups[row.CanonicalTestee] = struct{}{}
		if dateGroups[row.CreatedDate] == nil {
			dateGroups[row.CreatedDate] = map[uint64]struct{}{}
		}
		dateGroups[row.CreatedDate][row.CanonicalTestee] = struct{}{}
		dateDuplicates[row.CreatedDate]++
	}
	dates := make([]string, 0, len(dateGroups))
	for date := range dateGroups {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	var buf strings.Builder
	fmt.Fprintf(&buf, "source=%s\nclinician_id=%d\ncreated_from=%s\ncreated_until_exclusive=%s\n", cfg.source, cfg.clinicianID, cfg.createdFrom, cfg.createdUntil)
	fmt.Fprintf(&buf, "duplicate_groups=%d\nduplicate_testees=%d\ntotal_testees_in_duplicate_groups=%d\n", len(groups), len(rows), len(groups)+len(rows))
	buf.WriteString("selection=keep highest downstream progress, then earliest created_at,testee_id per user+date+org+name+gender+birthday+source\n")
	for _, date := range dates {
		groupCount := len(dateGroups[date])
		duplicateCount := dateDuplicates[date]
		fmt.Fprintf(&buf, "date=%s groups=%d duplicate_testees=%d total_testees=%d\n", date, groupCount, duplicateCount, groupCount+duplicateCount)
	}
	return buf.String()
}

func encodeIDs(ids []uint64) []byte {
	var buf bytes.Buffer
	for _, id := range ids {
		fmt.Fprintf(&buf, "%d\n", id)
	}
	return buf.Bytes()
}

func encodeManifest(rows []duplicateRow) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{
		"testee_id", "profile_id", "profile_link_id", "user_id",
		"canonical_testee_id", "canonical_profile_id", "org_id", "name", "gender",
		"birthday", "source", "created_date", "created_at", "group_size", "duplicate_ordinal",
		"outcome_count", "evaluated_count", "submitted_count", "assessment_count",
		"completed_tasks", "intake_count", "enrollment_count",
		"canonical_outcome_count", "canonical_evaluated_count", "canonical_submitted_count", "canonical_assessment_count",
		"canonical_completed_tasks", "canonical_intake_count", "canonical_enrollment_count",
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := []string{
			strconv.FormatUint(row.TesteeID, 10), strconv.FormatUint(row.ProfileID, 10), strconv.FormatUint(row.ProfileLinkID, 10), strconv.FormatUint(row.UserID, 10),
			strconv.FormatUint(row.CanonicalTestee, 10), strconv.FormatUint(row.CanonicalProfile, 10), strconv.FormatInt(row.OrgID, 10), row.Name, strconv.Itoa(row.Gender),
			row.Birthday, row.Source, row.CreatedDate, row.CreatedAt, strconv.Itoa(row.GroupSize), strconv.Itoa(row.DuplicateOrdinal),
			strconv.Itoa(row.OutcomeCount), strconv.Itoa(row.EvaluatedCount), strconv.Itoa(row.SubmittedCount), strconv.Itoa(row.AssessmentCount),
			strconv.Itoa(row.CompletedTasks), strconv.Itoa(row.IntakeCount), strconv.Itoa(row.EnrollmentCount),
			strconv.Itoa(row.CanonicalOutcomeCount), strconv.Itoa(row.CanonicalEvaluatedCount), strconv.Itoa(row.CanonicalSubmittedCount), strconv.Itoa(row.CanonicalAssessmentCount),
			strconv.Itoa(row.CanonicalCompletedTasks), strconv.Itoa(row.CanonicalIntakeCount), strconv.Itoa(row.CanonicalEnrollmentCount),
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
