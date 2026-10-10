package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// This closed descriptor is derived from authenticated original source frames
// and the actual complete local snapshot. It is never a caller's target list or
// provider-terminal claim. The fixed external producer independently verifies
// its entire mapping set against both actual peer epochs.
type aiExternalKnownHandoff struct {
	CommandID        string `json:"command_id"`
	RequestID        string `json:"request_id"`
	SourceKind       string `json:"source_kind"`
	OrganizationID   string `json:"organization_id"`
	SubjectID        string `json:"subject_id"`
	ResourceID       string `json:"resource_id"`
	TesteeID         string `json:"testee_id"`
	BridgePayloadSHA string `json:"bridge_payload_bytes_sha256"`
	LegacyPayloadSHA string `json:"legacy_payload_bytes_sha256"`
	WriterSHA        string `json:"writer_payload_sha256"`
	MessagingSHA     string `json:"messaging_body_sha256"`
	SourceAttempts   uint64 `json:"source_attempts"`
	Sequence         uint64 `json:"aggregate_sequence"`
	OperationRowSHA  string `json:"operation_row_sha256"`
	OutboxRowSHA     string `json:"outbox_row_sha256"`
}

func aiExternalActualHandoffs(ctx context.Context, c *HistoricalCoordinator, reverse *AIReverseSnapshot) ([]aiExternalKnownHandoff, error) {
	if ctx == nil || c == nil || c.authenticated == nil || !c.authenticated.complete || reverse == nil || reverse.scope == nil || reverse.scope.owner != c || reverse.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAILocalBinding
	}
	commands := [2]map[string]*DecodedAICommand{make(map[string]*DecodedAICommand), make(map[string]*DecodedAICommand)}
	for n := 0; n < 2; n++ {
		copy := c.copies[n+1]
		at, ok := copy.Input.(io.ReaderAt)
		if !ok || copy.Expected.Records > 10000 {
			return nil, ErrAIExternalInput
		}
		reader, err := NewAISQLSourceReader(io.NewSectionReader(at, 0, math.MaxInt64), copy.Expected)
		if err != nil {
			return nil, err
		}
		for {
			value, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			bound, err := c.authenticated.BindAICommand(value)
			if err != nil {
				return nil, err
			}
			actual, err := bound.Facts()
			if err != nil || commands[n][actual.CommandID] != nil {
				return nil, ErrSourceAuthentication
			}
			commands[n][actual.CommandID] = actual
		}
		if reader.Receipt() != c.authenticated.receipts[n+1] {
			return nil, ErrSourceIncomplete
		}
	}
	if len(commands[0]) != len(reverse.byTable[AIBridgeCommandSource]) || len(commands[1]) != len(reverse.byTable[AILegacyCommandSource]) {
		return nil, ErrAIExternalChanged
	}
	out := make([]aiExternalKnownHandoff, 0, len(commands[1]))
	for id, legacy := range commands[1] {
		bridge := commands[0][id]
		if bridge == nil || ValidateAISourcePair(bridge, legacy) != nil || aiCommandHandoffNodes(reverse, bridge, legacy) != nil {
			return nil, ErrAILocalResponsibility
		}
		request, op, box := reverse.byTable["ai_bridge_requests"][bridge.RequestID], reverse.byTable["ai_messaging_operations"][id], reverse.byTable["ai_messaging_outbox"][id]
		if request == nil || op == nil || box == nil || !evidenceHash(op.observation.RowSHA256) || !evidenceHash(box.observation.RowSHA256) {
			return nil, ErrAIExternalChanged
		}
		out = append(out, aiExternalKnownHandoff{CommandID: id, RequestID: bridge.RequestID, SourceKind: bridge.SourceKind, OrganizationID: bridge.OrganizationID, SubjectID: bridge.SubjectID, ResourceID: bridge.ResourceID, TesteeID: request.testee, BridgePayloadSHA: bridge.PayloadBytesDigest.SHA256, LegacyPayloadSHA: legacy.PayloadBytesDigest.SHA256, WriterSHA: bridge.WriterPayloadDigest.SHA256, MessagingSHA: legacy.Transport.MessagingBodySHA256, SourceAttempts: uint64(bridge.Transport.SourceAttempts), Sequence: op.sequence, OperationRowSHA: op.observation.RowSHA256, OutboxRowSHA: box.observation.RowSHA256})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CommandID < out[j].CommandID })
	if reverse.ValidateBorrowedSnapshot(ctx) != nil || c.alive(ctx) != nil {
		return nil, ErrAIExternalChanged
	}
	return out, nil
}

func aiExternalPersistenceGuard(ctx context.Context, c *HistoricalCoordinator, page *HistoricalSourcePage, reverse *AIReverseSnapshot) (*AICommandHandoffBatch, error) {
	if c == nil || c.pageValid(page) != nil || reverse == nil || reverse.self != reverse || reverse.scope == nil || reverse.scope.owner != c || reverse.scope.auth != c.authenticated || reverse.ValidateBorrowedSnapshot(ctx) != nil || len(reverse.metadata) != len(aiReverseSpecs) || len(reverse.structuralReasons) != 0 || reverse.report.Unknown != 0 {
		return nil, ErrAILocalBinding
	}
	admission := reverse.byTable["ai_messaging_admission"]["1"]
	if admission == nil || admission.state != "closed" {
		return nil, ErrAILocalBinding
	}
	binding := AIResolverBinding{DatabaseIdentityHash: reverse.report.DatabaseIdentitySHA256, MigrationVersion: reverse.head, SourceSHA: c.binding.SourceSHA, OperationID: c.binding.OperationID, AdmissionRevision: admission.sequence, BridgeBoundary: c.copies[1].Expected.Boundary, LegacyBoundary: c.copies[2].Expected.Boundary}
	p := &AICommandHandoffBatch{binding: binding, oldPool: reverse.pool, started: reverse.started, expires: reverse.started.Add(reverse.limits.MaxDuration), limits: reverse.limits, copies: reverse.scope.receipts, ledgers: append([]AIReverseLedgerSummary(nil), reverse.report.Ledgers...), anchorMetadataSHA: reverse.anchorMetadataSHA}
	for i, meta := range reverse.metadata {
		p.metadata = append(p.metadata, aiReverseMetadata{columns: append([]string(nil), meta.columns...), sourceColumns: aiCloneColumns(meta.sourceColumns), schema: meta.schema, pk: meta.pk})
		switch aiReverseSpecs[i].table {
		case AIBridgeCommandSource:
			p.binding.BridgeColumns = aiCloneColumns(meta.sourceColumns)
		case AILegacyCommandSource:
			p.binding.LegacyColumns = aiCloneColumns(meta.sourceColumns)
		}
	}
	for _, anchor := range reverse.anchors {
		p.anchors = append(p.anchors, aiCommandHandoffAnchor{anchor.id, anchor.org, anchor.testee, anchor.sheet, anchor.rawSHA})
	}
	sort.Slice(p.anchors, func(i, j int) bool {
		a, _ := strconv.ParseUint(p.anchors[i].ID, 10, 64)
		b, _ := strconv.ParseUint(p.anchors[j].ID, 10, 64)
		return a < b
	})
	for _, deadline := range []time.Time{c.started.Add(c.limits.MaxDuration), page.issued.Add(c.limits.PageTTL)} {
		if deadline.Before(p.expires) {
			p.expires = deadline
		}
	}
	if !aiValidBinding(p.binding) || time.Now().After(p.expires) {
		return nil, ErrAILocalBounds
	}
	return p, nil
}

