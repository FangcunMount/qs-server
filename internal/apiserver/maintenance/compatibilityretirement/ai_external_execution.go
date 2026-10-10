package retirement

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
)

const aiExternalHostSHA = "e1d06df65b11ea66fe8930ca0012f6ac3b8c120888f490f399236cbb3d0b5268"
const aiExternalResultLimit = 32 << 20
const aiExternalInputLimit = 16 << 20

var (
	ErrAIExternalInput     SourceError = "ai_external_execution_input_required"
	ErrAIExternalRuntime   SourceError = "ai_external_actual_runtime_binding_rejected"
	ErrAIExternalExecution SourceError = "ai_external_actual_execution_rejected_or_outcome_unknown"
	ErrAIExternalChanged   SourceError = "ai_external_execution_original_facts_changed"
)

// These inputs constrain an actual read-only call. They are not approval,
// execution/fence permission or a deserializable external qualification.
// The host independently supplies the real qs-ai image/source and approved
// bounds. There is no executable, command, SQL, namespace default or closure flag.
type AIExternalExecutionInput struct {
	// Same actual operation_directory(root, operation_id) across attempts.
	// Never an attempt/run child directory or a caller-generated journal name.
	OperationDirectory                               string
	AssetsDirectory                                  string
	RunID                                            string
	RuntimeSourceSHA, ImageID, ContainerID           string
	AIBounds, PeerBounds                             []byte
	ApprovedAIBoundsSHA256, ApprovedPeerBoundsSHA256 string
	ApprovedAIRuntimeBindingSHA256                   string
	PeerConnection                                   AIExternalPeerConnection
	ProtectionJSON                                   []byte
	SudoDocker                                       bool // Fixed sudo -n Docker executor only; it grants no capability.
}
type AIExternalPeerConnection struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (AIExternalExecutionInput) String() string               { return "private actual external AI execution inputs" }
func (v AIExternalExecutionInput) GoString() string           { return v.String() }
func (AIExternalExecutionInput) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (AIExternalPeerConnection) String() string               { return "private host-owned peer connection" }
func (v AIExternalPeerConnection) GoString() string           { return v.String() }

func (AIExternalPeerConnection) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }

type aiExternalPeerConnectionWire AIExternalPeerConnection

type aiExternalSection struct {
	Rows  uint64 `json:"rows"`
	Bytes uint64 `json:"source_bytes"`
	SHA   string `json:"source_sha256"`
}
type aiExternalSource struct {
	Table      string `json:"table"`
	PayloadSHA string `json:"payload_bytes_sha256"`
}
type aiExternalOriginal struct {
	CommandID       string             `json:"command_id"`
	RequestID       string             `json:"request_id"`
	SessionID       string             `json:"session_id"`
	OrganizationID  string             `json:"organization_id"`
	SubjectID       string             `json:"subject_id"`
	ResourceID      string             `json:"resource_id"`
	TesteeID        string             `json:"testee_id"`
	ActiveRunID     string             `json:"active_run_id"`
	TerminalEventID string             `json:"terminal_event_id"`
	WriterSHA       string             `json:"writer_sha256"`
	RequestSHA      string             `json:"original_request_sha256"`
	Attempts        uint64             `json:"attempts"`
	HandoffSHA      *string            `json:"handoff_sha256"`
	Status          string             `json:"status"`
	Result          string             `json:"execution_result"`
	Sources         []aiExternalSource `json:"sources"`
}
type aiExternalSnapshot struct {
	Side        string `json:"side"`
	Identity    string `json:"identity_hash"`
	Head        string `json:"head"`
	CatalogSHA  string `json:"catalog_sha256"`
	SectionsSHA string `json:"sections_sha256"`
}
type aiExternalPeerRow struct {
	Store         string `json:"store"`
	PrimaryKeySHA string `json:"primary_key_sha256"`
	RowSHA        string `json:"row_sha256"`
}
type aiExternalFacts struct {
	Protocol          string                       `json:"protocol"`
	SourceSHA         string                       `json:"source_sha"`
	OperationID       string                       `json:"operation_id"`
	RunID             string                       `json:"run_id"`
	RuntimeSourceSHA  string                       `json:"runtime_source_sha"`
	RuntimeBindingSHA string                       `json:"runtime_binding_sha256"`
	ImageID           string                       `json:"image_id"`
	ContainerID       string                       `json:"container_id"`
	AIBoundsSHA       string                       `json:"ai_bounds_sha256"`
	PeerBoundsSHA     string                       `json:"peer_bounds_sha256"`
	Snapshots         []aiExternalSnapshot         `json:"snapshots"`
	Sections          map[string]aiExternalSection `json:"original_sections"`
	Originals         []aiExternalOriginal         `json:"originals"`
	PeerRows          []aiExternalPeerRow          `json:"peer_rows"`
	KnownHandoffs     []aiExternalKnownHandoff     `json:"known_handoffs"`
}
type aiExternalMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}
type aiExternalRuntime struct {
	ContainerID, ImageID, Name, Status, StartedAt, ContainerRevision, ImageRevision string
	Running, ReadOnlyRoot                                                           bool
	Restarts                                                                        int64
	Entrypoint, Command                                                             []string
	Mounts                                                                          []aiExternalMount
}

// The only factory performs the real fixed producer call below. A received
// report/JSON, caller complete flag or local summary cannot create this object.
// It proves two observed database epochs, not a distributed snapshot, runtime
// execution fence, Broker queue coverage, retirement persistence or DROP.
type AIExternalExecutionQualification struct {
	self     *AIExternalExecutionQualification
	owner    *HistoricalCoordinator
	local    *AIReadOnlyResolver
	reverse  *AIReverseSnapshot
	facts    aiExternalFacts
	byID     map[string]aiExternalOriginal
	handoffs map[string]aiExternalKnownHandoff
	expires  time.Time
	seal     string
}
type AIExternalExecutionSummary struct {
	Scope                                                                    string
	Originals                                                                uint64
	FactsSHA256                                                              string
	IndependentEpochs                                                        int
	ExternalDatabaseFactsObserved                                            bool
	ProductionApproval, WriterFence, BrokerCoverage, CASAuthority, DropReady bool
}

