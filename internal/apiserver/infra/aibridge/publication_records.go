package aibridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/protobuf/proto"
)

type publicationAssetRef struct {
	Identity      string `json:"identity"`
	Version       string `json:"version"`
	Fingerprint   string `json:"fingerprint"`
	ContentSHA256 string `json:"content_sha256"`
}
type publicationAudit struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	At     string `json:"at"`
}
type publicationProof struct {
	PublicationID string           `json:"publication_id"`
	Audit         publicationAudit `json:"audit"`
	Evidence      struct {
		RunID      string `json:"run_id"`
		RunVersion int64  `json:"run_version"`
		Profile    struct {
			ProfileID      string `json:"profile_id"`
			Version        string `json:"version"`
			Fingerprint    string `json:"fingerprint"`
			DefinitionJSON string `json:"definition_json"`
		} `json:"profile"`
		Manifest                     map[string]publicationAssetRef     `json:"manifest"`
		Release                      map[string]app.FrozenEvaluationRef `json:"release"`
		EvaluatedManifestFingerprint string                             `json:"evaluated_manifest_fingerprint"`
		FinalReview                  struct {
			Actor       string `json:"actor"`
			Reason      string `json:"reason"`
			FinalizedAt string `json:"finalized_at"`
			Passed      *bool  `json:"passed"`
		} `json:"final_review"`
	} `json:"evidence"`
}

func digest(raw []byte) string {
	hash := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}
func releaseDigest(release map[string]app.FrozenEvaluationRef) string {
	raw, _ := json.Marshal(release)
	return digest(raw)
}
func validPublicationAudit(actor, reason, at string) bool {
	id, err := strconv.ParseInt(strings.TrimPrefix(actor, "user:"), 10, 64)
	_, dateErr := time.Parse(time.RFC3339Nano, at)
	return err == nil && id > 0 && actor == fmt.Sprintf("user:%d", id) && strings.TrimSpace(reason) != "" && len(reason) <= 1000 && !strings.ContainsAny(reason, "<>") && dateErr == nil
}