// Both subsets are produced from one genuine pending page. The complete page,
// original physical source identities and budget facts remain covered; a mapped
// row is not silently filtered out of the retirement operation.
func (c *HistoricalCoordinator) PrepareAICommandPersistencePage(ctx context.Context, page *HistoricalSourcePage, q *AIExternalExecutionQualification) (*AIExternalPageQualification, error) {
	if c == nil || ctx == nil {
		return nil, ErrAIExternalInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pageValid(page) != nil || q == nil || q.self != q || q.owner != c || q.seal != aiJSONHash(q.facts) || q.local.validate(ctx) != nil || q.reverse.ValidateBorrowedSnapshot(ctx) != nil || time.Now().After(q.expires) {
		return nil, ErrAIExternalChanged
	}
	var mapped, unmapped []coordinatorRow
	for _, row := range page.rows {
		if row.event != nil || row.bridge == nil {
			return nil, ErrCoordinatorPage
		}
		if row.legacy != nil {
			mapped = append(mapped, row)
		} else {
			unmapped = append(unmapped, row)
		}
	}
	p := &AIExternalPageQualification{owner: q, page: page, covered: make(map[verifiedSourceKey]string)}
	if len(unmapped) > 0 {
		part, err := c.prepareAIExternalRowsLocked(ctx, page, q, unmapped)
		if err != nil {
			return nil, err
		}
		p.evidence = part.evidence
	}
	if len(mapped) > 0 {
		part, err := c.prepareAICommandHandoffRowsLocked(ctx, page, q.reverse, mapped)
		if err != nil {
			return nil, err
		}
		for _, row := range mapped {
			actual, err := row.bridge.Facts()
			if err != nil {
				return nil, err
			}
			expected, ok := q.handoffs[actual.CommandID]
			if !ok || expected.CommandID != actual.CommandID || expected.RequestID != actual.RequestID || expected.BridgePayloadSHA != actual.PayloadBytesDigest.SHA256 || expected.WriterSHA != actual.WriterPayloadDigest.SHA256 || expected.SourceKind != actual.SourceKind || expected.OrganizationID != actual.OrganizationID || expected.SubjectID != actual.SubjectID || expected.ResourceID != actual.ResourceID || expected.SourceAttempts != uint64(actual.Transport.SourceAttempts) {
				return nil, ErrAIExternalChanged
			}
		}
		p.handoff = part
	}
	guard, err := aiExternalPersistenceGuard(ctx, c, page, q.reverse)
	if err != nil {
		return nil, err
	}
	if q.expires.Before(guard.expires) {
		guard.expires = q.expires
	}
	p.guard = guard
	for _, row := range page.rows {
		for _, handle := range []*VerifiedSourceAICommand{row.bridge, row.legacy} {
			if handle == nil {
				continue
			}
			facts, err := handle.Facts()
			if err != nil {
				return nil, err
			}
			key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
			hash, he := privateFactsSHA(facts)
			bound, ok := c.authenticated.rows[key]
			if err != nil || he != nil || !ok || hash != bound.facts || p.covered[key] != "" {
				return nil, ErrSourceAuthentication
			}
			p.covered[key] = hex.EncodeToString(hash[:])
		}
	}
	p.self = p
	p.seal = p.persistenceDigest()
	return p, nil
}

func (p *AIExternalPageQualification) persistenceDigest() string {
	if p == nil || p.guard == nil || p.owner == nil {
		return ""
	}
	type entry struct {
		Object    uint8
		PK, Facts string
	}
	keys := make([]entry, 0, len(p.covered))
	for k, v := range p.covered {
		keys = append(keys, entry{k.object, hex.EncodeToString(k.pk[:]), v})
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Object != keys[j].Object {
			return keys[i].Object < keys[j].Object
		}
		return keys[i].PK < keys[j].PK
	})
	handoff := ""
	if p.handoff != nil {
		handoff = p.handoff.seal
	}
	return aiJSONHash(struct {
		External, Guard, Handoff string
		Sequence                 uint64
		Keys                     []entry
		Evidence                 []store.CommandRetirementEvidence
	}{p.owner.seal, p.guard.digest(), handoff, p.page.sequence, keys, p.evidence})
}

type AICommandPersistenceBatch struct {
	mu               sync.Mutex
	self             *AICommandPersistenceBatch
	owner            *HistoricalCoordinator
	guard            *AICommandHandoffBatch
	pages            []*AIExternalPageQualification
	expected         uint64
	seal             string
	attempted        bool
	writePool        gorm.ConnPool
	afterLedgers     []AIReverseLedgerSummary
	written          map[string]string
	afterSeal        string
	historical       *AIExternalExecutionQualification
	readbackPool     gorm.ConnPool
	readbackSeal     string
	readbackSnapshot *AIReverseSnapshot
}