func (*AIExternalExecutionQualification) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*AIExternalExecutionQualification) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*AIExternalExecutionQualification) UnmarshalJSON([]byte) error {
	return ErrSourceSerialization
}
func (*AIExternalExecutionQualification) UnmarshalBSON([]byte) error {
	return ErrSourceSerialization
}
func (*AIExternalExecutionQualification) String() string {
	return "private actual external AI database execution qualification; no write authority"
}
func (q *AIExternalExecutionQualification) GoString() string { return q.String() }
func (q *AIExternalExecutionQualification) Summary() AIExternalExecutionSummary {
	if q == nil || q.self != q || q.seal != aiJSONHash(q.facts) {
		return AIExternalExecutionSummary{}
	}
	return AIExternalExecutionSummary{Scope: "actual-full-qs-ai-and-peer-two-epoch-database-facts-only", Originals: uint64(len(q.byID) + len(q.handoffs)), FactsSHA256: q.seal, IndependentEpochs: 2, ExternalDatabaseFactsObserved: true}
}

// PrepareAIExternalExecution never opens/closes the borrowed Go transaction.
// The fixed Python host owns its separate ai/peer read-only pools and sessions.
// The controller accepts only the producer it just executed, never saved output.
func (c *HistoricalCoordinator) PrepareAIExternalExecution(ctx context.Context, local *AIReadOnlyResolver, reverse *AIReverseSnapshot, in AIExternalExecutionInput) (*AIExternalExecutionQualification, error) {
	return c.prepareAIExternalExecution(ctx, local, reverse, in, aiExternalVerifyMode, nil)
}

// The window's fixed final phase runs a complete fresh native producer after
// a known successful original phase. It cannot select another mode or reuse a
// saved Q, and never changes the original operation, journal or business IDs.
func (c *HistoricalCoordinator) PrepareAIFinalExternalExecution(ctx context.Context, local *AIReadOnlyResolver, reverse *AIReverseSnapshot, in AIExternalExecutionInput) (*AIExternalExecutionQualification, error) {
	return c.prepareAIExternalExecution(ctx, local, reverse, in, aiExternalFinalVerifyMode, nil)
}

// The stopped-final path reuses every frozen ledger/module check below. The
// carrier lease is an original live owner, never a caller completion receipt.
func (c *HistoricalCoordinator) PrepareAIStoppedFinalExternalExecution(ctx context.Context, local *AIReadOnlyResolver, reverse *AIReverseSnapshot, in AIExternalExecutionInput, lease *AIStoppedRuntimeLease) (*AIExternalExecutionQualification, error) {
	if lease == nil {
		return nil, ErrAIStoppedRuntime
	}
	return c.prepareAIExternalExecution(ctx, local, reverse, in, aiExternalFinalVerifyMode, lease)
}

