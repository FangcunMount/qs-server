package retirement

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"gorm.io/gorm"
)

// AICommandHandoffBatch is prepared from real source capabilities and the full
// local AI ledger. It records a transfer of the OLD PUBLISHER's responsibility,
// never provider completion, final retirement, a writer fence or DROP authority.
// No record, flag or serialized summary can construct this batch.
type AICommandHandoffBatch struct {
	mu                sync.Mutex
	self              *AICommandHandoffBatch
	binding           AIResolverBinding
	oldPool           gorm.ConnPool
	started, expires  time.Time
	limits            AIReverseLimits
	metadata          []aiReverseMetadata
	ledgers           []AIReverseLedgerSummary
	anchors           []aiCommandHandoffAnchor
	anchorMetadataSHA string
	copies            [4]SourceCopyReceipt
	evidence          []store.CommandRetirementEvidence
	seal              string
	attempted         bool
}

// AICommandHandoffResult is only an in-transaction observation. The host owns
// commit/rollback and its durable operation journal. It cannot be imported as
// a proof or used to finish the current operation/provider responsibilities.
type AICommandHandoffResult struct {
	Scope                   string `json:"scope"`
	Records                 uint64 `json:"records"`
	EvidenceSHA256          string `json:"evidence_sha256"`
	HostCommitRequired      bool   `json:"host_commit_required"`
	ProviderClosureVerified bool   `json:"provider_closure_verified"`
	RetirementAuthority     bool   `json:"retirement_authority"`
	DropReady               bool   `json:"drop_ready"`
}

func (*AICommandHandoffBatch) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AICommandHandoffBatch) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AICommandHandoffBatch) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*AICommandHandoffBatch) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (*AICommandHandoffBatch) String() string {
	return "private original-command handoff batch; no execution or DROP authority"
}
func (p *AICommandHandoffBatch) GoString() string { return p.String() }

