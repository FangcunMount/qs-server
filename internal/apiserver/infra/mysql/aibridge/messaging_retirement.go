package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
// receipt or permission to execute. The maintenance caller must independently
// verify terminal state/ownership/responsibility; this store enforces source CAS
// and placement in the caller's already active, closed-admission transaction.
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

func validRetirementID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}

func (e CommandRetirementEvidence) validate(transferred bool) error {
	org, err := strconv.ParseUint(e.OrganizationID, 10, 64)
	_, offset := e.VerifiedAt.Zone()
	if e.Version != 1 || !validRetirementID(e.OperationID) || !validRetirementID(e.CommandID) || !validRetirementID(e.RequestID) || !validRetirementID(e.ResourceID) || err != nil || org == 0 || strconv.FormatUint(org, 10) != e.OrganizationID || e.SubjectID == "" || len(e.SubjectID) > 128 || len(e.VerifierVersion) > 128 || !retirementToken.MatchString(e.VerifierVersion) || e.VerifiedAt.IsZero() || offset != 0 || !e.OwnershipVerified || len(e.Sources) == 0 || len(e.Sources) > 2 || len(e.References) == 0 || len(e.References) > 16 {
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
	} else if e.LiveBodySHA256 != "" || !e.ResponsibilityClosed || !e.BusinessTerminal || e.VerificationMethod != "source_identity_hash_and_business_closure" || !((e.Conclusion == "verified" && e.Reason == "history_terminal_verified") || (e.Conclusion == "unverifiable" && e.Reason == "history_terminal_evidence_gap")) {
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
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func retirementPool(tx *gorm.DB) (gorm.ConnPool, error) {
	if tx == nil || tx.Error != nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return nil, app.ErrConflict
	}
	if prepared, ok := tx.Statement.ConnPool.(*gorm.PreparedStmtTX); ok && prepared == nil {
		return nil, app.ErrConflict
	}
	// Use the same SDK gate as standard message writes. A Commit/Rollback
	// interface alone does not establish an original SQL transaction.
	if _, err := sdkmysql.BindGORM(tx); err != nil {
		return nil, app.ErrConflict
	}
	return tx.Statement.ConnPool, nil
}

func verifyRetirementSources(ctx context.Context, pool gorm.ConnPool, e CommandRetirementEvidence) error {
	var closed bool
	var revision uint64
	if err := pool.QueryRowContext(ctx, "SELECT closed,revision FROM ai_messaging_admission WHERE singleton=1 LOCK IN SHARE MODE").Scan(&closed, &revision); err != nil {
		return err
	}
	if !closed || revision != e.AdmissionRevision {
		return app.ErrConflict
	}
	var original []byte
	var originalHash string
	if err := pool.QueryRowContext(ctx, "SELECT CAST(payload AS BINARY),request_hash FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", e.RequestID).Scan(&original, &originalHash); err != nil {
		return err
	}
	var request app.Start
	if json.Unmarshal(original, &request) != nil || request.RequestID != e.RequestID || request.Actor.OrgID != e.OrganizationID || request.Actor.SubjectID != e.SubjectID {
		return app.ErrConflict
	}
	_, hash, err := encode(request)
	if err != nil || hash != originalHash {
		return app.ErrConflict
	}
	for _, source := range e.Sources {
		query := "SELECT request_id,kind,CAST(payload AS BINARY),payload_hash FROM ai_bridge_commands WHERE command_id=? FOR UPDATE"
		if source.Table == "ai_messaging_legacy_commands" {
			query = "SELECT request_id,source_kind,source_payload,source_payload_hash FROM ai_messaging_legacy_commands WHERE command_id=? FOR UPDATE"
		}
		var requestID, kind, businessHash string
		var raw []byte
		if err := pool.QueryRowContext(ctx, query, e.CommandID).Scan(&requestID, &kind, &raw, &businessHash); err != nil {
			return err
		}
		if requestID != e.RequestID || kind != e.SourceKind || messagingHash(raw) != source.BytesSHA256 || businessHash != source.BusinessPayloadHash {
			return app.ErrConflict
		}
		if kind == "start" {
			var value app.Start
			if json.Unmarshal(raw, &value) != nil || value.RequestID != e.RequestID || value.Actor != request.Actor {
				return app.ErrConflict
			}
			_, hash, err = encode(value)
		} else {
			var value app.Change
			if json.Unmarshal(raw, &value) != nil || value.CommandID != e.CommandID || value.SessionID != e.ResourceID || value.Action != e.SourceKind || value.Actor != request.Actor {
				return app.ErrConflict
			}
			_, hash, err = encode(value)
		}
		if err != nil || hash != businessHash {
			return app.ErrConflict
		}
	}
	return nil
}

func retirementMetadataMatches(raw []byte, expected CommandRetirementEvidence) bool {
	var stored CommandRetirementEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&stored) != nil {
		return false
	}
	return reflect.DeepEqual(stored, expected)
}