func (c *HistoricalCoordinator) prepareAIExternalExecution(ctx context.Context, local *AIReadOnlyResolver, reverse *AIReverseSnapshot, in AIExternalExecutionInput, mode aiExternalExecMode, stopped *AIStoppedRuntimeLease) (*AIExternalExecutionQualification, error) {
	if c == nil || ctx == nil {
		return nil, ErrAIExternalInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now == nil || c.alive(ctx) != nil || c.authenticated == nil || !c.authenticated.complete || local == nil || local.owner != c || local.validate(ctx) != nil || reverse == nil || reverse.self != reverse || reverse.scope == nil || reverse.scope.owner != c || reverse.scope.auth != c.authenticated || reverse.pool != local.inner.pool || reverse.ValidateBorrowedSnapshot(ctx) != nil || reverse.scope.missingCurrent {
		return nil, ErrAILocalBinding
	}
	if !aiOriginalSourceSHA(in.RuntimeSourceSHA) || !aiExternalImageID(in.ImageID) || len(in.ContainerID) != 64 || !evidenceHash(in.ContainerID) || !aiExternalRunID(in.RunID) || !evidenceHash(in.ApprovedAIBoundsSHA256) || !evidenceHash(in.ApprovedPeerBoundsSHA256) || !evidenceHash(in.ApprovedAIRuntimeBindingSHA256) || sourceSHA(in.AIBounds) != in.ApprovedAIBoundsSHA256 || sourceSHA(in.PeerBounds) != in.ApprovedPeerBoundsSHA256 || len(in.AIBounds) > 4<<20 || len(in.PeerBounds) > 4<<20 || strictJSON(in.ProtectionJSON) != nil {
		return nil, ErrAIExternalInput
	}
	if reverse.report.Unknown != 0 || len(reverse.structuralReasons) != 0 || len(reverse.report.Ledgers) != 14 || reverse.report.DatabaseIdentitySHA256 != local.inner.binding.DatabaseIdentityHash {
		return nil, ErrAILocalUnknown
	}
	assets, err := aiExternalAssets(in.AssetsDirectory)
	if err != nil {
		return nil, err
	}
	release, err := aiExternalReadRelease(in.RuntimeSourceSHA, in.ImageID)
	if err != nil || release.seal != in.ApprovedAIRuntimeBindingSHA256 {
		return nil, ErrAIExternalRuntime
	}
	sections := map[string]aiExternalSection{}
	for _, index := range []int{1, 2} {
		expected := c.copies[index].Expected
		sections[expected.Boundary.Name] = aiExternalSection{expected.Records, expected.Bytes, expected.DataHash}
	}
	if len(sections) != 2 || sections[AIBridgeCommandSource].Rows+sections[AILegacyCommandSource].Rows > 10000 {
		return nil, ErrAIExternalInput
	}
	knownHandoffs, err := aiExternalActualHandoffs(ctx, c, reverse)
	if err != nil {
		return nil, err
	}
	packet := struct {
		Protocol          string                       `json:"protocol"`
		SourceSHA         string                       `json:"source_sha"`
		OperationID       string                       `json:"operation_id"`
		RunID             string                       `json:"run_id"`
		RuntimeSHA        string                       `json:"runtime_source_sha"`
		ImageID           string                       `json:"image_id"`
		ContainerID       string                       `json:"container_id"`
		RuntimeBindingSHA string                       `json:"runtime_binding_sha256"`
		RuntimeMessaging  json.RawMessage              `json:"runtime_messaging"`
		AIBounds          string                       `json:"ai_bounds"`
		AIBoundsSHA       string                       `json:"ai_bounds_sha256"`
		PeerBounds        string                       `json:"peer_bounds"`
		PeerBoundsSHA     string                       `json:"peer_bounds_sha256"`
		Sections          map[string]aiExternalSection `json:"original_sections"`
		Peer              aiExternalPeerConnectionWire `json:"peer_connection"`
		Protection        json.RawMessage              `json:"protection"`
		Modules           map[string]string            `json:"modules"`
		KnownHandoffs     []aiExternalKnownHandoff     `json:"known_handoffs"`
	}{"qs-ai-readonly-host-input/v3", c.binding.SourceSHA, c.binding.OperationID, in.RunID, in.RuntimeSourceSHA, in.ImageID, in.ContainerID, release.seal, release.messaging, base64.StdEncoding.EncodeToString(in.AIBounds), in.ApprovedAIBoundsSHA256, base64.StdEncoding.EncodeToString(in.PeerBounds), in.ApprovedPeerBoundsSHA256, sections, aiExternalPeerConnectionWire(in.PeerConnection), in.ProtectionJSON, assets.modules, knownHandoffs}
	raw, err := json.Marshal(packet)
	if err != nil || len(raw) > aiExternalInputLimit {
		return nil, ErrAIExternalInput
	}
	// The original actual epoch's lifetime still applies; this does not extend it.
	deadline := c.started.Add(c.limits.MaxDuration)
	if d := reverse.started.Add(reverse.limits.MaxDuration); d.Before(deadline) {
		deadline = d
	}
	if d := time.Now().Add(1520 * time.Second); d.Before(deadline) {
		deadline = d
	}
	work, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var result []byte
	if stopped != nil {
		if mode != aiExternalFinalVerifyMode {
			return nil, ErrAIStoppedRuntime
		}
		result, err = stopped.executeFinal(work, c.binding, in, assets.host, raw)
	} else {
		docker, e := aiExternalDocker(in.SudoDocker)
		if e != nil {
			return nil, e
		}
		before, e := docker.inspect(work, in.ContainerID)
		if e != nil || !before.matches(in) || !release.matchesMounts(before.Mounts) {
			return nil, ErrAIExternalRuntime
		}
		result, err = aiExternalExecuteMode(work, docker, in.OperationDirectory, mode, c.binding, in.RunID, in.RuntimeSourceSHA, in.ImageID, in.ContainerID, assets.host, raw)
		if err == nil {
			after, e := docker.inspect(work, in.ContainerID)
			if e != nil || !reflect.DeepEqual(before, after) || !after.matches(in) || !release.matchesMounts(after.Mounts) {
				return nil, ErrAIExternalRuntime
			}
		}
	}
	if err != nil {
		return nil, ErrAIExternalExecution
	}
	repeated, err := aiExternalReadRelease(in.RuntimeSourceSHA, in.ImageID)
	if err != nil || repeated.seal != release.seal || !reflect.DeepEqual(repeated, release) {
		return nil, ErrAIExternalRuntime
	}
	facts, err := aiExternalDecodeActualResult(result)
	if err != nil || facts.SourceSHA != c.binding.SourceSHA || facts.OperationID != c.binding.OperationID || facts.RunID != in.RunID || facts.RuntimeSourceSHA != in.RuntimeSourceSHA || facts.RuntimeBindingSHA != release.seal || facts.ImageID != in.ImageID || facts.ContainerID != in.ContainerID || facts.AIBoundsSHA != in.ApprovedAIBoundsSHA256 || facts.PeerBoundsSHA != in.ApprovedPeerBoundsSHA256 || !reflect.DeepEqual(facts.Sections, sections) || len(facts.Snapshots) != 2 {
		return nil, ErrAIExternalChanged
	}
	peerSnapshot := facts.Snapshots[1]
	if facts.Snapshots[0].Side != "ai" || peerSnapshot.Side != "peer" || peerSnapshot.Identity != local.inner.binding.DatabaseIdentityHash || peerSnapshot.Head != strconv.FormatUint(local.inner.binding.MigrationVersion, 10) {
		return nil, ErrAIExternalChanged
	}
	if err = aiExternalPeerRowsMatch(facts.PeerRows, reverse); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(facts.KnownHandoffs, knownHandoffs) {
		return nil, ErrAIExternalChanged
	}
	handoffs := make(map[string]aiExternalKnownHandoff, len(knownHandoffs))
	for _, entry := range knownHandoffs {
		if _, exists := handoffs[entry.CommandID]; exists {
			return nil, ErrAIExternalChanged
		}
		handoffs[entry.CommandID] = entry
	}
	byID := make(map[string]aiExternalOriginal, len(facts.Originals))
	for _, original := range facts.Originals {
		if _, duplicate := byID[original.CommandID]; duplicate || !aiExternalOriginalValid(original) || handoffs[original.CommandID].CommandID != "" {
			return nil, ErrAIExternalChanged
		}
		byID[original.CommandID] = original
	}
	// Match every actual source row, including duplicates across the two tables,
	// through its authenticated source frame. No sampled/caller target list exists.
	for _, source := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		for _, node := range reverse.byTable[source] {
			if _, mapped := handoffs[node.id]; mapped {
				continue
			}
			original, ok := byID[node.id]
			if !ok || original.RequestID != node.request || original.OrganizationID != node.org || original.SubjectID != node.subject || original.ResourceID != node.resource || original.TesteeID != node.testee || original.WriterSHA != node.hash || original.Attempts != node.attempts {
				return nil, ErrAIExternalChanged
			}
		}
	}
	if len(byID)+len(handoffs) != len(reverse.byTable[AIBridgeCommandSource]) || len(handoffs) != len(reverse.byTable[AILegacyCommandSource]) || local.validate(ctx) != nil || reverse.ValidateBorrowedSnapshot(ctx) != nil || c.alive(ctx) != nil {
		return nil, ErrAIExternalChanged
	}
	q := &AIExternalExecutionQualification{owner: c, local: local, reverse: reverse, facts: facts, byID: byID, handoffs: handoffs, expires: deadline}
	q.self = q
	q.seal = aiJSONHash(q.facts)
	return q, nil
}