// PrepareAICommandHandoffBatch does not consume the coordinator page. The host
// prepares it before ordinary page qualification/consumption, then ends the
// original read-only transaction before Record. It cannot repair an unmapped
// command by staging a new ID. Those rows still need actual external closure.
func (c *HistoricalCoordinator) PrepareAICommandHandoffBatch(ctx context.Context, page *HistoricalSourcePage, reverse *AIReverseSnapshot) (*AICommandHandoffBatch, error) {
	if c == nil || ctx == nil || reverse == nil || reverse.self != reverse {
		return nil, ErrAILocalBinding
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now == nil || c.alive(ctx) != nil || c.pageValid(page) != nil || c.authenticated == nil || !c.authenticated.complete || reverse.scope == nil || reverse.scope.owner != c || reverse.scope.auth != c.authenticated || reverse.scope.verifiedEntries != c.authenticated.entries || reverse.scope.missingCurrent || reverse.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAILocalBinding
	}
	if len(reverse.metadata) != len(aiReverseSpecs) || len(reverse.report.Ledgers) != len(aiReverseSpecs) || len(reverse.structuralReasons) != 0 || reverse.report.Unknown != 0 || reverse.dataDigest() != reverse.report.DataSHA256 {
		return nil, ErrAILocalUnknown
	}
	for _, node := range reverse.nodes {
		if node == nil || node.observation.Invalid || node.observation.Scope == "unknown" || len(node.observation.Reasons) != 0 {
			return nil, ErrAILocalUnknown
		}
	}
	admission := reverse.byTable["ai_messaging_admission"]["1"]
	if admission == nil || admission.state != "closed" {
		return nil, ErrAILocalBinding
	}
	binding := AIResolverBinding{DatabaseIdentityHash: reverse.report.DatabaseIdentitySHA256, MigrationVersion: reverse.head, SourceSHA: c.binding.SourceSHA, OperationID: c.binding.OperationID, AdmissionRevision: admission.sequence, BridgeBoundary: c.copies[1].Expected.Boundary, LegacyBoundary: c.copies[2].Expected.Boundary}
	for i, spec := range aiReverseSpecs {
		if reverse.report.Ledgers[i].Store != spec.table || reverse.report.Ledgers[i].SchemaSHA256 != reverse.metadata[i].schema || reverse.report.Ledgers[i].PrimaryKeySHA256 != reverse.metadata[i].pk {
			return nil, ErrAILocalBinding
		}
		switch spec.table {
		case AIBridgeCommandSource:
			binding.BridgeColumns = aiCloneColumns(reverse.metadata[i].sourceColumns)
		case AILegacyCommandSource:
			binding.LegacyColumns = aiCloneColumns(reverse.metadata[i].sourceColumns)
		}
	}
	if !aiValidBinding(binding) {
		return nil, ErrAILocalBinding
	}
	p := &AICommandHandoffBatch{binding: binding, oldPool: reverse.pool, started: reverse.started, limits: reverse.limits, copies: reverse.scope.receipts, ledgers: append([]AIReverseLedgerSummary(nil), reverse.report.Ledgers...), expires: reverse.started.Add(reverse.limits.MaxDuration)}
	for _, deadline := range []time.Time{c.started.Add(c.limits.MaxDuration), page.issued.Add(c.limits.PageTTL)} {
		if deadline.Before(p.expires) {
			p.expires = deadline
		}
	}
	p.anchorMetadataSHA = reverse.anchorMetadataSHA
	for _, a := range reverse.anchors {
		p.anchors = append(p.anchors, aiCommandHandoffAnchor{a.id, a.org, a.testee, a.sheet, a.rawSHA})
	}
	sort.Slice(p.anchors, func(i, j int) bool {
		left, _ := strconv.ParseUint(p.anchors[i].ID, 10, 64)
		right, _ := strconv.ParseUint(p.anchors[j].ID, 10, 64)
		return left < right
	})
	p.metadata = make([]aiReverseMetadata, len(reverse.metadata))
	for i, meta := range reverse.metadata {
		p.metadata[i] = aiReverseMetadata{columns: append([]string(nil), meta.columns...), sourceColumns: aiCloneColumns(meta.sourceColumns), schema: meta.schema, pk: meta.pk}
	}
	verifiedAt := time.Now().UTC()
	seen := map[string]bool{}
	for _, row := range page.rows {
		if row.event != nil {
			continue
		}
		if row.bridge == nil || row.bridge.facts == nil || row.legacy == nil || row.legacy.facts == nil {
			// Delivered/published is not business closure. There is no actual Go
			// qs-ai 53-ledger closure adapter for an unmapped historical ID yet.
			return nil, ErrAILocalUnknown
		}
		bridge, err := c.authenticated.BindAICommand(row.bridge.facts)
		if err != nil {
			return nil, err
		}
		legacy, err := c.authenticated.BindAICommand(row.legacy.facts)
		if err != nil {
			return nil, err
		}
		if ValidateAISourcePair(bridge.facts, legacy.facts) != nil || seen[bridge.facts.CommandID] {
			return nil, ErrAISourceHandoff
		}
		seen[bridge.facts.CommandID] = true
		if err := aiCommandHandoffNodes(reverse, bridge.facts, legacy.facts); err != nil {
			return nil, err
		}
		p.evidence = append(p.evidence, aiCommandHandoffEvidence(binding, bridge.facts, legacy.facts, verifiedAt))
	}
	if len(p.evidence) == 0 || len(p.evidence) > c.limits.MaxAICommands || reverse.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAILocalBinding
	}
	p.self = p
	p.seal = p.digest()
	if !p.intact() || time.Now().After(p.expires) {
		return nil, ErrAILocalBounds
	}
	return p, nil
}