// RecordOperationRetirement adds only a durable forbidden-ID fact. It never
// commits/opens a transaction, creates an Outbox, reserves order or seals bytes.
func RecordOperationRetirement(ctx context.Context, tx *gorm.DB, e CommandRetirementEvidence) error {
	return recordOperationRetirement(ctx, tx, e, false)
}

// RecordOperationTransferEvidence appends a verified handoff summary to an
// existing live operation without changing any original submission or receipt.
func RecordOperationTransferEvidence(ctx context.Context, tx *gorm.DB, e CommandRetirementEvidence) error {
	return recordOperationRetirement(ctx, tx, e, true)
}

func recordOperationRetirement(ctx context.Context, tx *gorm.DB, e CommandRetirementEvidence, transferred bool) error {
	if err := e.validate(transferred); err != nil {
		return err
	}
	pool, err := retirementPool(tx)
	if err != nil {
		return err
	}
	if err = verifyRetirementSources(ctx, pool, e); err != nil {
		return err
	}
	kind := int32(pb.MessagingKind_CHANGE)
	if e.SourceKind == "start" {
		kind = int32(pb.MessagingKind_START)
	}
	var retired bool
	var actualKind int32
	var org, subject, resource, aggregate string
	var metadata []byte
	err = pool.QueryRowContext(ctx, "SELECT retired,kind,CAST(organization_id AS CHAR),subject_id,resource_id,aggregate_key,retirement_evidence FROM ai_messaging_operations WHERE command_id=? FOR UPDATE", e.CommandID).Scan(&retired, &actualKind, &org, &subject, &resource, &aggregate, &metadata)
	if err == nil {
		if retired == transferred || actualKind != kind || org != e.OrganizationID || subject != e.SubjectID || resource != e.ResourceID || aggregate != e.RequestID {
			return app.ErrConflict
		}
		if !transferred && len(metadata) == 0 {
			return app.ErrConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	} else if transferred {
		return app.ErrConflict
	}
	var outboxes int
	if transferred {
		var operationHash, outboxHash, aggregateKey, mappingHash, wireHash, outboxOrg string
		var body, wire []byte
		var outboxKind int32
		var operationSequence, outboxSequence uint64
		if err = pool.QueryRowContext(ctx, `SELECT o.body_sha256,b.body_sha256,b.aggregate_key,b.kind,m.messaging_body_sha256,b.body,b.wire,b.wire_sha256,CAST(b.organization_id AS CHAR),o.aggregate_sequence,b.aggregate_sequence FROM ai_messaging_operations o JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=o.command_id JOIN ai_messaging_legacy_commands m ON m.command_id=o.command_id WHERE o.command_id=? FOR UPDATE`, e.CommandID).Scan(&operationHash, &outboxHash, &aggregateKey, &outboxKind, &mappingHash, &body, &wire, &wireHash, &outboxOrg, &operationSequence, &outboxSequence); err != nil {
			return err
		}
		if !validDigest(operationHash) || operationHash != e.LiveBodySHA256 || operationHash != outboxHash || operationHash != mappingHash || aggregateKey != e.RequestID || outboxKind != kind || messagingHash(body) != operationHash || len(wire) == 0 || messagingHash(wire) != wireHash || outboxOrg != e.OrganizationID || operationSequence == 0 || operationSequence != outboxSequence {
			return app.ErrConflict
		}
	} else {
		if err = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox WHERE producer='qs-server' AND destination='qs-ai' AND message_id=?", e.CommandID).Scan(&outboxes); err != nil {
			return err
		}
		if outboxes != 0 {
			return app.ErrConflict
		}
		// A mapping with missing live facts is unresolved, not an unmapped
		// historical ID. It must not be silently replaced by a retirement.
		var mappings int
		if err = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_legacy_commands WHERE command_id=?", e.CommandID).Scan(&mappings); err != nil {
			return err
		}
		if mappings != 0 {
			return app.ErrConflict
		}
	}
	if len(metadata) != 0 {
		if !retirementMetadataMatches(metadata, e) {
			return app.ErrConflict
		}
		return nil
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return app.ErrConflict
	}
	var result sql.Result
	if transferred {
		result, err = pool.ExecContext(ctx, "UPDATE ai_messaging_operations SET retirement_evidence=? WHERE command_id=? AND retired=FALSE AND retirement_evidence IS NULL", raw, e.CommandID)
	} else {
		result, err = pool.ExecContext(ctx, `INSERT INTO ai_messaging_operations(command_id,kind,body_sha256,organization_id,subject_id,resource_id,aggregate_key,aggregate_sequence,created_at,retired,retirement_evidence,retired_at) VALUES(?,?,NULL,?,?,?,?,NULL,NULL,TRUE,?,UTC_TIMESTAMP(6))`, e.CommandID, kind, e.OrganizationID, e.SubjectID, e.ResourceID, e.RequestID, raw)
	}
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return app.ErrConflict
	}
	return nil
}