// PrepareAIExternalPage binds only actual NextPage source capabilities to the
// actual external result. Returned metadata stays private and is NOT a write
// permit. The lifecycle host still needs fence/fresh-origin/closed-admission
// CAS and independent readback before using the existing retirement store.
type AIExternalPageQualification struct {
	self     *AIExternalPageQualification
	page     *HistoricalSourcePage
	covered  map[verifiedSourceKey]string
	guard    *AICommandHandoffBatch
	handoff  *AICommandHandoffBatch
	seal     string
	owner    *AIExternalExecutionQualification
	evidence []store.CommandRetirementEvidence
}

func (*AIExternalPageQualification) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIExternalPageQualification) String() string {
	return "private external original-command facts; no CAS or DROP authority"
}
func (p *AIExternalPageQualification) GoString() string { return p.String() }
func (c *HistoricalCoordinator) PrepareAIExternalPage(ctx context.Context, page *HistoricalSourcePage, q *AIExternalExecutionQualification) (*AIExternalPageQualification, error) {
	if c == nil || ctx == nil {
		return nil, ErrAIExternalInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prepareAIExternalRowsLocked(ctx, page, q, pageRows(page))
}

func pageRows(page *HistoricalSourcePage) []coordinatorRow {
	if page == nil {
		return nil
	}
	return page.rows
}
func (c *HistoricalCoordinator) prepareAIExternalRowsLocked(ctx context.Context, page *HistoricalSourcePage, q *AIExternalExecutionQualification, rows []coordinatorRow) (*AIExternalPageQualification, error) {
	if q == nil || q.self != q || q.owner != c || q.seal != aiJSONHash(q.facts) || time.Now().After(q.expires) || c.now == nil || c.pageValid(page) != nil || c.alive(ctx) != nil || q.local.validate(ctx) != nil || q.reverse.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAIExternalChanged
	}
	p := &AIExternalPageQualification{owner: q}
	for _, row := range rows {
		if row.event != nil || row.bridge == nil {
			return nil, ErrCoordinatorPage
		}
		bridge, err := row.bridge.Facts()
		if err != nil {
			return nil, err
		}
		original, ok := q.byID[bridge.CommandID]
		if !ok || original.CommandID != bridge.RequestID || original.WriterSHA != bridge.WriterPayloadDigest.SHA256 || original.OrganizationID != bridge.OrganizationID || original.SubjectID != bridge.SubjectID || original.ResourceID != bridge.ResourceID || original.TesteeID != bridge.Business.TesteeID || original.Attempts != uint64(bridge.Transport.SourceAttempts) {
			return nil, ErrAIExternalChanged
		}
		// A live mapping is handled by the existing publisher handoff caller;
		// this kernel does not retire its current operation or pending messages.
		if row.legacy != nil || original.HandoffSHA != nil {
			return nil, ErrAILocalResponsibility
		}
		if len(original.Sources) != 1 || original.Sources[0].Table != AIBridgeCommandSource || original.Sources[0].PayloadSHA != bridge.PayloadBytesDigest.SHA256 {
			return nil, ErrAIExternalChanged
		}
		conclusion, reason := "verified", "history_terminal_verified"
		if original.Result == "historical_original_configuration_not_retained" {
			conclusion, reason = "unverifiable", "history_terminal_evidence_gap"
		}
		p.evidence = append(p.evidence, store.CommandRetirementEvidence{Version: 1, OperationID: c.binding.OperationID, VerifierVersion: "qs-ai-actual-execution/v1", VerificationMethod: "source_identity_hash_and_business_closure", VerifiedAt: time.Now().UTC(), AdmissionRevision: q.local.inner.binding.AdmissionRevision, CommandID: bridge.CommandID, RequestID: bridge.RequestID, SourceKind: bridge.SourceKind, OrganizationID: bridge.OrganizationID, SubjectID: bridge.SubjectID, ResourceID: bridge.ResourceID, Sources: []store.CommandRetirementSource{{Table: AIBridgeCommandSource, CommandID: bridge.CommandID, BytesKind: bridge.PayloadBytesDigest.Kind, BytesSHA256: bridge.PayloadBytesDigest.SHA256, BusinessPayloadHash: bridge.WriterPayloadDigest.SHA256}}, References: []store.CommandRetirementReference{{Kind: "business_record", ID: bridge.RequestID}, {Kind: "business_record", ID: "qs-ai/session/" + original.SessionID}, {Kind: "business_record", ID: "qs-ai/run/" + original.ActiveRunID}, {Kind: "event", ID: original.TerminalEventID}, {Kind: "migration_manifest", ID: q.seal}, {Kind: "readonly_run", ID: c.binding.OperationID}}, Conclusion: conclusion, Reason: reason, OwnershipVerified: true, ResponsibilityClosed: true, BusinessTerminal: true})
	}
	if len(p.evidence) == 0 {
		return nil, ErrCoordinatorPage
	}
	return p, nil
}

