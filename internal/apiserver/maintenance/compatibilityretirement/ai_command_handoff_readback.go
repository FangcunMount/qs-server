package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
)

// Exported struct fields are used only inside the private integrity digest.
// The original ordinal metadata, nullable defaults and hashes stay exact.
type aiCommandHandoffAnchor struct{ ID, Org, Testee, Sheet, RowSHA256 string }

type aiReverseMetadataSeal struct {
	Columns            []string
	SourceColumns      SQLColumns
	Schema, PrimaryKey string
}

func aiCommandHandoffMetadataSeal(metadata []aiReverseMetadata) []aiReverseMetadataSeal {
	out := make([]aiReverseMetadataSeal, len(metadata))
	for i, m := range metadata {
		out[i] = aiReverseMetadataSeal{m.columns, m.sourceColumns, m.schema, m.pk}
	}
	return out
}

// lockCurrentLedgers is called exactly once per batch. The projection, tables,
// keys and query structure come only from the closed real AIReverse catalog.
// Full pagination reaches actual EOF, including insertions above the old bound;
// neither approved counts nor DTO completion flags substitute for this read.
func (p *AICommandHandoffBatch) lockCurrentLedgers(ctx context.Context, resolver *AILocalResolver) (map[string]aiReverseRow, error) {
	probe := &AIReverseSnapshot{pool: resolver.pool, started: p.started, limits: p.limits}
	before := make(map[string]aiReverseRow, len(p.evidence))
	wanted := make(map[string]bool, len(p.evidence))
	for _, e := range p.evidence {
		wanted[e.CommandID] = true
	}
	var totalRows, totalBytes uint64
	for i, spec := range aiReverseSpecs {
		if timeExpiredHandoff(ctx, p) {
			return nil, ErrAILocalBounds
		}
		meta, err := probe.schema(ctx, spec)
		if err != nil || !aiCommandHandoffMetadataEqual([]aiReverseMetadata{meta}, p.metadata[i:i+1]) {
			return nil, ErrAILocalChanged
		}
		ledger := p.ledgers[i]
		if ledger.Store != spec.table || ledger.SchemaSHA256 != meta.schema || ledger.PrimaryKeySHA256 != meta.pk {
			return nil, ErrAILocalBinding
		}
		projection, order := aiCommandHandoffProjection(spec)
		hash := sha256.New()
		sourceFrame(hash, []byte("ai-reverse-ledger/v1"), false)
		sourceFrame(hash, []byte(meta.schema), false)
		var count, size uint64
		var after []string
		for {
			query := "SELECT " + projection + " FROM `" + spec.table + "` FORCE INDEX(PRIMARY)"
			var args []any
			if after != nil {
				predicate, values := aiReversePredicate(spec, after, false)
				query += " WHERE " + predicate
				args = append(args, values...)
			}
			query += " ORDER BY " + order + " LIMIT ? FOR UPDATE"
			args = append(args, p.limits.PageRows)
			rows, names, bytesRead, err := probe.read(ctx, query, p.limits.PageRows, args...)
			if err != nil || !reflect.DeepEqual(names, meta.columns) {
				return nil, ErrAILocalRead
			}
			if bytesRead > p.limits.MaxBytes-totalBytes || uint64(len(rows)) > p.limits.MaxRows-totalRows {
				return nil, ErrAILocalBounds
			}
			totalBytes += bytesRead
			totalRows += uint64(len(rows))
			size += bytesRead
			count += uint64(len(rows))
			for _, row := range rows {
				key, err := aiReverseKey(spec, row)
				if err != nil || after != nil && aiReverseCompare(spec, after, key) >= 0 {
					return nil, ErrAILocalChanged
				}
				after = key
				sourceFrame(hash, []byte(aiReverseRowSHA(meta.columns, row)), false)
				if spec.table == "ai_messaging_operations" && wanted[row.text("command_id")] {
					if before[row.text("command_id")] != nil {
						return nil, ErrAILocalChanged
					}
					before[row.text("command_id")] = row
				}
			}
			if timeExpiredHandoff(ctx, p) {
				return nil, ErrAILocalBounds
			}
			if len(rows) == 0 {
				break
			}
		}
		if count != ledger.Rows || size != ledger.Bytes || aiReverseKeySHA(after) != ledger.UpperSHA256 || hex.EncodeToString(hash.Sum(nil)) != ledger.RowsSHA256 {
			return nil, ErrAILocalChanged
		}
	}
	for _, e := range p.evidence {
		if (before[e.CommandID] != nil) != (e.Conclusion == "transferred_verified") {
			return nil, ErrAILocalChanged
		}
	}
	if err := p.lockBusinessAnchors(ctx, probe, p.limits.MaxBytes-totalBytes); err != nil {
		return nil, err
	}
	if err := p.verifyCurrentMetadata(ctx, resolver); err != nil {
		return nil, err
	}
	return before, nil
}

func aiCommandHandoffProjection(spec aiReverseSpec) (string, string) {
	projection := make([]string, 0, len(spec.columns))
	order := make([]string, 0, len(spec.keys))
	for _, col := range spec.columns {
		projection = append(projection, "CAST(`"+col+"` AS BINARY) AS `"+col+"`")
	}
	for _, key := range spec.keys {
		order = append(order, "`"+key+"`")
	}
	return strings.Join(projection, ","), strings.Join(order, ",")
}