// QS verifies wire identity/checksum bindings. AI remains responsible for G1-G5 and asset authority.
func publicationDocument(state app.PublicationState) (publicationProof, error) {
	var document struct {
		SchemaVersion string           `json:"schema_version"`
		Publication   publicationProof `json:"publication"`
	}
	decoder := json.NewDecoder(bytes.NewReader(state.Publication))
	decoder.DisallowUnknownFields()
	if len(state.Publication) > 256*1024 || !utf8.Valid(state.Publication) || !json.Valid(state.Publication) || decoder.Decode(&document) != nil || document.SchemaVersion != "qs-ai-publication/v1" {
		return publicationProof{}, app.ErrConflict
	}
	p := document.Publication
	e := p.Evidence
	if p.PublicationID != state.ActivePublicationID || !app.ValidPublicationID(p.PublicationID) || !app.ValidPublicationID(e.RunID) || e.RunVersion < 1 ||
		e.FinalReview.Passed == nil || !*e.FinalReview.Passed || !validPublicationAudit(p.Audit.Actor, p.Audit.Reason, p.Audit.At) || !validPublicationAudit(e.FinalReview.Actor, e.FinalReview.Reason, e.FinalReview.FinalizedAt) {
		return publicationProof{}, app.ErrConflict
	}
	approved, _ := time.Parse(time.RFC3339Nano, e.FinalReview.FinalizedAt)
	published, _ := time.Parse(time.RFC3339Nano, p.Audit.At)
	changed, err := time.Parse(time.RFC3339Nano, state.ChangedAt)
	if err != nil || published.Before(approved) || changed.Before(published) || len(e.Manifest) != 5 || len(e.Release) != 11 {
		return publicationProof{}, app.ErrConflict
	}
	for _, name := range []string{"suite", "profile", "prompt", "input_schema", "output_schema", "generation_route", "semantic_prompt", "semantic_output_schema", "semantic_route", "execution_policy", "gate_policy"} {
		ref, ok := e.Release[name]
		if !ok || strings.TrimSpace(ref.ID) == "" || ref.Version == "" || !gateReleaseFingerprint.MatchString(ref.Fingerprint) {
			return publicationProof{}, app.ErrConflict
		}
	}
	canonical := map[string]any{"schema_version": "qs-ai-generation-manifest/v1"}
	for _, name := range []string{"profile", "prompt", "generation_route", "input_schema", "output_schema"} {
		ref, ok := e.Manifest[name]
		frozen := e.Release[name]
		hash, err := hex.DecodeString(ref.ContentSHA256)
		version := ref.Version
		if strings.HasSuffix(name, "schema") {
			version = ref.Identity + "/" + version
		}
		if !ok || ref.Identity != frozen.ID || version != frozen.Version || ref.Fingerprint != frozen.Fingerprint || err != nil || len(hash) != 32 || hex.EncodeToString(hash) != ref.ContentSHA256 {
			return publicationProof{}, app.ErrConflict
		}
		canonical[name] = map[string]string{"identity": ref.Identity, "version": ref.Version, "fingerprint": ref.Fingerprint, "content_sha256": ref.ContentSHA256}
	}
	raw, _ := json.Marshal(canonical)
	if digest(raw) != e.EvaluatedManifestFingerprint {
		return publicationProof{}, app.ErrConflict
	}
	profile := e.Profile
	ref := e.Manifest["profile"]
	if profile.ProfileID != ref.Identity || profile.Version != ref.Version || profile.Fingerprint != ref.Fingerprint || digest([]byte(profile.DefinitionJSON)) != profile.Fingerprint || "sha256:"+ref.ContentSHA256 != profile.Fingerprint {
		return publicationProof{}, app.ErrConflict
	}
	var definition struct {
		ProfileID string                  `json:"profile_id"`
		Version   string                  `json:"version"`
		Selector  app.PublicationSelector `json:"selector"`
	}
	if json.Unmarshal([]byte(profile.DefinitionJSON), &definition) != nil || definition.ProfileID != profile.ProfileID || definition.Version != profile.Version || !definition.Selector.Equal(state.Selector) {
		return publicationProof{}, app.ErrConflict
	}
	return p, nil
}
func publicationState(response *pb.PublicationState) (app.PublicationState, error) {
	if response == nil || response.Selector == nil || proto.Size(response) > 512*1024 {
		return app.PublicationState{}, app.ErrConflict
	}
	r := response.Selector
	result := app.PublicationState{Selector: app.PublicationSelector{Audience: r.Audience, ModelKind: r.ModelKind, DecisionKind: r.DecisionKind, ModelCode: r.ModelCode, ModelVersion: r.ModelVersion}, Version: response.Version, ActivePublicationID: response.ActivePublicationId, ChangedAt: response.ChangedAt}
	if !result.Selector.Valid() || result.Version < 0 {
		return app.PublicationState{}, app.ErrConflict
	}
	if result.Version == 0 {
		if result.ActivePublicationID != "" || response.PublicationJson != "" || result.ChangedAt != "" {
			return app.PublicationState{}, app.ErrConflict
		}
		return result, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, result.ChangedAt); err != nil {
		return app.PublicationState{}, app.ErrConflict
	}
	if result.ActivePublicationID == "" {
		if response.PublicationJson != "" {
			return app.PublicationState{}, app.ErrConflict
		}
		return result, nil
	}
	result.Publication = json.RawMessage(response.PublicationJson)
	if _, err := publicationDocument(result); err != nil {
		return app.PublicationState{}, err
	}
	return result, nil
}
func publicationReceipt(response *pb.PublicationReceipt, scope app.PublicationScope, commandID string) (app.PublicationReceipt, error) {
	if response == nil || proto.Size(response) > 1024*1024 || response.CommandId != commandID || !app.ValidPublicationID(commandID) || response.Actor != fmt.Sprintf("user:%d", scope.OperatorUserID) || !validPublicationAudit(response.Actor, response.Reason, response.ChangedAt) {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	previous, err := publicationState(response.Previous)
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	current, err := publicationState(response.Current)
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	if !previous.Selector.Equal(current.Selector) || current.Version != previous.Version+1 || current.ChangedAt != response.ChangedAt {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	at, _ := time.Parse(time.RFC3339Nano, response.ChangedAt)
	if previous.ChangedAt != "" {
		before, _ := time.Parse(time.RFC3339Nano, previous.ChangedAt)
		if at.Before(before) {
			return app.PublicationReceipt{}, app.ErrConflict
		}
	}
	switch response.Action {
	case "disable":
		if previous.ActivePublicationID == "" || current.ActivePublicationID != "" {
			return app.PublicationReceipt{}, app.ErrConflict
		}
	case "publish", "rollback":
		if current.ActivePublicationID == "" || current.ActivePublicationID == previous.ActivePublicationID {
			return app.PublicationReceipt{}, app.ErrConflict
		}
		proof, err := publicationDocument(current)
		if err != nil {
			return app.PublicationReceipt{}, err
		}
		if response.Action == "publish" && (proof.Audit.Actor != response.Actor || proof.Audit.Reason != response.Reason || proof.Audit.At != response.ChangedAt) {
			return app.PublicationReceipt{}, app.ErrConflict
		}
		if response.Action == "rollback" && previous.Version == 0 {
			return app.PublicationReceipt{}, app.ErrConflict
		}
	default:
		return app.PublicationReceipt{}, app.ErrConflict
	}
	return app.PublicationReceipt{CommandID: commandID, Previous: previous, Current: current, Action: response.Action, Actor: response.Actor, Reason: response.Reason, ChangedAt: response.ChangedAt}, nil
}