func aiExternalOriginalValid(v aiExternalOriginal) bool {
	if !aiOriginalUUID(v.CommandID) || v.CommandID != v.RequestID || !aiOriginalUUID(v.SessionID) || !aiOriginalUUID(v.ActiveRunID) || !aiOriginalUUID(v.TerminalEventID) || v.ResourceID != v.RequestID || !evidenceHash(v.WriterSHA) || !evidenceHash(v.RequestSHA) || v.SubjectID == "" || v.OrganizationID == "" || v.ResourceID == "" || len(v.Sources) == 0 || len(v.Sources) > 2 {
		return false
	}
	if v.Status != "completed" && v.Status != "cancelled" && v.Status != "blocked" {
		return false
	}
	switch v.Result {
	case "historical_original_configuration_not_retained", "frozen_configuration_and_artifact_verified", "known_terminal_without_artifact":
	default:
		return false
	}
	seen := map[string]bool{}
	for _, s := range v.Sources {
		if seen[s.Table] || s.Table != AIBridgeCommandSource && s.Table != AILegacyCommandSource || !evidenceHash(s.PayloadSHA) {
			return false
		}
		seen[s.Table] = true
	}
	return v.HandoffSHA == nil || evidenceHash(*v.HandoffSHA)
}
func aiExternalRunID(v string) bool {
	n, e := strconv.ParseUint(v, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == v
}
func aiExternalImageID(v string) bool {
	return strings.HasPrefix(v, "sha256:") && evidenceHash(strings.TrimPrefix(v, "sha256:"))
}
func aiOriginalSourceSHA(v string) bool {
	if len(v) != 40 {
		return false
	}
	for _, b := range []byte(v) {
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
			return false
		}
	}
	return true
}

func aiExternalDecodeActualResult(raw []byte) (aiExternalFacts, error) {
	var facts aiExternalFacts
	if len(raw) > aiExternalResultLimit || strictJSON(raw) != nil {
		return facts, ErrAIExternalExecution
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&facts) != nil || facts.Protocol != "qs-ai-actual-execution-facts/v3" {
		return facts, ErrAIExternalExecution
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return facts, ErrAIExternalExecution
	}
	for _, s := range facts.Snapshots {
		if !evidenceHash(s.Identity) || !evidenceHash(s.CatalogSHA) || !evidenceHash(s.SectionsSHA) {
			return facts, ErrAIExternalChanged
		}
	}
	return facts, nil
}
func aiExternalPeerRowsMatch(rows []aiExternalPeerRow, r *AIReverseSnapshot) error {
	want := map[string]string{}
	for _, n := range r.nodes {
		key := n.observation.Store + "/" + n.observation.PrimaryKeySHA256
		if _, dup := want[key]; dup {
			return ErrAIExternalChanged
		}
		want[key] = n.observation.RowSHA256
	}
	if len(rows) != len(want) {
		return ErrAIExternalChanged
	}
	for _, n := range rows {
		key := n.Store + "/" + n.PrimaryKeySHA
		if want[key] == "" || want[key] != n.RowSHA {
			return ErrAIExternalChanged
		}
		delete(want, key)
	}
	if len(want) != 0 {
		return ErrAIExternalChanged
	}
	return nil
}

type aiExternalAssetSet struct {
	host    []byte
	modules map[string]string
}

func aiExternalAssets(directory string) (aiExternalAssetSet, error) {
	out := aiExternalAssetSet{modules: map[string]string{}}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return out, ErrAIExternalInput
	}
	files := map[string]string{"qs-ai-retirement-readonly-host.py": aiExternalHostSHA, "qs-ai-retirement-readonly-verifier.py": "6a7e746e313a1028bd54edc878947c74c262d0e47af6de32b6c032fea035cee9", "qs-ai-retirement-readonly-observer.py": "20c501d0930a21c1e8be12416156d5635430cf6171e01d28a6db789e83f06d5a", "qs-ai-retirement-0040-layout.py": "690d68be91713641ad6ce13ad9ad126828c64fe780bda05522bd68697ead577f"}
	for name, want := range files {
		path := filepath.Join(directory, name)
		fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if e != nil {
			return out, ErrAIExternalInput
		}
		file := os.NewFile(uintptr(fd), name)
		before, e := file.Stat()
		if e != nil {
			_ = file.Close()
			return out, ErrAIExternalInput
		} // Already rejected; cleanup errors cannot grant qualification.
		if !before.Mode().IsRegular() || before.Size() > 1<<20 || before.Mode().Perm()&0022 != 0 {
			_ = file.Close()
			return out, ErrAIExternalInput
		} // Already rejected; cleanup errors cannot grant qualification.
		stat, ok := before.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink != 1 || int(stat.Uid) != os.Geteuid() && stat.Uid != 0 {
			_ = file.Close()
			return out, ErrAIExternalInput
		} // Already rejected; cleanup errors cannot grant qualification.
		raw, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		after, statErr := file.Stat()
		named, nameErr := os.Lstat(path)
		closeErr := file.Close()
		if readErr != nil || statErr != nil || nameErr != nil || closeErr != nil || !os.SameFile(before, after) || !os.SameFile(before, named) || before.Size() != after.Size() || before.Size() != int64(len(raw)) || !before.ModTime().Equal(after.ModTime()) || sourceSHA(raw) != want {
			return out, ErrAIExternalInput
		}
		if name == "qs-ai-retirement-readonly-host.py" {
			out.host = raw
		} else {
			out.modules[name] = base64.StdEncoding.EncodeToString(raw)
		}
	}
	return out, nil
}

type aiExternalDockerExecutor struct {
	executable string
	prefix     []string
}

