package retirement

import (
	"context"
	"io"
	"strings"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
)

const aiAdmissionQuery = "SELECT CAST(closed AS BINARY),CAST(revision AS BINARY),CAST(updated_at AS BINARY) FROM ai_messaging_admission WHERE singleton=1 LOCK IN SHARE MODE"

type aiReadOnlyMode struct{ closed string }

// AIReadOnlyResolver carries only one authenticated host-owned RR-RO epoch.
// Its locking resolver is private and cannot be used to obtain a RW capability.
// It neither owns the transaction nor proves full qs-ai or reverse MQ closure.
type AIReadOnlyResolver struct {
	owner        *HistoricalCoordinator
	snapshot     *SQLResponsibilitySnapshot
	inner        *AILocalResolver
	admissionSHA string
}

type AIReadOnlyQualification struct{ summary AIReadOnlySummary }

type AIReadOnlySummary struct {
	AILocalSummary
	LocalReadOnly bool `json:"local_readonly"`
	CASAuthority  bool `json:"cas_authority"`
}

// Stable facts only: transaction identity is checked privately, while cycle
// nonces and observation timestamps are deliberately not compared as data.
type AIReadOnlyBindingSummary struct {
	Scope                 string
	DatabaseIdentityHash  string
	MigrationVersion      uint64
	SourceSHA             string
	OperationID           string
	AdmissionClosed       bool
	AdmissionRevision     uint64
	AdmissionRowSHA256    string
	AuthenticatedCopies   [2]SourceCopyReceipt
	ActualReadOnlyRR      bool
	ExternalClosure       string
	GlobalReverseCoverage string
	CASAuthority          bool
	DropReady             bool
}

func (r *AIReadOnlyResolver) Summary() AIReadOnlyBindingSummary {
	if r == nil || r.inner == nil || r.inner.readOnly == nil || r.owner == nil || r.owner.authenticated == nil {
		return AIReadOnlyBindingSummary{}
	}
	b := r.inner.binding
	return AIReadOnlyBindingSummary{Scope: "qs-local-readonly-epoch-binding-only", DatabaseIdentityHash: b.DatabaseIdentityHash, MigrationVersion: b.MigrationVersion, SourceSHA: b.SourceSHA, OperationID: b.OperationID, AdmissionClosed: r.inner.readOnly.closed == "1", AdmissionRevision: b.AdmissionRevision, AdmissionRowSHA256: r.admissionSHA, AuthenticatedCopies: [2]SourceCopyReceipt{r.owner.authenticated.receipts[1], r.owner.authenticated.receipts[2]}, ActualReadOnlyRR: true, ExternalClosure: "unknown", GlobalReverseCoverage: "unknown"}
}

func (*AIReadOnlyResolver) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReadOnlyResolver) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReadOnlyResolver) String() string {
	return "private local readonly AI resolver; no execution authority"
}
func (r *AIReadOnlyResolver) GoString() string                { return r.String() }
func (*AIReadOnlyQualification) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReadOnlyQualification) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReadOnlyQualification) String() string {
	return "private local readonly AI qualification; external closure unproven"
}
func (q *AIReadOnlyQualification) GoString() string { return q.String() }

func (q *AIReadOnlyQualification) Summary() AIReadOnlySummary {
	if q == nil {
		return AIReadOnlySummary{}
	}
	s := q.summary
	s.Gaps = append([]string(nil), s.Gaps...)
	return s
}

