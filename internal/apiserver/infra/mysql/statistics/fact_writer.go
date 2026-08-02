package statistics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type factWriter struct{ db *gorm.DB }

type factWriteDisposition uint8

const (
	factWriteUnprocessed factWriteDisposition = iota
	factWriteInserted
	factWriteExisting
	factWriteConflict
	factWriteFailed
)

type factCandidate struct {
	SourceID uint64
	FactType string
	Values   map[string]any
}

type storedFactHash struct {
	FactKey  string `gorm:"column:fact_key"`
	CoreHash string `gorm:"column:core_hash"`
}

type pendingFact struct {
	index  int
	values map[string]any
}

func (w factWriter) write(ctx context.Context, table string, values map[string]any, validateOnly bool) (inserted, existing, conflict int64, err error) {
	coreHash := hashCore(values)
	values["core_hash"] = coreHash
	var stored struct{ CoreHash string }
	lookup := w.db.WithContext(ctx).Table(table).Select("core_hash").Where("fact_key = ?", values["fact_key"]).Take(&stored).Error
	if lookup == nil {
		if stored.CoreHash == coreHash {
			return 0, 1, 0, nil
		}
		return 0, 0, 1, fmt.Errorf("fact conflict: %s", values["fact_key"])
	}
	if lookup != gorm.ErrRecordNotFound {
		return 0, 0, 0, lookup
	}
	if validateOnly {
		return 1, 0, 0, nil
	}
	if err := w.db.WithContext(ctx).Table(table).Create(values).Error; err != nil {
		if mysql.IsDuplicateError(err) {
			return w.write(ctx, table, values, true)
		}
		return 0, 0, 0, err
	}
	return 1, 0, 0, nil
}

// writeBatch preserves the per-fact idempotency and conflict contract while
// collapsing the successful path into one hash lookup and batched inserts.
// Error paths fall back to the original per-row writer so partial progress and
// error classification remain compatible with historical repair retries.
func (w factWriter) writeBatch(ctx context.Context, table string, facts []map[string]any, validateOnly bool) ([]factWriteDisposition, error) {
	dispositions := make([]factWriteDisposition, len(facts))
	if len(facts) == 0 {
		return dispositions, nil
	}
	if !isFactTable(table) {
		dispositions[0] = factWriteFailed
		return dispositions, fmt.Errorf("unsupported statistics fact table %q", table)
	}

	keys := make([]string, 0, len(facts))
	uniqueKeys := make(map[string]struct{}, len(facts))
	hashes := make([]string, len(facts))
	for index, values := range facts {
		key, ok := values["fact_key"].(string)
		if !ok || strings.TrimSpace(key) == "" {
			dispositions[index] = factWriteFailed
			return dispositions, fmt.Errorf("statistics fact key is required")
		}
		hashes[index] = hashCore(values)
		values["core_hash"] = hashes[index]
		if _, exists := uniqueKeys[key]; !exists {
			uniqueKeys[key] = struct{}{}
			keys = append(keys, key)
		}
	}

	var storedRows []storedFactHash
	if err := w.db.WithContext(ctx).Table(table).Select("fact_key,core_hash").Where("fact_key IN ?", keys).Find(&storedRows).Error; err != nil {
		dispositions[0] = factWriteFailed
		return dispositions, err
	}
	known := make(map[string]string, len(storedRows)+len(facts))
	for _, row := range storedRows {
		known[row.FactKey] = row.CoreHash
	}

	pending := make([]pendingFact, 0, len(facts))
	var conflictErr error
	for index, values := range facts {
		key := values["fact_key"].(string)
		if coreHash, exists := known[key]; exists {
			if coreHash == hashes[index] {
				dispositions[index] = factWriteExisting
				continue
			}
			dispositions[index] = factWriteConflict
			conflictErr = fmt.Errorf("fact conflict: %s", key)
			break
		}
		known[key] = hashes[index]
		if validateOnly {
			dispositions[index] = factWriteInserted
			continue
		}
		pending = append(pending, pendingFact{index: index, values: values})
	}

	if !validateOnly {
		if err := w.insertPending(ctx, table, pending, dispositions); err != nil {
			return dispositions, err
		}
	}
	return dispositions, conflictErr
}