func aiExternalDocker(sudo bool) (*aiExternalDockerExecutor, error) {
	docker, e := aiExternalTrustedExecutable("docker")
	if e != nil {
		return nil, e
	}
	d := &aiExternalDockerExecutor{executable: docker}
	if sudo {
		path, e := aiExternalTrustedExecutable("sudo")
		if e != nil {
			return nil, e
		}
		d.executable = path
		d.prefix = []string{"-n", docker}
	}
	return d, nil
}
func aiExternalTrustedExecutable(name string) (string, error) {
	path, e := exec.LookPath(name)
	if e != nil {
		return "", ErrAIExternalRuntime
	}
	path, e = filepath.EvalSymlinks(path)
	if e != nil || !filepath.IsAbs(path) {
		return "", ErrAIExternalRuntime
	}
	for current := path; ; current = filepath.Dir(current) {
		stat, e := os.Lstat(current)
		if e != nil || stat.Mode()&os.ModeSymlink != 0 || stat.Mode().Perm()&0022 != 0 {
			return "", ErrAIExternalRuntime
		}
		owner, ok := stat.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != 0 {
			return "", ErrAIExternalRuntime
		}
		if current == path {
			if !stat.Mode().IsRegular() || stat.Mode().Perm()&0111 == 0 {
				return "", ErrAIExternalRuntime
			}
		} else if !stat.IsDir() {
			return "", ErrAIExternalRuntime
		}
		if current == "/" {
			break
		}
	}
	return path, nil
}

type aiExternalBoundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *aiExternalBoundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("fixed_private_output_limit")
	}
	return b.Buffer.Write(p)
}
func (d *aiExternalDockerExecutor) call(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, d.executable, append(append([]string(nil), d.prefix...), args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "HOME=/", "LC_ALL=C"}
	cmd.Stdin = bytes.NewReader(input)
	out := &aiExternalBoundedOutput{limit: aiExternalResultLimit}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if cmd.Run() != nil {
		return nil, ErrAIExternalExecution
	}
	return out.Bytes(), nil
}
func (d *aiExternalDockerExecutor) inspect(ctx context.Context, id string) (aiExternalRuntime, error) {
	// Fixed private projection excludes Env, addresses and secrets. Mount Source
	// paths stay private and bind the actual frozen release's individual files.
	const projection = `{{json .Id}}|{{json .Image}}|{{json .Name}}|{{json .State.Status}}|{{json .State.StartedAt}}|{{json .State.Running}}|{{json .RestartCount}}|{{json (index .Config.Labels "org.opencontainers.image.revision")}}|{{json .HostConfig.ReadonlyRootfs}}|{{json .Config.Entrypoint}}|{{json .Config.Cmd}}|[{{range $i, $m := .Mounts}}{{if $i}},{{end}}{"Type":{{json $m.Type}},"Source":{{json $m.Source}},"Destination":{{json $m.Destination}},"RW":{{json $m.RW}}}{{end}}]`
	raw, e := d.call(ctx, nil, "inspect", "--type", "container", "--format", projection, id)
	if e != nil || len(raw) > 32768 {
		return aiExternalRuntime{}, ErrAIExternalRuntime
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "|")
	if len(parts) != 12 {
		return aiExternalRuntime{}, ErrAIExternalRuntime
	}
	var r aiExternalRuntime
	values := []any{&r.ContainerID, &r.ImageID, &r.Name, &r.Status, &r.StartedAt, &r.Running, &r.Restarts, &r.ContainerRevision, &r.ReadOnlyRoot, &r.Entrypoint, &r.Command, &r.Mounts}
	for i, v := range values {
		if json.Unmarshal([]byte(parts[i]), v) != nil {
			return r, ErrAIExternalRuntime
		}
	}
	image, e := d.call(ctx, nil, "image", "inspect", "--format", `{{json .Id}}|{{json (index .Config.Labels "org.opencontainers.image.revision")}}`, r.ImageID)
	if e != nil || len(image) > 1024 {
		return r, ErrAIExternalRuntime
	}
	fields := strings.Split(strings.TrimSpace(string(image)), "|")
	var imageID string
	if len(fields) != 2 || json.Unmarshal([]byte(fields[0]), &imageID) != nil || json.Unmarshal([]byte(fields[1]), &r.ImageRevision) != nil || imageID != r.ImageID {
		return r, ErrAIExternalRuntime
	}
	return r, nil
}
func (r aiExternalRuntime) matches(in AIExternalExecutionInput) bool {
	_, e := time.Parse(time.RFC3339Nano, r.StartedAt)
	if r.ContainerID != in.ContainerID || r.ImageID != in.ImageID || r.Name != "/qs-ai" || r.Status != "running" || !r.Running || !r.ReadOnlyRoot || r.Restarts < 0 || e != nil || r.ContainerRevision != in.RuntimeSourceSHA || r.ImageRevision != in.RuntimeSourceSHA || len(r.Entrypoint) != 0 || !reflect.DeepEqual(r.Command, []string{"/app/.venv/bin/python", "-m", "qs_ai.bootstrap.server"}) {
		return false
	}
	seen := map[string]bool{}
	for _, mount := range r.Mounts {
		if seen[mount.Destination] {
			return false
		}
		seen[mount.Destination] = true
		switch mount.Destination {
		case "/run/qs-ai-tls/ca-chain.crt", "/run/qs-ai-tls/qs-ai-fullchain.crt", "/run/qs-ai-tls/qs-ai.key":
			if mount.Type != "bind" || mount.RW {
				return false
			}
		case "/tmp":
			if mount.Type != "tmpfs" {
				return false
			}
		default:
			if !aiExternalJOSEPath(mount.Destination, "", "") || mount.Type != "bind" || mount.RW {
				return false
			}
		}
	}
	return true
}

type aiExternalRuntimeMessaging struct {
	Decrypt   map[string]string `json:"decrypt_key_files"`
	Enabled   bool              `json:"enabled"`
	InFlight  int               `json:"max_in_flight"`
	NSQD      map[string]string `json:"nsqd"`
	Recipient string            `json:"qs_recipient_key_file"`
	Signers   map[string]string `json:"qs_signer_files"`
	Signing   string            `json:"signing_key_file"`
}
type aiExternalReleaseVolume struct {
	Bind struct {
		Create bool `json:"create_host_path"`
	} `json:"bind"`
	ReadOnly bool   `json:"read_only"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"`
}
type aiExternalRelease struct {
	seal      string
	messaging json.RawMessage
	mounts    []aiExternalReleaseVolume
}