func aiCommandHandoffNodes(s *AIReverseSnapshot, bridge, legacy *DecodedAICommand) error {
	if s == nil || ValidateAISourcePair(bridge, legacy) != nil {
		return ErrAISourceHandoff
	}
	id := bridge.CommandID
	old := s.byTable[AIBridgeCommandSource][id]
	mapped := s.byTable[AILegacyCommandSource][id]
	op := s.byTable["ai_messaging_operations"][id]
	box := s.byTable["ai_messaging_outbox"][id]
	request := s.byTable["ai_bridge_requests"][bridge.RequestID]
	for _, node := range []*aiReverseNode{old, mapped, op, box, request} {
		if node == nil || node.observation.Invalid || node.observation.Scope != "retirement_related" || len(node.observation.Reasons) != 0 {
			return ErrAILocalRelation
		}
		if node.observation.Held {
			return ErrAILocalResponsibility
		}
	}
	if old.observation.Unfinished || op.retired || old.sourceRowHash != bridge.Source.Digest.SHA256 || mapped.sourceRowHash != legacy.Source.Digest.SHA256 || op.bodyHash != legacy.Transport.MessagingBodySHA256 || box.bodyHash != op.bodyHash || op.aggregate != bridge.RequestID || op.resource != bridge.ResourceID || op.org != bridge.OrganizationID || op.subject != bridge.SubjectID || box.aggregate != op.aggregate || box.sequence != op.sequence || box.org != op.org || box.subject != op.subject || box.resource != op.resource || op.sequence == 0 {
		return ErrAILocalRelation
	}
	// Known current-protocol pending is retained. No old publisher work may
	// remain under this owner, and neither held nor unresolved rows are adopted.
	for _, node := range s.nodes {
		if node.request != bridge.RequestID && node.aggregate != bridge.RequestID && node.id != id && node.command != id {
			continue
		}
		if node.observation.Held || node.observation.Invalid || len(node.observation.Reasons) != 0 {
			return ErrAILocalResponsibility
		}
		if !node.observation.Unfinished {
			continue
		}
		switch node.observation.Store {
		case "ai_bridge_requests":
		case "ai_messaging_operations":
			if node.retired || node.state != "" {
				return ErrAILocalResponsibility
			}
		case "ai_messaging_outbox":
			if node.state != "staged" && node.state != "awaiting_receipt" {
				return ErrAILocalResponsibility
			}
		default:
			return ErrAILocalResponsibility
		}
	}
	return nil
}

func aiCommandHandoffEvidence(binding AIResolverBinding, bridge, legacy *DecodedAICommand, at time.Time) store.CommandRetirementEvidence {
	return store.CommandRetirementEvidence{Version: 1, OperationID: binding.OperationID, VerifierVersion: "qs-original-handoff/v1", VerificationMethod: "source_identity_hash_and_live_ledger", VerifiedAt: at.UTC(), AdmissionRevision: binding.AdmissionRevision, CommandID: bridge.CommandID, RequestID: bridge.RequestID, SourceKind: bridge.SourceKind, OrganizationID: bridge.OrganizationID, SubjectID: bridge.SubjectID, ResourceID: bridge.ResourceID, LiveBodySHA256: legacy.Transport.MessagingBodySHA256, Sources: []store.CommandRetirementSource{{Table: AIBridgeCommandSource, CommandID: bridge.CommandID, BytesKind: bridge.PayloadBytesDigest.Kind, BytesSHA256: bridge.PayloadBytesDigest.SHA256, BusinessPayloadHash: bridge.WriterPayloadDigest.SHA256}, {Table: AILegacyCommandSource, CommandID: legacy.CommandID, BytesKind: legacy.PayloadBytesDigest.Kind, BytesSHA256: legacy.PayloadBytesDigest.SHA256, BusinessPayloadHash: legacy.WriterPayloadDigest.SHA256}}, References: []store.CommandRetirementReference{{Kind: "business_record", ID: bridge.RequestID}, {Kind: "operation", ID: bridge.CommandID}, {Kind: "readonly_run", ID: binding.OperationID}}, Conclusion: "transferred_verified", Reason: "handoff_verified", OwnershipVerified: true, ResponsibilityClosed: false, BusinessTerminal: false}
}

func (p *AICommandHandoffBatch) digest() string {
	if p == nil {
		return ""
	}
	return aiJSONHash(struct {
		Binding           AIResolverBinding
		Started, Expires  time.Time
		Limits            AIReverseLimits
		Metadata          []aiReverseMetadataSeal
		Ledgers           []AIReverseLedgerSummary
		Anchors           []aiCommandHandoffAnchor
		AnchorMetadataSHA string
		Copies            [4]SourceCopyReceipt
		Evidence          []store.CommandRetirementEvidence
	}{p.binding, p.started, p.expires, p.limits, aiCommandHandoffMetadataSeal(p.metadata), p.ledgers, p.anchors, p.anchorMetadataSHA, p.copies, p.evidence})
}
func (p *AICommandHandoffBatch) intact() bool {
	return p != nil && p.self == p && p.oldPool != nil && aiValidBinding(p.binding) && len(p.evidence) > 0 && len(p.metadata) == len(aiReverseSpecs) && len(p.ledgers) == len(aiReverseSpecs) && p.limits.valid() && evidenceHash(p.anchorMetadataSHA) && !p.expires.IsZero() && evidenceHash(p.seal) && p.seal == p.digest()
}