type AICommandPersistenceSummary struct {
	OriginalCommands, OriginalSourceReferences                       uint64
	InTransactionWritten, IndependentReadbackMatched                 bool
	ProviderCompletion, ProductionAuthority, CASAuthority, DropReady bool
}

func (*AICommandPersistenceBatch) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AICommandPersistenceBatch) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AICommandPersistenceBatch) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*AICommandPersistenceBatch) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (*AICommandPersistenceBatch) String() string {
	return "private complete original-command persistence batch; no production or DROP authority"
}

// Seal requires authentic complete source consumption, not a caller count.
// All pages are merged before one complete current locking read in Record.
func (c *HistoricalCoordinator) SealAICommandPersistencePages(ctx context.Context, pages []*AIExternalPageQualification) (*AICommandPersistenceBatch, error) {
	if c == nil || ctx == nil {
		return nil, ErrAILocalBinding
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alive(ctx) != nil || c.pending != nil || len(c.remaining) != 0 || !c.coverage {
		return nil, ErrSourceIncomplete
	}
	expected := c.authenticated.receipts[1].Records + c.authenticated.receipts[2].Records
	if expected == 0 && len(pages) == 0 {
		return nil, nil
	}
	if len(pages) == 0 {
		return nil, ErrSourceIncomplete
	}
	b := &AICommandPersistenceBatch{owner: c, pages: append([]*AIExternalPageQualification(nil), pages...), expected: expected, written: make(map[string]string)}
	seen := make(map[verifiedSourceKey]bool)
	commands := make(map[string]bool)
	for _, p := range pages {
		if p == nil || p.self != p || p.owner == nil || p.owner.owner != c || p.page == nil || p.page.owner != c || !p.page.consumed || p.seal == "" || p.seal != p.persistenceDigest() || p.owner.local.validate(ctx) != nil || p.owner.reverse.ValidateBorrowedSnapshot(ctx) != nil {
			return nil, ErrAILocalChanged
		}
		if b.guard == nil {
			b.guard = aiPersistenceCloneGuard(p.guard)
		} else if !aiPersistenceGuardEqual(b.guard, p.guard) {
			return nil, ErrAILocalChanged
		}
		if p.guard.expires.Before(b.guard.expires) {
			b.guard.expires = p.guard.expires
		}
		for k, v := range p.covered {
			row, ok := c.authenticated.rows[k]
			if !ok || seen[k] || hex.EncodeToString(row.facts[:]) != v {
				return nil, ErrSourceAuthentication
			}
			seen[k] = true
		}
		all := append([]store.CommandRetirementEvidence(nil), p.evidence...)
		if p.handoff != nil {
			if !p.handoff.intact() {
				return nil, ErrAILocalChanged
			}
			all = append(all, p.handoff.evidence...)
		}
		for _, e := range all {
			if commands[e.CommandID] {
				return nil, ErrSourceIdentity
			}
			commands[e.CommandID] = true
			b.guard.evidence = append(b.guard.evidence, e)
		}
	}
	if uint64(len(seen)) != expected || len(commands) != int(c.authenticated.receipts[1].Records) {
		return nil, ErrSourceIncomplete
	}
	b.guard.self = b.guard
	b.guard.seal = b.guard.digest()
	b.self = b
	b.seal = b.digest()
	if !b.intact() {
		return nil, ErrAILocalChanged
	}
	return b, nil
}

func aiPersistenceGuardEqual(a, b *AICommandHandoffBatch) bool {
	return a != nil && b != nil && a.oldPool == b.oldPool && reflect.DeepEqual(a.binding, b.binding) && a.started.Equal(b.started) && a.limits == b.limits && reflect.DeepEqual(a.metadata, b.metadata) && reflect.DeepEqual(a.ledgers, b.ledgers) && reflect.DeepEqual(a.anchors, b.anchors) && a.anchorMetadataSHA == b.anchorMetadataSHA && a.copies == b.copies
}
func (b *AICommandPersistenceBatch) digest() string {
	if b == nil || b.guard == nil {
		return ""
	}
	seals := make([]string, len(b.pages))
	for i, p := range b.pages {
		if p == nil {
			return ""
		}
		seals[i] = p.seal
	}
	return aiJSONHash(struct {
		Guard    string
		Pages    []string
		Expected uint64
	}{b.guard.digest(), seals, b.expected})
}
func (b *AICommandPersistenceBatch) intact() bool {
	return b != nil && b.self == b && b.guard != nil && (b.guard.intact() || b.historicalEmptyGuardIntact()) && (b.owner != nil && b.historical == nil || b.owner == nil && b.historical != nil && b.historical.self == b.historical) && b.seal != "" && b.seal == b.digest()
}

// This is an in-transaction caller, not a commit or writer-fence permission.
// Mapped entries use the identical handoff recording core as public Record;
// unmapped entries can only occupy their verified ORIGINAL command ID.
func (b *AICommandPersistenceBatch) Record(ctx context.Context, tx *gorm.DB) (AICommandPersistenceSummary, error) {
	if b == nil || b.self != b {
		return AICommandPersistenceSummary{}, ErrAILocalBinding
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.intact() || b.attempted || ctx == nil || ctx.Err() != nil || time.Now().After(b.guard.expires) {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	if b.historical != nil {
		if tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
			return AICommandPersistenceSummary{}, ErrAILocalTransaction
		}
		if b.historical.historicalIntact(ctx, b.historical.historicalSources, b.historical.historicalPair) != nil {
			return AICommandPersistenceSummary{}, ErrAIExternalChanged
		}
		probe := &AIReverseSnapshot{pool: tx.Statement.ConnPool, started: time.Now(), limits: b.guard.limits, report: AIReverseSummary{DatabaseIdentitySHA256: b.guard.binding.DatabaseIdentityHash}}
		if _, err := probe.historicalTransaction(ctx, "READ WRITE"); err != nil {
			return AICommandPersistenceSummary{}, err
		}
	}
	b.attempted = true
	r, err := NewAILocalResolver(ctx, tx, b.guard.binding)
	if err != nil {
		return AICommandPersistenceSummary{}, err
	}
	actual, err := hostmysql.RequireTx(ctx)
	if err != nil || actual == nil || actual.Statement == nil || actual.Statement.ConnPool != r.pool || r.pool == b.guard.oldPool {
		return AICommandPersistenceSummary{}, ErrAILocalTransaction
	}
	if err = aiPersistencePoolEnded(ctx, b.guard.oldPool); err != nil {
		return AICommandPersistenceSummary{}, err
	}
	admission, err := r.read(ctx, aiAdmissionQuery)
	if err != nil {
		return AICommandPersistenceSummary{}, err
	}
	closed, revision, _, err := aiAdmissionSnapshot(admission)
	if err != nil || closed != "1" || revision != b.guard.binding.AdmissionRevision {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	before, err := b.guard.lockCurrentLedgers(ctx, r)
	if err != nil {
		return AICommandPersistenceSummary{}, err
	}
	if b.historical != nil && b.expected == 0 {
		if err = b.guard.verifyCurrentMetadata(ctx, r); err != nil {
			return AICommandPersistenceSummary{}, err
		}
	} else if err = b.guard.recordCurrent(ctx, tx, r, before); err != nil {
		return AICommandPersistenceSummary{}, err
	}
	after, hashes, err := aiPersistenceOperationLedger(ctx, b.guard, r)
	if err != nil {
		return AICommandPersistenceSummary{}, err
	}
	b.afterLedgers = append([]AIReverseLedgerSummary(nil), b.guard.ledgers...)
	for i, ledger := range b.afterLedgers {
		if ledger.Store == after.Store {
			b.afterLedgers[i] = after
		}
	}
	b.written = hashes
	b.writePool = r.pool
	b.afterSeal = aiJSONHash(struct {
		Ledgers []AIReverseLedgerSummary
		Written map[string]string
	}{b.afterLedgers, b.written})
	return AICommandPersistenceSummary{OriginalCommands: uint64(len(b.guard.evidence)), OriginalSourceReferences: b.expected, InTransactionWritten: len(b.guard.evidence) > 0}, nil
}

func aiPersistencePoolEnded(ctx context.Context, pool gorm.ConnPool) error {
	if pool == nil {
		return ErrAILocalTransaction
	}
	q, cancel := context.WithTimeout(ctx, 5*time.Second)
	rows, err := pool.QueryContext(q, "SELECT 1")
	if rows != nil {
		if ce := rows.Close(); ce != nil {
			err = ErrAILocalRead
		}
	}
	cancel()
	if !errors.Is(err, sql.ErrTxDone) {
		return ErrAILocalTransaction
	}
	return nil
}

// The third fresh epoch compares all fourteen complete ledgers, allowing only
// the exact dedicated operation metadata/new tombstones written by this batch.
func (b *AICommandPersistenceBatch) VerifyReadback(ctx context.Context, current *AIReverseSnapshot) (AICommandPersistenceSummary, error) {
	if b == nil || b.self != b {
		return AICommandPersistenceSummary{}, ErrAILocalBinding
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.intact() || !b.attempted || b.afterSeal == "" || b.afterSeal != aiJSONHash(struct {
		Ledgers []AIReverseLedgerSummary
		Written map[string]string
	}{b.afterLedgers, b.written}) || current == nil || current.self != current || current.pool == b.guard.oldPool || current.pool == b.writePool || current.ValidateBorrowedSnapshot(ctx) != nil || !current.report.ActualReadOnlyRR || !current.report.WholeLedgerEOF || current.report.DatabaseIdentitySHA256 != b.guard.binding.DatabaseIdentityHash || current.head != b.guard.binding.MigrationVersion || time.Now().After(b.guard.expires) {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	if current.anchorMetadataSHA != b.guard.anchorMetadataSHA || len(current.anchors) != len(b.guard.anchors) {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	for _, a := range b.guard.anchors {
		actual, ok := current.anchors[a.ID]
		if !ok || actual.rawSHA != a.RowSHA256 || actual.org != a.Org || actual.testee != a.Testee || actual.sheet != a.Sheet {
			return AICommandPersistenceSummary{}, ErrAILocalChanged
		}
	}
	if aiPersistencePoolEnded(ctx, b.guard.oldPool) != nil || aiPersistencePoolEnded(ctx, b.writePool) != nil {
		return AICommandPersistenceSummary{}, ErrAILocalTransaction
	}
	if len(current.report.Ledgers) != len(b.afterLedgers) {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	for i, want := range b.afterLedgers {
		got := current.report.Ledgers[i]
		got.Pages = want.Pages
		if got != want {
			return AICommandPersistenceSummary{}, ErrAILocalChanged
		}
	}
	for id, sha := range b.written {
		node := current.byTable["ai_messaging_operations"][id]
		if node == nil || node.observation.RowSHA256 != sha || node.observation.Invalid || node.observation.Held || len(node.observation.Reasons) != 0 {
			return AICommandPersistenceSummary{}, ErrAILocalChanged
		}
	}
	if current.ValidateBorrowedSnapshot(ctx) != nil {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	return AICommandPersistenceSummary{OriginalCommands: uint64(len(b.guard.evidence)), OriginalSourceReferences: b.expected, InTransactionWritten: true, IndependentReadbackMatched: true}, nil
}

func aiCommandRetirementOperationExpected(columns []string, row aiReverseRow, e store.CommandRetirementEvidence) error {
	if !reflect.DeepEqual(columns, aiReverseSpecByTable("ai_messaging_operations").columns) || row == nil || len(row) != len(columns) || row.text("command_id") != e.CommandID || row.text("retired") != "1" || row.text("organization_id") != e.OrganizationID || row.text("subject_id") != e.SubjectID || row.text("resource_id") != e.ResourceID || row.text("aggregate_key") != e.RequestID || !aiReverseClock(row["retired_at"], false) {
		return ErrAILocalChanged
	}
	for _, col := range columns {
		if _, present := row[col]; !present {
			return ErrAILocalChanged
		}
	}
	kind := "2"
	if e.SourceKind == "start" {
		kind = "1"
	}
	if row.text("kind") != kind {
		return ErrAILocalChanged
	}
	for _, name := range []string{"body_sha256", "aggregate_sequence", "receipt_id", "receipt", "created_at", "decided_at"} {
		if value, ok := row[name]; !ok || value != nil {
			return ErrAILocalChanged
		}
	}
	for _, name := range []string{"decision", "code"} {
		if row.text(name) != "" {
			return ErrAILocalChanged
		}
	}
	d := json.NewDecoder(bytes.NewReader(row["retirement_evidence"]))
	d.DisallowUnknownFields()
	var actual store.CommandRetirementEvidence
	if d.Decode(&actual) != nil || d.Decode(new(json.RawMessage)) != io.EOF || !reflect.DeepEqual(actual, e) || actual.Conclusion == "transferred_verified" || !actual.ResponsibilityClosed || !actual.BusinessTerminal {
		return ErrAILocalChanged
	}
	return nil
}

func aiPersistenceCloneGuard(p *AICommandHandoffBatch) *AICommandHandoffBatch {
	if p == nil {
		return nil
	}
	return &AICommandHandoffBatch{binding: p.binding, oldPool: p.oldPool, started: p.started, expires: p.expires, limits: p.limits, copies: p.copies, ledgers: append([]AIReverseLedgerSummary(nil), p.ledgers...), metadata: append([]aiReverseMetadata(nil), p.metadata...), anchors: append([]aiCommandHandoffAnchor(nil), p.anchors...), anchorMetadataSHA: p.anchorMetadataSHA}
}

// Reads only the operation ledger changed by this batch. The preceding full14
// locking read remains once per batch; all other ledgers must stay byte-exact.
func aiPersistenceOperationLedger(ctx context.Context, p *AICommandHandoffBatch, r *AILocalResolver) (AIReverseLedgerSummary, map[string]string, error) {
	spec := aiReverseSpecByTable("ai_messaging_operations")
	probe := &AIReverseSnapshot{pool: r.pool, started: p.started, limits: p.limits}
	meta, err := probe.schema(ctx, spec)
	if err != nil {
		return AIReverseLedgerSummary{}, nil, ErrAILocalChanged
	}
	var original AIReverseLedgerSummary
	for i, specification := range aiReverseSpecs {
		if specification.table == spec.table {
			if !aiCommandHandoffMetadataEqual([]aiReverseMetadata{meta}, p.metadata[i:i+1]) {
				return AIReverseLedgerSummary{}, nil, ErrAILocalChanged
			}
			original = p.ledgers[i]
		}
	}
	if original.Store == "" {
		return AIReverseLedgerSummary{}, nil, ErrAILocalBinding
	}
	ledger := AIReverseLedgerSummary{Store: spec.table, SchemaSHA256: meta.schema, PrimaryKeySHA256: meta.pk}
	hash := sha256.New()
	sourceFrame(hash, []byte("ai-reverse-ledger/v1"), false)
	sourceFrame(hash, []byte(meta.schema), false)
	wanted := map[string]bool{}
	var additions uint64
	for _, e := range p.evidence {
		wanted[e.CommandID] = true
		if e.Conclusion != "transferred_verified" {
			additions++
		}
	}
	matched := map[string]string{}
	var after []string
	projection, order := aiCommandHandoffProjection(spec)
	for {
		if timeExpiredHandoff(ctx, p) {
			return ledger, nil, ErrAILocalBounds
		}
		query := "SELECT " + projection + " FROM `ai_messaging_operations` FORCE INDEX(PRIMARY)"
		var args []any
		if after != nil {
			predicate, values := aiReversePredicate(spec, after, false)
			query += " WHERE " + predicate
			args = append(args, values...)
		}
		query += " ORDER BY " + order + " LIMIT ? FOR UPDATE"
		args = append(args, p.limits.PageRows)
		rows, names, size, err := probe.read(ctx, query, p.limits.PageRows, args...)
		if err != nil || !reflect.DeepEqual(names, meta.columns) {
			return ledger, nil, ErrAILocalRead
		}
		if uint64(len(rows)) > p.limits.MaxRows-ledger.Rows || size > p.limits.MaxBytes-ledger.Bytes {
			return ledger, nil, ErrAILocalBounds
		}
		if len(rows) == 0 {
			break
		}
		ledger.Rows += uint64(len(rows))
		ledger.Bytes += size
		ledger.Pages++
		for _, row := range rows {
			key, err := aiReverseKey(spec, row)
			if err != nil || after != nil && aiReverseCompare(spec, after, key) >= 0 {
				return ledger, nil, ErrAILocalChanged
			}
			after = key
			sha := aiReverseRowSHA(meta.columns, row)
			sourceFrame(hash, []byte(sha), false)
			if wanted[row.text("command_id")] {
				if matched[row.text("command_id")] != "" {
					return ledger, nil, ErrAILocalChanged
				}
				matched[row.text("command_id")] = sha
			}
		}
	}
	ledger.UpperSHA256 = aiReverseKeySHA(after)
	ledger.RowsSHA256 = hex.EncodeToString(hash.Sum(nil))
	if ledger.Rows != original.Rows+additions || len(matched) != len(wanted) || p.verifyCurrentMetadata(ctx, r) != nil {
		return ledger, nil, ErrAILocalChanged
	}
	return ledger, matched, nil
}

// This preserves the original observed deadline; it is not a Window or a
// production permit. It also supports real all-AI batches with no event spool.
func (b *AICommandPersistenceBatch) InheritedContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	if parent == nil || parent.Err() != nil || b == nil || b.self != b {
		return nil, nil, ErrAILocalBinding
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.intact() || time.Now().After(b.guard.expires) {
		return nil, nil, ErrAILocalBounds
	}
	ctx, cancel := context.WithDeadline(parent, b.guard.expires)
	return ctx, cancel, nil
}

// Read original commands only from the protected complete AI input pages. Their
// real original full source authentication preceded capture; no DTO can enter.
func historicalAIOriginalCommands(ctx context.Context, sources *HistoricalSourceInputPair, pair *AIHistoricalInputPair) ([2]map[string]*DecodedAICommand, error) {
	out := [2]map[string]*DecodedAICommand{{}, {}}
	if aiComponentPairIntact(ctx, pair, sources) != nil {
		return out, ErrSourceAuthentication
	}
	for page := range pair.second.pages {
		frame, err := pair.second.componentFrame(ctx, page)
		if err != nil {
			return out, err
		}
		i := -1
		if frame.Ledger.Store == AIBridgeCommandSource {
			i = 0
		}
		if frame.Ledger.Store == AILegacyCommandSource {
			i = 1
		}
		if i < 0 || frame.EOF {
			continue
		}
		rows, spec, meta, err := pair.second.componentPage(ctx, page)
		if err != nil {
			return out, err
		}
		for _, row := range rows {
			cells := make([][]byte, len(meta.columns))
			for j, col := range meta.columns {
				cells[j] = row[col]
			}
			v, err := aiCurrentSource(cells, spec.table, meta.sourceColumns, sources.second.boundaries[i+1])
			if err != nil || out[i][v.CommandID] != nil {
				return out, ErrSourceAuthentication
			}
			out[i][v.CommandID] = v
			if len(out[0])+len(out[1]) > 10000 {
				return out, ErrAIExternalInput
			}
		}
	}
	for i := range out {
		if uint64(len(out[i])) != sources.second.receipts[i+1].Records {
			return out, ErrSourceIncomplete
		}
	}
	return out, nil
}
func historicalAIKnownHandoffs(reverse *AIReverseSnapshot, commands [2]map[string]*DecodedAICommand) ([]aiExternalKnownHandoff, error) {
	out := make([]aiExternalKnownHandoff, 0, len(commands[1]))
	for id, legacy := range commands[1] {
		bridge := commands[0][id]
		if bridge == nil || ValidateAISourcePair(bridge, legacy) != nil || aiCommandHandoffNodes(reverse, bridge, legacy) != nil {
			return nil, ErrAILocalResponsibility
		}
		request, op, box := reverse.byTable["ai_bridge_requests"][bridge.RequestID], reverse.byTable["ai_messaging_operations"][id], reverse.byTable["ai_messaging_outbox"][id]
		if request == nil || op == nil || box == nil {
			return nil, ErrAIExternalChanged
		}
		out = append(out, aiExternalKnownHandoff{CommandID: id, RequestID: bridge.RequestID, SourceKind: bridge.SourceKind, OrganizationID: bridge.OrganizationID, SubjectID: bridge.SubjectID, ResourceID: bridge.ResourceID, TesteeID: request.testee, BridgePayloadSHA: bridge.PayloadBytesDigest.SHA256, LegacyPayloadSHA: legacy.PayloadBytesDigest.SHA256, WriterSHA: bridge.WriterPayloadDigest.SHA256, MessagingSHA: legacy.Transport.MessagingBodySHA256, SourceAttempts: uint64(bridge.Transport.SourceAttempts), Sequence: op.sequence, OperationRowSHA: op.observation.RowSHA256, OutboxRowSHA: box.observation.RowSHA256})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CommandID < out[j].CommandID })
	return out, nil
}

// All original command IDs are one genuine bounded batch. The original full14
// baseline and admission are locked by Record; this factory never opens a RW
// connection, commits, sends, generates a new command ID or grants DROP.
func PrepareHistoricalAICommandPersistenceBatch(ctx context.Context, sources *HistoricalSourceInputPair, pair *AIHistoricalInputPair, q *AIExternalExecutionQualification) (*AICommandPersistenceBatch, error) {
	if q.historicalIntact(ctx, sources, pair) != nil || q.reverse.validateHistoricalSnapshot(ctx) != nil {
		return nil, ErrAIExternalChanged
	}
	reverse := q.reverse
	admission := reverse.byTable["ai_messaging_admission"]["1"]
	if admission == nil || admission.state != "closed" || reverse.report.Unknown != 0 || len(reverse.structuralReasons) != 0 {
		return nil, ErrAILocalResponsibility
	}
	binding := AIResolverBinding{DatabaseIdentityHash: reverse.report.DatabaseIdentitySHA256, MigrationVersion: 99, SourceSHA: q.historicalBinding.SourceSHA, OperationID: q.historicalBinding.OperationID, AdmissionRevision: admission.sequence, BridgeBoundary: sources.second.boundaries[1], LegacyBoundary: sources.second.boundaries[2]}
	guard := &AICommandHandoffBatch{binding: binding, oldPool: reverse.pool, started: reverse.started, expires: q.expires, limits: reverse.limits, copies: sources.second.receipts, ledgers: append([]AIReverseLedgerSummary(nil), reverse.report.Ledgers...), anchorMetadataSHA: reverse.anchorMetadataSHA}
	for i, meta := range reverse.metadata {
		guard.metadata = append(guard.metadata, aiReverseMetadata{columns: append([]string(nil), meta.columns...), sourceColumns: aiCloneColumns(meta.sourceColumns), schema: meta.schema, pk: meta.pk})
		switch aiReverseSpecs[i].table {
		case AIBridgeCommandSource:
			guard.binding.BridgeColumns = aiCloneColumns(meta.sourceColumns)
		case AILegacyCommandSource:
			guard.binding.LegacyColumns = aiCloneColumns(meta.sourceColumns)
		}
	}
	for _, a := range reverse.anchors {
		guard.anchors = append(guard.anchors, aiCommandHandoffAnchor{a.id, a.org, a.testee, a.sheet, a.rawSHA})
	}
	sort.Slice(guard.anchors, func(i, j int) bool {
		a, _ := strconv.ParseUint(guard.anchors[i].ID, 10, 64)
		b, _ := strconv.ParseUint(guard.anchors[j].ID, 10, 64)
		return a < b
	})
	commands, err := historicalAIOriginalCommands(ctx, sources, pair)
	if err != nil {
		return nil, err
	}
	for _, id := range aiReverseSortedKeys(commands[0]) {
		v := commands[0][id]
		if legacy := commands[1][id]; legacy != nil {
			if aiCommandHandoffNodes(reverse, v, legacy) != nil || q.handoffs[id].CommandID != id {
				return nil, ErrAILocalResponsibility
			}
			guard.evidence = append(guard.evidence, aiCommandHandoffEvidence(guard.binding, v, legacy, time.Now().UTC()))
			continue
		}
		original, ok := q.byID[id]
		if !ok || !aiExternalOriginalValid(original) || reverse.byTable["ai_messaging_operations"][id] != nil || reverse.byTable["ai_messaging_outbox"][id] != nil {
			return nil, ErrAILocalResponsibility
		}
		for _, node := range reverse.nodes {
			if node.request != v.RequestID && node.aggregate != v.RequestID && node.id != id && node.command != id {
				continue
			}
			if node.observation.Invalid || node.observation.Held || len(node.observation.Reasons) != 0 || node.observation.Unfinished && node.observation.Store != AIBridgeCommandSource {
				return nil, ErrAILocalResponsibility
			}
		}
		conclusion, reason := "verified", "history_terminal_verified"
		if original.Result == "historical_original_configuration_not_retained" {
			conclusion, reason = "unverifiable", "history_terminal_evidence_gap"
		}
		guard.evidence = append(guard.evidence, store.CommandRetirementEvidence{Version: 1, OperationID: guard.binding.OperationID, VerifierVersion: "qs-ai-actual-execution/v1", VerificationMethod: "source_identity_hash_and_business_closure", VerifiedAt: time.Now().UTC(), AdmissionRevision: guard.binding.AdmissionRevision, CommandID: v.CommandID, RequestID: v.RequestID, SourceKind: v.SourceKind, OrganizationID: v.OrganizationID, SubjectID: v.SubjectID, ResourceID: v.ResourceID, Sources: []store.CommandRetirementSource{{Table: AIBridgeCommandSource, CommandID: v.CommandID, BytesKind: v.PayloadBytesDigest.Kind, BytesSHA256: v.PayloadBytesDigest.SHA256, BusinessPayloadHash: v.WriterPayloadDigest.SHA256}}, References: []store.CommandRetirementReference{{Kind: "business_record", ID: v.RequestID}, {Kind: "business_record", ID: "qs-ai/session/" + original.SessionID}, {Kind: "business_record", ID: "qs-ai/run/" + original.ActiveRunID}, {Kind: "event", ID: original.TerminalEventID}, {Kind: "migration_manifest", ID: q.seal}, {Kind: "readonly_run", ID: guard.binding.OperationID}}, Conclusion: conclusion, Reason: reason, OwnershipVerified: true, ResponsibilityClosed: true, BusinessTerminal: true})
	}
	guard.self, guard.seal = guard, guard.digest()
	b := &AICommandPersistenceBatch{guard: guard, historical: q, expected: sources.second.receipts[1].Records + sources.second.receipts[2].Records, written: map[string]string{}}
	b.self, b.seal = b, b.digest()
	if !b.intact() || q.reverse.validateHistoricalSnapshot(ctx) != nil {
		return nil, ErrAILocalChanged
	}
	return b, nil
}

// The host must have ended the old and successful write transactions first.
// Actual current full14 rows, not statement expected bytes, establish continuity.
func (b *AICommandPersistenceBatch) VerifyHistoricalReadback(ctx context.Context, sources *HistoricalSourceInputPair, pair *AIHistoricalInputPair) (AICommandPersistenceSummary, error) {
	if b == nil || b.self != b {
		return AICommandPersistenceSummary{}, ErrAILocalBinding
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.intact() || b.historical.historicalIntact(ctx, sources, pair) != nil || !b.attempted || b.afterSeal == "" || b.afterSeal != aiJSONHash(struct {
		Ledgers []AIReverseLedgerSummary
		Written map[string]string
	}{b.afterLedgers, b.written}) || aiPersistencePoolEnded(ctx, b.guard.oldPool) != nil || aiPersistencePoolEnded(ctx, b.writePool) != nil {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	current, err := prepareHistoricalAIFullSnapshot(ctx, sources, pair, b.guard.limits, "READ ONLY")
	if err != nil {
		return AICommandPersistenceSummary{}, err
	}
	if current.pool == b.guard.oldPool || current.pool == b.writePool || !historicalAILedgersEqual(current.report.Ledgers, b.afterLedgers) || current.anchorMetadataSHA != b.guard.anchorMetadataSHA || len(current.anchors) != len(b.guard.anchors) {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	for _, a := range b.guard.anchors {
		got := current.anchors[a.ID]
		if got.id != a.ID || got.org != a.Org || got.testee != a.Testee || got.sheet != a.Sheet || got.rawSHA != a.RowSHA256 {
			return AICommandPersistenceSummary{}, ErrAILocalChanged
		}
	}
	for id, sha := range b.written {
		node := current.byTable["ai_messaging_operations"][id]
		if node == nil || node.observation.RowSHA256 != sha || node.observation.Invalid || node.observation.Held || len(node.observation.Reasons) != 0 {
			return AICommandPersistenceSummary{}, ErrAILocalChanged
		}
	}
	if current.validateHistoricalSnapshot(ctx) != nil {
		return AICommandPersistenceSummary{}, ErrAILocalChanged
	}
	b.readbackSnapshot = current
	b.readbackPool = current.pool
	b.readbackSeal = aiReverseHash("historical-ai-committed-server-read/v1", b.afterSeal, current.historicalSeal)
	return AICommandPersistenceSummary{OriginalCommands: uint64(len(b.guard.evidence)), OriginalSourceReferences: b.expected, InTransactionWritten: len(b.guard.evidence) > 0, IndependentReadbackMatched: true}, nil
}

// Aggregate-only validation consumes real native facts and committed AI
// continuity. It neither grants non-AI source closure nor a writer-fence permit.
func ValidateHistoricalComponentAI(ctx context.Context, o *HistoricalComponentObservation, q *AIExternalExecutionQualification, b *AICommandPersistenceBatch) error {
	if o == nil || o.ValidateBorrowedObservation(ctx) != nil || q.historicalIntact(ctx, o.source.pair, o.ai.componentPair) != nil || b == nil || b.self != b {
		return ErrAIReverseBinding
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.intact() || b.historical != q || b.readbackSeal == "" || b.readbackSnapshot == nil || b.readbackSnapshot.self != b.readbackSnapshot || b.readbackSnapshot.historicalSeal == "" || b.readbackSnapshot.historicalSeal != b.readbackSnapshot.historicalDigest() || b.readbackSeal != aiReverseHash("historical-ai-committed-server-read/v1", b.afterSeal, b.readbackSnapshot.historicalSeal) || b.writePool == nil || b.readbackPool == nil || aiPersistencePoolEnded(ctx, b.writePool) != nil || aiPersistencePoolEnded(ctx, b.readbackPool) != nil {
		return ErrAIReverseFresh
	}
	// This actual native component read has already expanded its necessary
	// request/command/resource/owner predicates to a fixed point. Compare that
	// complete selected closure with the independently read committed image;
	// unrelated ledgers may change and are not rescanned here. Global unbound
	// and new-organization negatives remain separate deletion gates.
	current := o.ai
	if current.ValidateComponentObservation(ctx) != nil || !historicalAIScopedRowsEqual(current, b.readbackSnapshot) {
		return ErrAIReverseChanged
	}
	admission := current.byTable["ai_messaging_admission"]["1"]
	if admission == nil || admission.state != "closed" || admission.sequence != b.guard.binding.AdmissionRevision {
		return ErrAILocalChanged
	}
	for _, node := range current.nodes {
		if node.observation.Store == "ai_messaging_quarantine" || node.observation.Store == "ai_messaging_observations" {
			continue
		}
		if node.observation.Invalid || node.observation.Held || len(node.observation.Reasons) != 0 {
			return ErrAILocalResponsibility
		}
		if node.observation.Unfinished {
			if node.observation.Store == AIBridgeCommandSource && q.byID[node.id].CommandID == node.id {
				op := current.byTable["ai_messaging_operations"][node.id]
				if op != nil && op.retired && b.written[node.id] == op.observation.RowSHA256 {
					continue
				}
			}
			if historicalAIKnownCurrentPending(node, q) {
				continue
			}
			return ErrAILocalResponsibility
		}
	}
	return current.ValidateComponentObservation(ctx)
}

// Only private actual observations call this data comparison. It neither
// constructs a snapshot nor proves the unbound/global negative ranges.
func historicalAIScopedRowsEqual(current, committed *AIReverseSnapshot) bool {
	if current == nil || committed == nil || current.scope == nil || len(current.metadata) != len(aiReverseSpecs) || !reflect.DeepEqual(current.metadata, committed.metadata) {
		return false
	}
	for _, spec := range aiReverseSpecs {
		if spec.table == "ai_messaging_quarantine" || spec.table == "ai_messaging_observations" {
			continue
		}
		for id, got := range current.byTable[spec.table] {
			want := committed.byTable[spec.table][id]
			if want == nil || got.observation.PrimaryKeySHA256 != want.observation.PrimaryKeySHA256 || got.observation.RowSHA256 != want.observation.RowSHA256 {
				return false
			}
		}
		for id, want := range committed.byTable[spec.table] {
			if historicalAIComponentSelects(current, spec.table, want) && current.byTable[spec.table][id] == nil {
				return false
			}
		}
	}
	return true
}

// This mirrors aiComponentPredicate against the original decoded native
// column values, so a disappeared matching row cannot be hidden by a JOIN.
func historicalAIComponentSelects(s *AIReverseSnapshot, table string, n *aiReverseNode) bool {
	if s == nil || s.scope == nil || n == nil {
		return false
	}
	r, ids, a, resources := s.scope.relatedRequests, s.scope.relatedIDs, s.componentAssessments, s.componentResources
	switch table {
	case "ai_bridge_requests":
		return r[n.id] || resources[n.resource]
	case "ai_bridge_request_assessments":
		return r[n.request] || a[n.resource]
	case AIBridgeCommandSource, AILegacyCommandSource, "ai_bridge_events":
		return r[n.request] || ids[n.id]
	case "ai_messaging_operations":
		return r[n.aggregate] || ids[n.id] || resources[n.resource] || ids[n.receipt]
	case "ai_messaging_outbox", "ai_messaging_inbox", "ai_messaging_failures":
		return r[n.aggregate] || ids[n.id]
	case "ai_messaging_aggregates":
		return r[n.aggregate]
	case "ai_messaging_evaluation_states":
		return resources[n.id] || r[n.id]
	case "ai_messaging_admission":
		return true
	}
	return false
}

func (b *AICommandPersistenceBatch) historicalEmptyGuardIntact() bool {
	if b == nil || b.historical == nil || b.expected != 0 || b.guard == nil || b.guard.self != b.guard || len(b.guard.evidence) != 0 || b.guard.oldPool == nil || !aiValidBinding(b.guard.binding) || len(b.guard.metadata) != len(aiReverseSpecs) || len(b.guard.ledgers) != len(aiReverseSpecs) || !b.guard.limits.valid() || !evidenceHash(b.guard.anchorMetadataSHA) || b.guard.expires.IsZero() || b.guard.seal != b.guard.digest() {
		return false
	}
	q := b.historical
	return q.self == q && q.seal == aiJSONHash(q.facts) && q.historicalSources != nil && q.historicalSources.second.receipts[1].Records == 0 && q.historicalSources.second.receipts[2].Records == 0 && len(q.byID) == 0 && len(q.handoffs) == 0
}

func historicalAIKnownCurrentPending(node *aiReverseNode, q *AIExternalExecutionQualification) bool {
	if node == nil || q == nil || node.observation.Invalid || node.observation.Held || len(node.observation.Reasons) != 0 {
		return false
	}
	switch node.observation.Store {
	case "ai_bridge_requests":
		for _, h := range q.handoffs {
			if h.RequestID == node.id {
				return true
			}
		}
	case "ai_messaging_operations":
		return q.handoffs[node.id].CommandID == node.id && !node.retired && node.state == ""
	case "ai_messaging_outbox":
		id := node.command
		if id == "" {
			id = node.id
		}
		return q.handoffs[id].CommandID == id && (node.state == "staged" || node.state == "awaiting_receipt")
	}
	return false
}