// The only rewritten statements are the exact compile-time reads used by the
// existing point graph. No caller-controlled SQL or identifiers are admitted.
func aiReadOnlyStatement(q string) (string, error) {
	switch q {
	case "SELECT @@server_uuid,DATABASE(),VERSION()",
		"SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION",
		"SHOW CREATE TABLE `ai_bridge_commands`", "SHOW CREATE TABLE `ai_messaging_legacy_commands`":
		return q, nil
	case "SELECT CAST(version AS BINARY),CAST(dirty AS BINARY) FROM schema_migrations FOR UPDATE",
		aiRequestQuery, aiBridgeQuery, aiLegacyQuery, aiOperationQuery, aiOutboxQuery,
		"SELECT CAST(event_id AS BINARY),CAST(request_id AS BINARY),CAST(version AS BINARY),CAST(payload_hash AS BINARY) FROM ai_bridge_events WHERE event_id=? OR request_id=? ORDER BY event_id LIMIT 1025 FOR UPDATE",
		"SELECT CAST(request_id AS BINARY),CAST(assessment_id AS BINARY) FROM ai_bridge_request_assessments WHERE request_id=? ORDER BY assessment_id LIMIT 1025 FOR UPDATE",
		"SELECT CAST(aggregate_key AS BINARY),CAST(next_sequence AS BINARY) FROM ai_messaging_aggregates WHERE aggregate_key=? FOR UPDATE",
		"SELECT CAST(wire_sha256 AS BINARY),CAST(code AS BINARY),CAST(attempts AS BINARY) FROM ai_messaging_quarantine ORDER BY wire_sha256 LIMIT 1 FOR UPDATE":
		return strings.TrimSuffix(q, " FOR UPDATE"), nil
	case aiAdmissionQuery:
		return strings.TrimSuffix(q, " LOCK IN SHARE MODE"), nil
	}
	const suffix = ") ORDER BY producer,message_id LIMIT 1025 FOR UPDATE"
	for _, base := range []string{aiInboxBase, aiFailureBase} {
		if !strings.HasPrefix(q, base) || !strings.HasSuffix(q, suffix) {
			continue
		}
		list := strings.TrimSuffix(strings.TrimPrefix(q, base), suffix)
		count := strings.Count(list, "?")
		if count < 1 || count > 2*aiLocalMaxRows+3 || list != strings.TrimSuffix(strings.Repeat("?,", count), ",") {
			return "", ErrAILocalRead
		}
		return strings.TrimSuffix(q, " FOR UPDATE"), nil
	}
	return "", ErrAILocalRead
}

func aiAdmissionSnapshot(rows [][][]byte) (string, uint64, string, error) {
	if len(rows) != 1 || len(rows[0]) != 3 || (aiText(rows[0], 0) != "0" && aiText(rows[0], 0) != "1") || len(rows[0][2]) == 0 {
		return "", 0, "", ErrAILocalBinding
	}
	revision, valid := aiUint(rows[0][1])
	if !valid {
		return "", 0, "", ErrAILocalBinding
	}
	s := &aiLocalSnapshot{}
	if err := s.add("admission", rows); err != nil {
		return "", 0, "", err
	}
	return aiText(rows[0], 0), revision, s.hash(), nil
}

