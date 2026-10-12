package aibridge

import (
	"regexp"
	"strconv"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
)

// CommandRetirementSource keeps a physical source-bytes digest separate from the
// historical business JSON hash. MySQL JSON CAST bytes are not the original JSON
// encoding, and neither digest is a standard MQ body_sha256.
type CommandRetirementSource struct {
	Table               string `json:"table"`
	CommandID           string `json:"command_id"`
	BytesKind           string `json:"bytes_kind"`
	BytesSHA256         string `json:"bytes_sha256"`
	BusinessPayloadHash string `json:"business_payload_hash"`
}

type CommandRetirementReference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// CommandRetirementEvidence is a minimal historical conclusion, never an MQ
// receipt or permission to execute. It preserves source identity and the independently verified historical
// ownership/responsibility conclusion for current readers and retired-ID guards.
type CommandRetirementEvidence struct {
	Version              int                          `json:"version"`
	OperationID          string                       `json:"operation_id"`
	VerifierVersion      string                       `json:"verifier_version"`
	VerificationMethod   string                       `json:"verification_method"`
	VerifiedAt           time.Time                    `json:"verified_at"`
	AdmissionRevision    uint64                       `json:"admission_revision"`
	CommandID            string                       `json:"command_id"`
	RequestID            string                       `json:"request_id"`
	SourceKind           string                       `json:"source_kind"`
	OrganizationID       string                       `json:"organization_id"`
	SubjectID            string                       `json:"subject_id"`
	ResourceID           string                       `json:"resource_id"`
	LiveBodySHA256       string                       `json:"live_body_sha256,omitempty"`
	Sources              []CommandRetirementSource    `json:"sources"`
	References           []CommandRetirementReference `json:"references"`
	Conclusion           string                       `json:"conclusion"`
	Reason               string                       `json:"reason"`
	OwnershipVerified    bool                         `json:"ownership_verified"`
	ResponsibilityClosed bool                         `json:"responsibility_closed"`
	BusinessTerminal     bool                         `json:"business_terminal"`
}

var retirementToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@#-]*$`)

var retirementOperationID = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)

func validRetirementID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}

func (e CommandRetirementEvidence) validate(transferred bool) error {
	org, err := strconv.ParseUint(e.OrganizationID, 10, 64)
	_, offset := e.VerifiedAt.Zone()
	if e.Version != 1 || !retirementOperationID.MatchString(e.OperationID) || !validRetirementID(e.CommandID) || !validRetirementID(e.RequestID) || !validRetirementID(e.ResourceID) || err != nil || org == 0 || strconv.FormatUint(org, 10) != e.OrganizationID || e.SubjectID == "" || len(e.SubjectID) > 128 || len(e.VerifierVersion) > 128 || !retirementToken.MatchString(e.VerifierVersion) || e.VerifiedAt.IsZero() || offset != 0 || !e.OwnershipVerified || len(e.Sources) == 0 || len(e.Sources) > 2 || len(e.References) == 0 || len(e.References) > 16 {
		return app.ErrConflict
	}
	if e.SourceKind != "start" && e.SourceKind != "answer" && e.SourceKind != "cancel" {
		return app.ErrConflict
	}
	if e.SourceKind == "start" && (e.CommandID != e.RequestID || e.ResourceID != e.RequestID) {
		return app.ErrConflict
	}
	if transferred {
		if e.Conclusion != "transferred_verified" || e.Reason != "handoff_verified" || e.VerificationMethod != "source_identity_hash_and_live_ledger" || !validDigest(e.LiveBodySHA256) {
			return app.ErrConflict
		}
	} else if e.LiveBodySHA256 != "" || !e.ResponsibilityClosed || !e.BusinessTerminal || e.VerificationMethod != "source_identity_hash_and_business_closure" || ((e.Conclusion != "verified" || e.Reason != "history_terminal_verified") && (e.Conclusion != "unverifiable" || e.Reason != "history_terminal_evidence_gap")) {
		return app.ErrConflict
	}
	seen := map[string]bool{}
	for _, source := range e.Sources {
		kind := ""
		switch source.Table {
		case "ai_bridge_commands":
			kind = "mysql_json_payload_cast_binary_sha256"
		case "ai_messaging_legacy_commands":
			kind = "mysql_blob_source_payload_sha256"
		default:
			return app.ErrConflict
		}
		if seen[source.Table] || source.CommandID != e.CommandID || source.BytesKind != kind || !validDigest(source.BytesSHA256) || !validDigest(source.BusinessPayloadHash) {
			return app.ErrConflict
		}
		seen[source.Table] = true
	}
	if !seen["ai_bridge_commands"] || (transferred && !seen["ai_messaging_legacy_commands"]) {
		return app.ErrConflict
	}
	for _, reference := range e.References {
		if reference.Kind != "business_record" && reference.Kind != "operation" && reference.Kind != "event" && reference.Kind != "migration_manifest" && reference.Kind != "readonly_run" {
			return app.ErrConflict
		}
		if len(reference.ID) > 192 || !retirementToken.MatchString(reference.ID) {
			return app.ErrConflict
		}
	}
	return nil
}

func validDigest(value string) bool {
	return len(value) == 64 && retirementToken.MatchString(value) && messagingHashDigest(value)
}

func messagingHashDigest(value string) bool {
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