func aiExternalJOSEPath(path, role, kid string) bool {
	if filepath.Dir(path) != "/run/qs-ai-jose" {
		return false
	}
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	if filepath.Base(path) != name+".json" || !regexp.MustCompile(`^(?:ai\.sign|ai\.encrypt|qs\.sign|qs\.encrypt)\.[a-z0-9][a-z0-9._-]{0,63}$`).MatchString(name) {
		return false
	}
	return (role == "" || strings.HasPrefix(name, role+".")) && (kid == "" || name == kid)
}
func aiExternalReleaseMounts(raw string, volumes []aiExternalReleaseVolume) (json.RawMessage, error) {
	if raw == "" || len(raw) > 16384 || strictJSON([]byte(raw)) != nil {
		return nil, ErrAIExternalRuntime
	}
	var config aiExternalRuntimeMessaging
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&config) != nil || !config.Enabled || config.InFlight != 1 || len(config.NSQD) < 1 || len(config.NSQD) > 8 || len(config.Decrypt) < 1 || len(config.Decrypt) > 8 || len(config.Signers) < 1 || len(config.Signers) > 8 || !aiExternalJOSEPath(config.Signing, "ai.sign", "") || !aiExternalJOSEPath(config.Recipient, "qs.encrypt", "") {
		return nil, ErrAIExternalRuntime
	}
	want := map[string]bool{config.Signing: true, config.Recipient: true}
	for _, pair := range []struct {
		paths map[string]string
		role  string
	}{{config.Decrypt, "ai.encrypt"}, {config.Signers, "qs.sign"}} {
		for kid, path := range pair.paths {
			if !aiExternalJOSEPath(path, pair.role, kid) || want[path] {
				return nil, ErrAIExternalRuntime
			}
			want[path] = true
		}
	}
	if len(volumes) != len(want) {
		return nil, ErrAIExternalRuntime
	}
	revision := ""
	seen := map[string]bool{}
	for _, v := range volumes {
		parts := strings.Split(v.Source, "/")
		if v.Type != "bind" || !v.ReadOnly || v.Bind.Create || !want[v.Target] || seen[v.Target] || len(parts) != 7 || strings.Join(parts[:5], "/") != "/data/infra/qs-ai-messaging/versions" || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`).MatchString(parts[5]) || v.Target != "/run/qs-ai-jose/"+parts[6] || (revision != "" && revision != parts[5]) {
			return nil, ErrAIExternalRuntime
		}
		revision = parts[5]
		seen[v.Target] = true
	}
	return json.RawMessage(raw), nil
}
func (r aiExternalRelease) matchesMounts(actual []aiExternalMount) bool {
	want := map[string]aiExternalReleaseVolume{}
	for _, v := range r.mounts {
		want[v.Target] = v
	}
	// Compose merges unique mount targets from base compose.yaml and runtime.json;
	// the MQ rewrite does not remove the base TLS mounts in the final container.
	base := map[string]string{"/run/qs-ai-tls/ca-chain.crt": "/data/infra/ssl/grpc/ca/ca-chain.crt", "/run/qs-ai-tls/qs-ai-fullchain.crt": "/data/infra/ssl/grpc/server/qs-ai-fullchain.crt", "/run/qs-ai-tls/qs-ai.key": "/data/infra/ssl/grpc/server/qs-ai.key"}
	seen := map[string]bool{}
	for _, v := range actual {
		if v.Destination == "/tmp" && v.Type == "tmpfs" {
			continue
		}
		if seen[v.Destination] {
			return false
		}
		seen[v.Destination] = true
		if source, ok := base[v.Destination]; ok {
			if v.Type != "bind" || v.RW || v.Source != source {
				return false
			}
			continue
		}
		w, ok := want[v.Destination]
		if !ok || v.Type != "bind" || v.RW || v.Source != w.Source {
			return false
		}
	}
	return len(seen) == len(want)+len(base)
}
func aiExternalReleaseFile(path string, optional bool) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrAIExternalRuntime
	}
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		s, e := os.Lstat(directory)
		if e != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 || s.Mode().Perm()&0022 != 0 {
			return nil, ErrAIExternalRuntime
		}
		uid, ok := s.Sys().(*syscall.Stat_t)
		if !ok || uid.Uid != 0 && int(uid.Uid) != os.Geteuid() {
			return nil, ErrAIExternalRuntime
		}
		if directory == "/" {
			break
		}
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		if optional && errors.Is(e, syscall.ENOENT) {
			return nil, nil
		}
		return nil, ErrAIExternalRuntime
	}
	f := os.NewFile(uintptr(fd), "private_release_file")
	before, e := f.Stat()
	if e != nil {
		_ = f.Close()
		return nil, ErrAIExternalRuntime
	} // Rejected regardless of cleanup result.
	u, ok := before.Sys().(*syscall.Stat_t)
	if !ok || !before.Mode().IsRegular() || before.Size() > 1<<20 || before.Mode().Perm()&0022 != 0 || u.Nlink != 1 || u.Uid != 0 && int(u.Uid) != os.Geteuid() {
		_ = f.Close()
		return nil, ErrAIExternalRuntime
	} // Rejected regardless of cleanup result.
	raw, read := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	after, check := f.Stat()
	visible, named := os.Lstat(path)
	closed := f.Close()
	if read != nil || check != nil || named != nil || closed != nil || !os.SameFile(before, after) || !os.SameFile(before, visible) || before.Size() != after.Size() || before.Size() != int64(len(raw)) || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() || before.Mode() != visible.Mode() {
		return nil, ErrAIExternalRuntime
	}
	a, ok := after.Sys().(*syscall.Stat_t)
	v, visibleOK := visible.Sys().(*syscall.Stat_t)
	if !ok || !visibleOK || a.Uid != u.Uid || a.Gid != u.Gid || a.Nlink != 1 || v.Uid != u.Uid || v.Gid != u.Gid || v.Nlink != 1 {
		return nil, ErrAIExternalRuntime
	}
	return raw, nil
}
func aiExternalReadRelease(source, image string) (aiExternalRelease, error) {
	var result aiExternalRelease
	state, e := aiExternalReleaseFile("/opt/qs-ai/state.json", false)
	if e != nil || strictJSON(state) != nil {
		return result, ErrAIExternalRuntime
	}
	var stateValue struct {
		Current  string `json:"current"`
		Previous string `json:"previous"`
	}
	decoder := json.NewDecoder(bytes.NewReader(state))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&stateValue) != nil || !regexp.MustCompile(`^[0-9a-f]{40}-[0-9]{1,20}-[0-9]{1,4}$`).MatchString(stateValue.Current) || !strings.HasPrefix(stateValue.Current, source+"-") {
		return result, ErrAIExternalRuntime
	}
	base := "/opt/qs-ai/releases/" + stateValue.Current
	files := map[string]string{"state.json": sourceSHA(state)}
	contents := map[string][]byte{}
	for _, name := range []string{"manifest.json", "runtime.json", "compose.yaml", "loaded-image.json", "image.env"} {
		raw, err := aiExternalReleaseFile(base+"/"+name, name == "loaded-image.json")
		if err != nil {
			return result, err
		}
		contents[name] = raw
		if raw != nil {
			files[name] = sourceSHA(raw)
		} else {
			files[name] = ""
		}
	}
	var manifest struct {
		Revision  string  `json:"revision"`
		ImageID   string  `json:"image_id"`
		Messaging *string `json:"messaging_binding_sha256"`
	}
	if strictJSON(contents["manifest.json"]) != nil || json.Unmarshal(contents["manifest.json"], &manifest) != nil || manifest.Revision != source || !aiExternalImageID(manifest.ImageID) {
		return result, ErrAIExternalRuntime
	}
	expected := manifest.ImageID
	if loaded := contents["loaded-image.json"]; loaded != nil {
		var v struct {
			Revision string `json:"revision"`
			Source   string `json:"source_image_id"`
			Loaded   string `json:"loaded_image_id"`
		}
		if strictJSON(loaded) != nil || json.Unmarshal(loaded, &v) != nil || v.Revision != source || v.Source != manifest.ImageID || !aiExternalImageID(v.Loaded) {
			return result, ErrAIExternalRuntime
		}
		expected = v.Loaded
	}
	imageEnvironment := "QS_AI_IMAGE=" + image + "\n"
	if expected != image || (string(contents["image.env"]) != imageEnvironment && (contents["loaded-image.json"] != nil || string(contents["image.env"]) != "QS_AI_IMAGE=qs-ai:"+source+"\n")) {
		return result, ErrAIExternalRuntime
	}
	var runtime struct {
		Services map[string]struct {
			Environment map[string]json.RawMessage `json:"environment"`
			Volumes     []aiExternalReleaseVolume  `json:"volumes"`
		} `json:"services"`
	}
	if strictJSON(contents["runtime.json"]) != nil || json.Unmarshal(contents["runtime.json"], &runtime) != nil || len(runtime.Services) != 1 {
		return result, ErrAIExternalRuntime
	}
	service, ok := runtime.Services["qs-ai"]
	if !ok {
		return result, ErrAIExternalRuntime
	}
	result.messaging = json.RawMessage("null")
	if binding, ok := service.Environment["QS_AI_MESSAGING"]; ok {
		var raw string
		if json.Unmarshal(binding, &raw) != nil {
			return result, ErrAIExternalRuntime
		}
		result.messaging, e = aiExternalReleaseMounts(raw, service.Volumes)
		if e != nil {
			return result, e
		}
		if manifest.Messaging == nil || *manifest.Messaging != aiJSONHash(map[string]any{"options": raw, "volumes": service.Volumes}) {
			return result, ErrAIExternalRuntime
		}
		result.mounts = service.Volumes
	} else if manifest.Messaging != nil || len(service.Volumes) != 0 {
		return result, ErrAIExternalRuntime
	}
	result.seal = aiJSONHash(struct {
		Protocol, Release, SourceSHA, ImageID string
		Files                                 map[string]string
	}{"actual-qs-ai-frozen-runtime-binding/v1", stateValue.Current, source, image, files})
	return result, nil
}

// A hash observation for independent approval, never a qualification or write
// permit. It reads only the fixed active release and before/after container.
func ObserveAIExternalRuntimeBinding(ctx context.Context, source, image, cid string, sudo bool) (string, error) {
	if ctx == nil || !aiOriginalSourceSHA(source) || !aiExternalImageID(image) || !evidenceHash(cid) {
		return "", ErrAIExternalInput
	}
	work, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	d, err := aiExternalDocker(sudo)
	if err != nil {
		return "", err
	}
	in := AIExternalExecutionInput{RuntimeSourceSHA: source, ImageID: image, ContainerID: cid}
	before, err := d.inspect(work, cid)
	if err != nil || !before.matches(in) {
		return "", ErrAIExternalRuntime
	}
	r, err := aiExternalReadRelease(source, image)
	if err != nil || !r.matchesMounts(before.Mounts) {
		return "", ErrAIExternalRuntime
	}
	after, err := d.inspect(work, cid)
	if err != nil || !reflect.DeepEqual(before, after) {
		return "", ErrAIExternalRuntime
	}
	recheck, err := aiExternalReadRelease(source, image)
	if err != nil || !reflect.DeepEqual(r, recheck) {
		return "", ErrAIExternalRuntime
	}
	return r.seal, nil
}

// Keep fmt from rendering sensitive input internals even in caller diagnostics.
var _ fmt.Stringer = AIExternalExecutionInput{}