// Record borrows the host's NEW actual RW transaction. It performs one full14
// current locking read for the whole batch, then delegates source/request/op/
// outbox CAS to the existing store. It never stages, sends, commits or closes.
// On every error the HOST MUST roll back; a partial batch cannot be retried or
// adopted by this object. The host's durable journal handles crash reconciliation.
func (p *AICommandHandoffBatch) Record(ctx context.Context, tx *gorm.DB) (AICommandHandoffResult, error) {
	if p == nil || p.self != p {
		return AICommandHandoffResult{}, ErrAILocalBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.intact() || p.attempted || ctx == nil || ctx.Err() != nil || time.Now().After(p.expires) {
		return AICommandHandoffResult{}, ErrAILocalChanged
	}
	p.attempted = true
	resolver, err := NewAILocalResolver(ctx, tx, p.binding)
	if err != nil {
		return AICommandHandoffResult{}, err
	}
	actual, err := hostmysql.RequireTx(ctx)
	if err != nil || actual == nil || actual.Statement == nil || actual.Statement.ConnPool == nil {
		return AICommandHandoffResult{}, ErrAILocalTransaction
	}
	if prepared, ok := actual.Statement.ConnPool.(*gorm.PreparedStmtTX); ok && prepared == nil {
		return AICommandHandoffResult{}, ErrAILocalTransaction
	}
	if _, err := sdkmysql.BindGORM(actual); err != nil {
		return AICommandHandoffResult{}, ErrAILocalTransaction
	}
	if actual.Statement.ConnPool != resolver.pool || resolver.pool == p.oldPool {
		return AICommandHandoffResult{}, ErrAILocalTransaction
	}
	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	rows, ended := p.oldPool.QueryContext(qctx, "SELECT 1")
	if rows != nil && rows.Close() != nil {
		ended = ErrAILocalRead
	}
	cancel()
	if !errors.Is(ended, sql.ErrTxDone) {
		return AICommandHandoffResult{}, ErrAILocalTransaction
	}
	admission, err := resolver.read(ctx, aiAdmissionQuery)
	if err != nil {
		return AICommandHandoffResult{}, err
	}
	closed, revision, _, err := aiAdmissionSnapshot(admission)
	if err != nil || closed != "1" || revision != p.binding.AdmissionRevision {
		return AICommandHandoffResult{}, ErrAILocalChanged
	}
	before, err := p.lockCurrentLedgers(ctx, resolver)
	if err != nil {
		return AICommandHandoffResult{}, err
	}
	for _, conclusion := range p.evidence {
		if ctx.Err() != nil || time.Now().After(p.expires) {
			return AICommandHandoffResult{}, ErrAILocalBounds
		}
		if err := store.RecordOperationTransferEvidence(ctx, tx, conclusion); err != nil {
			return AICommandHandoffResult{}, ErrAILocalChanged
		}
		if err := p.verifyWrittenOperation(ctx, resolver, before[conclusion.CommandID], conclusion); err != nil {
			return AICommandHandoffResult{}, err
		}
	}
	if err := p.verifyCurrentMetadata(ctx, resolver); err != nil {
		return AICommandHandoffResult{}, err
	}
	if !p.intact() || ctx.Err() != nil || time.Now().After(p.expires) {
		return AICommandHandoffResult{}, ErrAILocalBounds
	}
	return AICommandHandoffResult{Scope: "old-publisher-handoff-evidence-in-borrowed-transaction", Records: uint64(len(p.evidence)), EvidenceSHA256: aiJSONHash(p.evidence), HostCommitRequired: true}, nil
}

// Keep equality explicit for nullable/raw fields; no reserialization can hide
// a changed receipt, decision, original clock, body, sequence or retirement ID.
func aiCommandHandoffMetadataEqual(a, b []aiReverseMetadata) bool { return reflect.DeepEqual(a, b) }