// PrepareAIReadOnlyResolver re-reads the two exact authenticated source copies
// to EOF without seeking, closing, or taking ownership of them. The host must
// rewind its own assets before and after this call. Editable bindings cannot
// substitute for either the complete source authentication or SQL snapshot.
func (c *HistoricalCoordinator) PrepareAIReadOnlyResolver(ctx context.Context, snapshot *SQLResponsibilitySnapshot, expectedMigration uint64, copies []SourceCopyInput) (*AIReadOnlyResolver, error) {
	if c == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if !c.authenticated.complete || snapshot == nil || expectedMigration == 0 || len(copies) != 2 {
		return nil, ErrAILocalBinding
	}
	if snapshot.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAILocalTransaction
	}
	report := snapshot.Report()
	if !report.ActualTransactionReadOnlyRR || report.CompletedAt.IsZero() || len(report.Ledgers) != 8 || !evidenceHash(report.DatabaseIdentitySHA256) {
		return nil, ErrAILocalTransaction
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return nil, ErrAILocalTransaction
	}
	if _, err = sdkmysql.BindGORM(tx); err != nil {
		return nil, ErrAILocalTransaction
	}
	binding := AIResolverBinding{DatabaseIdentityHash: report.DatabaseIdentitySHA256, MigrationVersion: expectedMigration, SourceSHA: c.binding.SourceSHA, OperationID: c.binding.OperationID}
	for i, copy := range copies {
		if sourceReaderAbsent(copy.Input) || copy.Expected != c.copies[i+1].Expected {
			return nil, ErrSourceAuthentication
		}
		reader, err := NewAISQLSourceReader(copy.Input, copy.Expected)
		if err != nil {
			return nil, err
		}
		for {
			if err := c.alive(ctx); err != nil {
				return nil, err
			}
			facts, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if _, err = c.authenticated.BindAICommand(facts); err != nil {
				return nil, err
			}
		}
		if receipt := reader.Receipt(); !receipt.Complete || receipt != c.authenticated.receipts[i+1] {
			return nil, ErrSourceIncomplete
		}
		if i == 0 {
			binding.BridgeBoundary, binding.BridgeColumns = copy.Expected.Boundary, aiCloneColumns(reader.columns)
		} else {
			binding.LegacyBoundary, binding.LegacyColumns = copy.Expected.Boundary, aiCloneColumns(reader.columns)
		}
	}
	if !aiValidBinding(binding) {
		return nil, ErrAILocalBinding
	}
	inner := &AILocalResolver{pool: tx.Statement.ConnPool, binding: binding, readOnly: &aiReadOnlyMode{}}
	if err := inner.verifyBinding(ctx); err != nil {
		return nil, err
	}
	rows, err := inner.read(ctx, aiAdmissionQuery)
	if err != nil {
		return nil, err
	}
	closed, revision, digest, err := aiAdmissionSnapshot(rows)
	if err != nil {
		return nil, err
	}
	inner.binding.AdmissionRevision, inner.readOnly.closed = revision, closed
	r := &AIReadOnlyResolver{owner: c, snapshot: snapshot, inner: inner, admissionSHA: digest}
	if err := r.validate(ctx); err != nil {
		return nil, err
	}
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *AIReadOnlyResolver) validate(ctx context.Context) error {
	if r == nil || r.owner == nil || r.snapshot == nil || r.inner == nil || r.inner.readOnly == nil || !aiValidBinding(r.inner.binding) || !evidenceHash(r.admissionSHA) {
		return ErrAILocalBinding
	}
	if r.snapshot.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrAILocalTransaction
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx == nil || tx.Statement == nil || tx.Statement.ConnPool != r.inner.pool {
		return ErrAILocalTransaction
	}
	if err := r.inner.verifyBinding(ctx); err != nil {
		return err
	}
	rows, err := r.inner.read(ctx, aiAdmissionQuery)
	if err != nil {
		return err
	}
	closed, revision, digest, err := aiAdmissionSnapshot(rows)
	if err != nil || closed != r.inner.readOnly.closed || revision != r.inner.binding.AdmissionRevision || digest != r.admissionSHA {
		return ErrAILocalChanged
	}
	return nil
}

func (r *AIReadOnlyResolver) resolve(ctx context.Context, bridge, legacy *DecodedAICommand) (*AIReadOnlyQualification, error) {
	if err := r.validate(ctx); err != nil {
		return nil, err
	}
	if !aiDecodedIdentityValid(bridge) || bridge.Source.Object != AIBridgeCommandSource {
		return nil, ErrAILocalBinding
	}
	if legacy != nil && ValidateAISourcePair(bridge, legacy) != nil {
		return nil, ErrAILocalRelation
	}
	if _, err := r.owner.authenticated.BindAICommand(bridge); err != nil {
		return nil, err
	}
	if legacy != nil {
		if _, err := r.owner.authenticated.BindAICommand(legacy); err != nil {
			return nil, err
		}
	}
	observed, gaps, err := r.inner.observe(ctx, bridge, legacy)
	if err != nil {
		return nil, err
	}
	if err := r.validate(ctx); err != nil {
		return nil, err
	}
	return &AIReadOnlyQualification{summary: AIReadOnlySummary{AILocalSummary: AILocalSummary{Version: 1, Scope: "qs-local-readonly-point-graph-only", LocalQualified: true, DatabaseIdentityHash: r.inner.binding.DatabaseIdentityHash, SourceSHA: r.inner.binding.SourceSHA, OperationID: r.inner.binding.OperationID, BaselineSHA256: observed.hash(), ObservedNodes: observed.nodes, Gaps: append([]string(nil), gaps...), ExternalClosure: "unknown", GlobalReverseCoverage: "unknown", RetirementConclusion: "not_generated"}, LocalReadOnly: true}}, nil
}
