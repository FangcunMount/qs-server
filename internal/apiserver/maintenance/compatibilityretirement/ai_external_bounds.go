package retirement

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const ErrAIExternalBounds SourceError = "ai_external_bounds_discovery_rejected"

// This input constrains a diagnostic discovery. Prior AI identity/head may be
// explicitly unknown, in which case the result cannot report a matched prior
// binding. The peer identity/head instead come from the actual borrowed local
// resolver. There is no executable, AI URL, table name or approval flag input.
type AIExternalBoundsInput struct {
	// Host-provided original operation directory, unchanged across attempts.
	OperationDirectory                     string
	AssetsDirectory, RunID                 string
	RuntimeSourceSHA, ImageID, ContainerID string
	ApprovedAIRuntimeBindingSHA256         string
	ExpectedAIIdentityHash, ExpectedAIHead string
	PeerConnection                         AIExternalPeerConnection
	SudoDocker                             bool
}

func (AIExternalBoundsInput) String() string               { return "private external bounds discovery inputs" }
func (v AIExternalBoundsInput) GoString() string           { return v.String() }
func (AIExternalBoundsInput) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }

type aiExternalPriorBinding struct {
	Identity string `json:"identity_hash"`
	Head     string `json:"head"`
}
type aiExternalDiscoveredPacket struct {
	Bytes        string          `json:"bytes"`
	SHA          string          `json:"sha256"`
	Objects      int             `json:"objects"`
	PriorMatched bool            `json:"prior_binding_matched"`
	AfterUpper   map[string]bool `json:"after_upper"`
}
type aiExternalDiscoveryFacts struct {
	Protocol            string                                `json:"protocol"`
	SourceSHA           string                                `json:"source_sha"`
	OperationID         string                                `json:"operation_id"`
	RunID               string                                `json:"run_id"`
	RuntimeSourceSHA    string                                `json:"runtime_source_sha"`
	RuntimeBindingSHA   string                                `json:"runtime_binding_sha256"`
	ImageID             string                                `json:"image_id"`
	ContainerID         string                                `json:"container_id"`
	Bounds              map[string]aiExternalDiscoveredPacket `json:"bounds"`
	Sections            map[string]aiExternalSection          `json:"original_sections"`
	LogicalObjects      int                                   `json:"ai_logical_objects"`
	Epochs              int                                   `json:"independent_epochs"`
	Scope               string                                `json:"scope"`
	IndependentApproval bool                                  `json:"independent_approval"`
	BusinessClosure     bool                                  `json:"business_closure"`
	Fence               bool                                  `json:"fence"`
	CASAuthority        bool                                  `json:"cas_authority"`
	DropReady           bool                                  `json:"drop_ready"`
}

// Only the actual fixed execution factory below creates this object. It is
// intentionally not an AIExternalExecutionQualification and has no retirement,
// storage, CAS or page-qualification method. Packets require NEW independent
// approval before PrepareAIExternalExecution will accept their bytes/hashes.
type AIExternalBoundsObservation struct {
	self     *AIExternalBoundsObservation
	facts    aiExternalDiscoveryFacts
	ai, peer []byte
	seal     string
}
type AIExternalBoundsSummary struct {
	Scope                 string `json:"scope"`
	SourceSHA256          string `json:"facts_sha256"`
	RuntimeBindingSHA256  string `json:"runtime_binding_sha256"`
	AIBoundsSHA256        string `json:"ai_bounds_sha256"`
	PeerBoundsSHA256      string `json:"peer_bounds_sha256"`
	AIPhysicalObjects     int    `json:"ai_physical_objects"`
	AILogicalObjects      int    `json:"ai_logical_objects"`
	PeerObjects           int    `json:"peer_objects"`
	IndependentEpochs     int    `json:"independent_epochs"`
	PriorAIBindingMatched bool   `json:"prior_ai_binding_matched"`
	NextCycleRequired     bool   `json:"next_cycle_required"`
	IndependentApproval   bool   `json:"independent_approval"`
	BusinessClosure       bool   `json:"business_closure"`
	WriterFence           bool   `json:"writer_fence"`
	BrokerCoverage        bool   `json:"broker_coverage"`
	CASAuthority          bool   `json:"cas_authority"`
	RetirementWritten     bool   `json:"retirement_written"`
	RecoveryAuthority     bool   `json:"recovery_authority"`
	DropReady             bool   `json:"drop_ready"`
}