func (p *AICommandHandoffBatch) verifyCurrentMetadata(ctx context.Context, resolver *AILocalResolver) error {
	if timeExpiredHandoff(ctx, p) {
		return ErrAILocalBounds
	}
	if err := resolver.verifyBinding(ctx); err != nil {
		return err
	}
	probe := &AIReverseSnapshot{pool: resolver.pool, started: p.started, limits: p.limits}
	for i, spec := range aiReverseSpecs {
		meta, err := probe.schema(ctx, spec)
		if err != nil || !aiCommandHandoffMetadataEqual([]aiReverseMetadata{meta}, p.metadata[i:i+1]) {
			return ErrAILocalChanged
		}
	}
	_, anchorSchema, err := probe.assessmentMetadata(ctx)
	if err != nil || anchorSchema != p.anchorMetadataSHA {
		return ErrAILocalChanged
	}
	return nil
}

func (p *AICommandHandoffBatch) verifyWrittenOperation(ctx context.Context, resolver *AILocalResolver, before aiReverseRow, expected store.CommandRetirementEvidence) error {
	if timeExpiredHandoff(ctx, p) {
		return ErrAILocalChanged
	}
	spec := aiReverseSpecByTable("ai_messaging_operations")
	projection, _ := aiCommandHandoffProjection(spec)
	probe := &AIReverseSnapshot{pool: resolver.pool, started: p.started, limits: p.limits}
	rows, names, _, err := probe.read(ctx, "SELECT "+projection+" FROM `ai_messaging_operations` WHERE command_id=? FOR UPDATE", 2, expected.CommandID)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(names, spec.columns) {
		return ErrAILocalChanged
	}
	if before == nil {
		return aiCommandRetirementOperationExpected(spec.columns, rows[0], expected)
	}
	return aiCommandHandoffOperationUnchanged(spec.columns, before, rows[0], expected)
}

func aiCommandHandoffOperationUnchanged(columns []string, before, after aiReverseRow, expected store.CommandRetirementEvidence) error {
	if before == nil || after == nil || len(before) != len(columns) || len(after) != len(columns) || after.text("command_id") != expected.CommandID {
		return ErrAILocalChanged
	}
	for _, col := range columns {
		left, leftOK := before[col]
		right, rightOK := after[col]
		if !leftOK || !rightOK {
			return ErrAILocalChanged
		}
		if col == "retirement_evidence" {
			continue
		}
		if (left == nil) != (right == nil) || !bytes.Equal(left, right) {
			return ErrAILocalChanged
		}
	}
	// MySQL JSON canonical storage is not Go's source encoding. Decode only
	// this dedicated evidence column, with EOF and exact typed field equality;
	// every other original column remains a byte/null comparison above.
	if after["retirement_evidence"] == nil {
		return ErrAILocalChanged
	}
	decoder := json.NewDecoder(bytes.NewReader(after["retirement_evidence"]))
	decoder.DisallowUnknownFields()
	var actual store.CommandRetirementEvidence
	if decoder.Decode(&actual) != nil || decoder.Decode(new(json.RawMessage)) != io.EOF || !reflect.DeepEqual(actual, expected) || actual.ResponsibilityClosed || actual.BusinessTerminal || actual.Conclusion != "transferred_verified" || actual.VerificationMethod != "source_identity_hash_and_live_ledger" {
		return ErrAILocalChanged
	}
	return nil
}

func timeExpiredHandoff(ctx context.Context, p *AICommandHandoffBatch) bool {
	return ctx == nil || ctx.Err() != nil || p == nil || time.Now().After(p.expires)
}

// The original full reverse derives business ownership from these real rows.
// Lock/recheck their complete nullable byte baselines too, not merely the IDs
// copied from the request. Only original numeric IDs enter these fixed reads.
func (p *AICommandHandoffBatch) lockBusinessAnchors(ctx context.Context, probe *AIReverseSnapshot, bytesRemaining uint64) error {
	columns, schema, err := probe.assessmentMetadata(ctx)
	if err != nil || schema != p.anchorMetadataSHA {
		return ErrAILocalChanged
	}
	projection := make([]string, len(columns))
	for i, col := range columns {
		projection[i] = "CAST(`" + col + "` AS BINARY) AS `" + col + "`"
	}
	for offset := 0; offset < len(p.anchors); offset += p.limits.PageRows {
		end := offset + p.limits.PageRows
		if end > len(p.anchors) {
			end = len(p.anchors)
		}
		args := make([]any, 0, end-offset)
		place := make([]string, 0, end-offset)
		for _, original := range p.anchors[offset:end] {
			if !aiPositiveNumber(original.ID) {
				return ErrAILocalBinding
			}
			id, err := strconv.ParseUint(original.ID, 10, 64)
			if err != nil {
				return ErrAILocalBinding
			}
			args = append(args, id)
			place = append(place, "?")
		}
		rows, names, size, err := probe.read(ctx, "SELECT "+strings.Join(projection, ",")+" FROM assessment FORCE INDEX(PRIMARY) WHERE id IN ("+strings.Join(place, ",")+") ORDER BY id FOR UPDATE", end-offset, args...)
		if err != nil || len(rows) != end-offset || !reflect.DeepEqual(names, columns) {
			return ErrAILocalChanged
		}
		if size > bytesRemaining {
			return ErrAILocalBounds
		}
		bytesRemaining -= size
		for i, row := range rows {
			original := p.anchors[offset+i]
			if row.text("id") != original.ID || row.text("org_id") != original.Org || row.text("testee_id") != original.Testee || row.text("answer_sheet_id") != original.Sheet || row["deleted_at"] != nil || aiReverseRowSHA(columns, row) != original.RowSHA256 {
				return ErrAILocalChanged
			}
		}
		if timeExpiredHandoff(ctx, p) {
			return ErrAILocalBounds
		}
	}
	return nil
}