func (w factWriter) insertPending(ctx context.Context, table string, pending []pendingFact, dispositions []factWriteDisposition) error {
	for start := 0; start < len(pending); {
		signature, columns, err := factColumnSignature(pending[start].values)
		if err != nil {
			dispositions[pending[start].index] = factWriteFailed
			return err
		}
		end := start + 1
		for end < len(pending) && end-start < collectorBatchSize {
			nextSignature, _, signatureErr := factColumnSignature(pending[end].values)
			if signatureErr != nil || nextSignature != signature {
				break
			}
			end++
		}
		group := pending[start:end]
		if err := w.bulkInsert(ctx, table, columns, group); err != nil {
			if fallbackErr := w.insertSequential(ctx, table, group, dispositions); fallbackErr != nil {
				clearUnprocessedAfterFailure(dispositions, group)
				return fallbackErr
			}
		} else {
			for _, item := range group {
				dispositions[item.index] = factWriteInserted
			}
		}
		start = end
	}
	return nil
}

func clearUnprocessedAfterFailure(dispositions []factWriteDisposition, group []pendingFact) {
	failureIndex := -1
	for _, item := range group {
		if dispositions[item.index] == factWriteFailed || dispositions[item.index] == factWriteConflict {
			failureIndex = item.index
			break
		}
	}
	if failureIndex < 0 {
		return
	}
	for index := failureIndex + 1; index < len(dispositions); index++ {
		dispositions[index] = factWriteUnprocessed
	}
}

func (w factWriter) bulkInsert(ctx context.Context, table string, columns []string, pending []pendingFact) error {
	var query strings.Builder
	query.WriteString("INSERT INTO `")
	query.WriteString(table)
	query.WriteString("` (`")
	query.WriteString(strings.Join(columns, "`,`"))
	query.WriteString("`) VALUES ")

	args := make([]any, 0, len(columns)*len(pending))
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
	for rowIndex, item := range pending {
		if rowIndex > 0 {
			query.WriteByte(',')
		}
		query.WriteString(rowPlaceholder)
		for _, column := range columns {
			args = append(args, item.values[column])
		}
	}
	result := w.db.WithContext(ctx).Exec(query.String(), args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(pending)) {
		return fmt.Errorf("statistics fact batch inserted %d rows, expected %d", result.RowsAffected, len(pending))
	}
	return nil
}

func (w factWriter) insertSequential(ctx context.Context, table string, pending []pendingFact, dispositions []factWriteDisposition) error {
	for _, item := range pending {
		inserted, existing, conflict, err := w.write(ctx, table, item.values, false)
		switch {
		case conflict > 0:
			dispositions[item.index] = factWriteConflict
		case inserted > 0:
			dispositions[item.index] = factWriteInserted
		case existing > 0:
			dispositions[item.index] = factWriteExisting
		default:
			dispositions[item.index] = factWriteFailed
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func factColumnSignature(values map[string]any) (string, []string, error) {
	columns := make([]string, 0, len(values))
	for column := range values {
		if !isFactColumn(column) {
			return "", nil, fmt.Errorf("unsupported statistics fact column %q", column)
		}
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return strings.Join(columns, ","), columns, nil
}

func isFactTable(table string) bool {
	switch table {
	case "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact":
		return true
	default:
		return false
	}
}

func isFactColumn(column string) bool {
	if column == "" {
		return false
	}
	for _, value := range column {
		if (value < 'a' || value > 'z') && value != '_' {
			return false
		}
	}
	return true
}

func writeFactCandidates(
	ctx context.Context,
	writer factWriter,
	table string,
	candidates []factCandidate,
	validateOnly bool,
	result *statisticsDomain.CollectResult,
) error {
	seenSources := make(map[uint64]struct{}, len(candidates))
	for start := 0; start < len(candidates); {
		end := start + collectorBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		for end < len(candidates) && candidates[end].SourceID == candidates[end-1].SourceID {
			end++
		}
		values := make([]map[string]any, end-start)
		for index := start; index < end; index++ {
			values[index-start] = candidates[index].Values
		}
		dispositions, err := writer.writeBatch(ctx, table, values, validateOnly)
		for offset, disposition := range dispositions {
			if disposition == factWriteUnprocessed {
				continue
			}
			candidate := candidates[start+offset]
			if _, exists := seenSources[candidate.SourceID]; !exists {
				seenSources[candidate.SourceID] = struct{}{}
				result.SourceCount++
			}
			result.FactTypeCounts[candidate.FactType]++
			switch disposition {
			case factWriteInserted:
				result.InsertedCount++
			case factWriteExisting:
				result.ExistingCount++
			case factWriteConflict:
				result.ConflictCount++
			}
		}
		if err != nil {
			return err
		}
		start = end
	}
	return nil
}