func (*AIExternalBoundsObservation) String() string {
	return "private unapproved external bounds observation; no authority"
}
func (o *AIExternalBoundsObservation) GoString() string           { return o.String() }
func (*AIExternalBoundsObservation) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIExternalBoundsObservation) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (o *AIExternalBoundsObservation) valid() bool {
	return o != nil && o.self == o && o.seal == aiJSONHash(o.facts) && sourceSHA(o.ai) == o.facts.Bounds["ai"].SHA && sourceSHA(o.peer) == o.facts.Bounds["peer"].SHA
}
func (o *AIExternalBoundsObservation) Summary() AIExternalBoundsSummary {
	if !o.valid() {
		return AIExternalBoundsSummary{}
	}
	ai, peer := o.facts.Bounds["ai"], o.facts.Bounds["peer"]
	next := false
	for _, side := range o.facts.Bounds {
		for _, present := range side.AfterUpper {
			next = next || present
		}
	}
	return AIExternalBoundsSummary{Scope: o.facts.Scope, SourceSHA256: o.seal, RuntimeBindingSHA256: o.facts.RuntimeBindingSHA, AIBoundsSHA256: ai.SHA, PeerBoundsSHA256: peer.SHA, AIPhysicalObjects: ai.Objects, AILogicalObjects: o.facts.LogicalObjects, PeerObjects: peer.Objects, IndependentEpochs: o.facts.Epochs, PriorAIBindingMatched: ai.PriorMatched, NextCycleRequired: next}
}

// These copies contain private PK upper tokens/schema metadata. The host owns
// fresh O_EXCL/0600 packet registration, durability and the subsequent separate
// approval. They must not be copied to stdout, workflow logs or public receipts.
func (o *AIExternalBoundsObservation) PrivatePackets() ([]byte, []byte, error) {
	if !o.valid() {
		return nil, nil, ErrAIExternalBounds
	}
	return append([]byte(nil), o.ai...), append([]byte(nil), o.peer...), nil
}

func aiExternalDiscoveryInputValid(in AIExternalBoundsInput) bool {
	if !aiOriginalSourceSHA(in.RuntimeSourceSHA) || !aiExternalImageID(in.ImageID) || !evidenceHash(in.ContainerID) || !aiExternalRunID(in.RunID) || !evidenceHash(in.ApprovedAIRuntimeBindingSHA256) {
		return false
	}
	if in.ExpectedAIIdentityHash == "" && in.ExpectedAIHead == "" {
		return true
	}
	return evidenceHash(in.ExpectedAIIdentityHash) && (in.ExpectedAIHead == "0038_messaging_observations" || in.ExpectedAIHead == "0040_module_table_names")
}

