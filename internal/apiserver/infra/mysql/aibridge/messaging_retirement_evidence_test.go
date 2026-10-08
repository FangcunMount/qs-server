package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestRetirementEvidenceRejectsUnknownPermissionAndMixedDigestKinds(t *testing.T) {
	id := uuid.NewString()
	e := CommandRetirementEvidence{Version: 1, OperationID: uuid.NewString(), VerifierVersion: "retirement/v1", VerificationMethod: "source_identity_hash_and_business_closure", VerifiedAt: time.Now().UTC(), CommandID: id, RequestID: id, SourceKind: "start", OrganizationID: "1", SubjectID: "42", ResourceID: id, Sources: []CommandRetirementSource{{Table: "ai_bridge_commands", CommandID: id, BytesKind: "mysql_json_payload_cast_binary_sha256", BytesSHA256: strings.Repeat("a", 64), BusinessPayloadHash: strings.Repeat("b", 64)}}, References: []CommandRetirementReference{{Kind: "business_record", ID: id}}, Conclusion: "verified", Reason: "history_terminal_verified", OwnershipVerified: true, ResponsibilityClosed: true, BusinessTerminal: true}
	if err := e.validate(false); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"responsibility", "terminal", "owner", "source_kind", "bytes_kind", "standard_hash", "source_reference", "raw_body_reference", "unknown_conclusion", "timezone", "version"} {
		bad := e
		bad.Sources = append([]CommandRetirementSource(nil), e.Sources...)
		bad.References = append([]CommandRetirementReference(nil), e.References...)
		switch scenario {
		case "responsibility":
			bad.ResponsibilityClosed = false
		case "terminal":
			bad.BusinessTerminal = false
		case "owner":
			bad.OwnershipVerified = false
		case "source_kind":
			bad.SourceKind = "unknown"
		case "bytes_kind":
			bad.Sources[0].BytesKind = "standard_body_sha256"
		case "standard_hash":
			bad.LiveBodySHA256 = strings.Repeat("a", 64)
		case "source_reference":
			bad.Sources[0].CommandID = uuid.NewString()
		case "raw_body_reference":
			bad.References[0].ID = "{\"private_body\":\"value\"}"
		case "unknown_conclusion":
			bad.Conclusion = "unknown_execution"
		case "timezone":
			bad.VerifiedAt = bad.VerifiedAt.In(time.FixedZone("offset", 3600))
		case "version":
			bad.Version = 2
		}
		if err := bad.validate(false); !errors.Is(err, app.ErrConflict) {
			t.Fatal(scenario, err)
		}
	}
	gap := e
	gap.Conclusion = "unverifiable"
	gap.Reason = "history_terminal_evidence_gap"
	if err := gap.validate(false); err != nil {
		t.Fatal("closed historical evidence gap incorrectly rejected", err)
	}
}

// A custom wrapper may expose Commit/Rollback while its writes are unrelated to
// the borrowed SQL transaction. Only the SDK's supported original pools qualify.
type unrecognizedRetirementPool struct{ gorm.ConnPool }

func (*unrecognizedRetirementPool) Commit() error   { return nil }
func (*unrecognizedRetirementPool) Rollback() error { return nil }

func TestRetirementPoolUsesOriginalSDKTransactionGate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var nilSQL *sql.Tx
	var nilPrepared *gorm.PreparedStmtTX
	for _, scenario := range []struct {
		name  string
		pool  gorm.ConnPool
		valid bool
	}{
		{"original_sql", tx, true},
		{"original_prepared_sql", &gorm.PreparedStmtTX{Tx: tx}, true},
		{"ordinary_pool", db, false},
		{"unrecognized_wrapper", &unrecognizedRetirementPool{ConnPool: tx}, false},
		{"typed_nil_sql", nilSQL, false},
		{"typed_nil_prepared", nilPrepared, false},
		{"prepared_typed_nil_sql", &gorm.PreparedStmtTX{Tx: nilSQL}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			g := &gorm.DB{Statement: &gorm.Statement{ConnPool: scenario.pool}}
			pool, err := retirementPool(g)
			if scenario.valid {
				if err != nil || pool != scenario.pool {
					t.Fatal("original transaction was not retained", err)
				}
			} else if !errors.Is(err, app.ErrConflict) || pool != nil {
				t.Fatal("unsupported transaction shape was accepted", err)
			}
		})
	}
	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