func (c *HistoricalCoordinator) ObserveAIExternalBounds(ctx context.Context, local *AIReadOnlyResolver, in AIExternalBoundsInput) (*AIExternalBoundsObservation, error) {
	if c == nil || ctx == nil {
		return nil, ErrAIExternalBounds
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now == nil || c.alive(ctx) != nil || c.authenticated == nil || !c.authenticated.complete || local == nil || local.owner != c || local.validate(ctx) != nil || !aiExternalDiscoveryInputValid(in) {
		return nil, ErrAIExternalBounds
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
		e := c.copies[index].Expected
		sections[e.Boundary.Name] = aiExternalSection{e.Records, e.Bytes, e.DataHash}
	}
	if len(sections) != 2 || sections[AIBridgeCommandSource].Rows+sections[AILegacyCommandSource].Rows > 10000 {
		return nil, ErrAIExternalBounds
	}
	peerBinding := aiExternalPriorBinding{local.inner.binding.DatabaseIdentityHash, strconv.FormatUint(local.inner.binding.MigrationVersion, 10)}
	packet := struct {
		Protocol       string                       `json:"protocol"`
		SourceSHA      string                       `json:"source_sha"`
		OperationID    string                       `json:"operation_id"`
		RunID          string                       `json:"run_id"`
		RuntimeSHA     string                       `json:"runtime_source_sha"`
		RuntimeBinding string                       `json:"runtime_binding_sha256"`
		Messaging      json.RawMessage              `json:"runtime_messaging"`
		Image          string                       `json:"image_id"`
		Container      string                       `json:"container_id"`
		Sections       map[string]aiExternalSection `json:"original_sections"`
		Peer           aiExternalPeerConnectionWire `json:"peer_connection"`
		ExpectedAI     aiExternalPriorBinding       `json:"expected_ai"`
		ExpectedPeer   aiExternalPriorBinding       `json:"expected_peer"`
		Modules        map[string]string            `json:"modules"`
	}{"qs-ai-readonly-bounds-discovery-input/v1", c.binding.SourceSHA, c.binding.OperationID, in.RunID, in.RuntimeSourceSHA, release.seal, release.messaging, in.ImageID, in.ContainerID, sections, aiExternalPeerConnectionWire(in.PeerConnection), aiExternalPriorBinding{in.ExpectedAIIdentityHash, in.ExpectedAIHead}, peerBinding, assets.modules}
	raw, err := json.Marshal(packet)
	if err != nil || len(raw) > aiExternalInputLimit {
		return nil, ErrAIExternalBounds
	}
	deadline := c.started.Add(c.limits.MaxDuration)
	if d := time.Now().Add(1520 * time.Second); d.Before(deadline) {
		deadline = d
	}
	work, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	docker, err := aiExternalDocker(in.SudoDocker)
	if err != nil {
		return nil, err
	}
	runtimeInput := AIExternalExecutionInput{RuntimeSourceSHA: in.RuntimeSourceSHA, ImageID: in.ImageID, ContainerID: in.ContainerID}
	before, err := docker.inspect(work, in.ContainerID)
	if err != nil || !before.matches(runtimeInput) || !release.matchesMounts(before.Mounts) {
		return nil, ErrAIExternalRuntime
	}
	result, err := aiExternalExecuteMode(work, docker, in.OperationDirectory, aiExternalBoundsMode, c.binding, in.RunID, in.RuntimeSourceSHA, in.ImageID, in.ContainerID, assets.host, raw)
	if err != nil {
		return nil, ErrAIExternalBounds
	}
	after, err := docker.inspect(work, in.ContainerID)
	if err != nil || !reflect.DeepEqual(before, after) || !after.matches(runtimeInput) || !release.matchesMounts(after.Mounts) {
		return nil, ErrAIExternalRuntime
	}
	repeated, err := aiExternalReadRelease(in.RuntimeSourceSHA, in.ImageID)
	if err != nil || repeated.seal != release.seal || !reflect.DeepEqual(repeated, release) {
		return nil, ErrAIExternalRuntime
	}
	facts, ai, peer, err := aiExternalDecodeDiscovery(result)
	if err != nil || facts.SourceSHA != c.binding.SourceSHA || facts.OperationID != c.binding.OperationID || facts.RunID != in.RunID || facts.RuntimeSourceSHA != in.RuntimeSourceSHA || facts.RuntimeBindingSHA != release.seal || facts.ImageID != in.ImageID || facts.ContainerID != in.ContainerID || !reflect.DeepEqual(facts.Sections, sections) {
		return nil, ErrAIExternalBounds
	}
	var peerPacket struct {
		Identity string `json:"identity_hash"`
		Head     string `json:"head"`
		Source   string `json:"source_sha"`
	}
	if json.Unmarshal(peer, &peerPacket) != nil || peerPacket.Identity != peerBinding.Identity || peerPacket.Head != peerBinding.Head || peerPacket.Source != c.binding.SourceSHA {
		return nil, ErrAIExternalChanged
	}
	var aiPacket struct {
		Identity string `json:"identity_hash"`
		Head     string `json:"head"`
	}
	if json.Unmarshal(ai, &aiPacket) != nil || aiPacket.Identity == peerBinding.Identity || facts.Bounds["ai"].PriorMatched != (in.ExpectedAIIdentityHash != "") || in.ExpectedAIIdentityHash != "" && (aiPacket.Identity != in.ExpectedAIIdentityHash || aiPacket.Head != in.ExpectedAIHead) {
		return nil, ErrAIExternalChanged
	}
	if local.validate(ctx) != nil || c.alive(ctx) != nil {
		return nil, ErrAIExternalChanged
	}
	o := &AIExternalBoundsObservation{facts: facts, ai: ai, peer: peer}
	o.self = o
	o.seal = aiJSONHash(facts)
	return o, nil
}

func aiExternalDecodeDiscovery(raw []byte) (aiExternalDiscoveryFacts, []byte, []byte, error) {
	var f aiExternalDiscoveryFacts
	if len(raw) == 0 || len(raw) > aiExternalResultLimit || strictJSON(raw) != nil {
		return f, nil, nil, ErrAIExternalBounds
	}
	fields, err := aiExternalDiscoveryFields(raw, "protocol source_sha operation_id run_id runtime_source_sha runtime_binding_sha256 image_id container_id bounds original_sections ai_logical_objects independent_epochs scope independent_approval business_closure fence cas_authority drop_ready")
	if err != nil {
		return f, nil, nil, err
	}
	for _, key := range []string{"independent_approval", "business_closure", "fence", "cas_authority", "drop_ready"} {
		if string(fields[key]) != "false" {
			return f, nil, nil, ErrAIExternalBounds
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&f) != nil || f.Protocol != "qs-ai-readonly-bounds-discovery-facts/v1" || f.Scope != "diagnostic-unapproved-bounds-only" || f.Epochs != 2 || f.IndependentApproval || f.BusinessClosure || f.Fence || f.CASAuthority || f.DropReady || len(f.Bounds) != 2 || len(f.Sections) != 2 || f.LogicalObjects != 53 {
		return f, nil, nil, ErrAIExternalBounds
	}
	if !aiOriginalSourceSHA(f.SourceSHA) || !aiOriginalSourceSHA(f.RuntimeSourceSHA) || !aiLocalOperationID(f.OperationID) || !aiExternalRunID(f.RunID) || !evidenceHash(f.RuntimeBindingSHA) || !aiExternalImageID(f.ImageID) || !evidenceHash(f.ContainerID) {
		return f, nil, nil, ErrAIExternalBounds
	}
	var wireBounds map[string]json.RawMessage
	if json.Unmarshal(fields["bounds"], &wireBounds) != nil {
		return f, nil, nil, ErrAIExternalBounds
	}
	var copies [2][]byte
	for i, side := range []string{"ai", "peer"} {
		wire, err := aiExternalDiscoveryFields(wireBounds[side], "bytes sha256 objects prior_binding_matched after_upper")
		if err != nil {
			return f, nil, nil, err
		}
		if string(wire["prior_binding_matched"]) != "false" && string(wire["prior_binding_matched"]) != "true" {
			return f, nil, nil, ErrAIExternalBounds
		}
		var afterWire map[string]json.RawMessage
		if json.Unmarshal(wire["after_upper"], &afterWire) != nil {
			return f, nil, nil, ErrAIExternalBounds
		}
		for _, value := range afterWire {
			if string(value) != "false" && string(value) != "true" {
				return f, nil, nil, ErrAIExternalBounds
			}
		}
		packet, ok := f.Bounds[side]
		if !ok || !evidenceHash(packet.SHA) || packet.Objects <= 0 || packet.Objects > 64 || len(packet.AfterUpper) != packet.Objects || side == "peer" && (packet.Objects != 14 || !packet.PriorMatched) {
			return f, nil, nil, ErrAIExternalBounds
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(packet.Bytes)
		if err != nil || len(decoded) > 4<<20 || sourceSHA(decoded) != packet.SHA || strictJSON(decoded) != nil {
			return f, nil, nil, ErrAIExternalBounds
		}
		if _, err = aiExternalDiscoveryFields(decoded, "protocol side source_sha identity_hash head catalog_sha256 tables profile"); err != nil {
			return f, nil, nil, err
		}
		var b struct {
			Protocol string                     `json:"protocol"`
			Side     string                     `json:"side"`
			Source   string                     `json:"source_sha"`
			Identity string                     `json:"identity_hash"`
			Head     string                     `json:"head"`
			Catalog  string                     `json:"catalog_sha256"`
			Tables   map[string]json.RawMessage `json:"tables"`
			Profile  []int64                    `json:"profile"`
		}
		d := json.NewDecoder(bytes.NewReader(decoded))
		d.DisallowUnknownFields()
		if d.Decode(&b) != nil || b.Protocol != "qs-ai-full-ledger-bounds/v1" || b.Side != side || !aiOriginalSourceSHA(b.Source) || !evidenceHash(b.Identity) || !evidenceHash(b.Catalog) || len(b.Tables) != packet.Objects || !reflect.DeepEqual(b.Profile, []int64{1000, 1000000, 2 << 30, 256 << 20, 32 << 20, 30, 1500}) {
			return f, nil, nil, ErrAIExternalBounds
		}
		if side == "ai" && (b.Head != "0040_module_table_names" && b.Head != "0038_messaging_observations" || b.Head == "0040_module_table_names" && (packet.Objects != 44 || b.Source != "82ffa1b43308f23fbb1ebe669c3071e0486e105a") || b.Head == "0038_messaging_observations" && packet.Objects != 53) {
			return f, nil, nil, ErrAIExternalBounds
		}
		if side == "peer" {
			version, e := strconv.ParseUint(b.Head, 10, 64)
			if e != nil || len(b.Head) > 9 || version == 0 || strconv.FormatUint(version, 10) != b.Head || b.Source != f.SourceSHA {
				return f, nil, nil, ErrAIExternalBounds
			}
		}
		for table := range b.Tables {
			if _, ok := packet.AfterUpper[table]; !ok {
				return f, nil, nil, ErrAIExternalBounds
			}
		}
		copies[i] = decoded
	}
	var sectionWire map[string]json.RawMessage
	if json.Unmarshal(fields["original_sections"], &sectionWire) != nil {
		return f, nil, nil, ErrAIExternalBounds
	}
	for _, name := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		if _, err = aiExternalDiscoveryFields(sectionWire[name], "rows source_bytes source_sha256"); err != nil {
			return f, nil, nil, err
		}
		s, ok := f.Sections[name]
		if !ok || !evidenceHash(s.SHA) || s.Rows > 10000 {
			return f, nil, nil, ErrAIExternalBounds
		}
	}
	return f, copies[0], copies[1], nil
}

// Exact grammar for a just-executed producer response, never a public import or
// authority constructor. Missing/null flags cannot be interpreted as false.
func aiExternalDiscoveryFields(raw []byte, keys string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(strings.Fields(keys)) {
		return nil, ErrAIExternalBounds
	}
	for _, key := range strings.Fields(keys) {
		value, ok := fields[key]
		if !ok || len(value) == 0 || bytes.Equal(value, []byte("null")) {
			return nil, ErrAIExternalBounds
		}
	}
	return fields, nil
}
