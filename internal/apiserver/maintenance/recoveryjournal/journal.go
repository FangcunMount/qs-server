// Package recoveryjournal owns the private, single-use operator journal.
// It does not decide business eligibility or publish messages.
package recoveryjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type Authority interface{ RecoveryEventID() string }

type Intent[P Authority] struct {
	Plan                   P         `json:"original_authority"`
	RequestID              string    `json:"recovery_request_id"`
	Operator               string    `json:"operator"`
	Reason                 string    `json:"reason"`
	ExternalResultReviewed bool      `json:"external_result_reviewed"`
	RequestedAt            time.Time `json:"requested_at"`
}
type Receipt struct {
	RequestID                string    `json:"recovery_request_id"`
	EventID                  string    `json:"event_id"`
	TransportOutcome         string    `json:"transport_outcome"`
	BusinessCompletionProven bool      `json:"business_completion_proven"`
	RecordedAt               time.Time `json:"recorded_at"`
	EffectOutcome            string    `json:"effect_outcome,omitempty"`
	AssessmentID             uint64    `json:"assessment_id,omitempty"`
}

// Reservation and intent are exclusive and fsynced before PUB. A missing
// receipt or Unknown never causes another PUB, even with a different request.
// This single-use operator journal is not a distributed consumer ledger. Its
// private directory must remain on the designated recovery host and be kept
// for future reconciliation; an operator must not bypass it with a new path.
func Reserve[P Authority](dir string, intent Intent[P]) error {
	info, err := os.Lstat(dir)
	if err != nil || !filepath.IsAbs(dir) || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("existing absolute private audit directory required")
	}
	id, err := uuid.Parse(intent.RequestID)
	if err != nil || id.String() != intent.RequestID || intent.Operator == "" || intent.Reason == "" || !intent.ExternalResultReviewed {
		return fmt.Errorf("explicit reviewed operation required")
	}
	intent.RequestedAt = time.Now().In(time.FixedZone("UTC+8", 8*3600))
	key := sha256.Sum256([]byte(intent.Plan.RecoveryEventID()))
	if err := PersistExclusive(dir, "original-"+hex.EncodeToString(key[:])+".reservation.json", intent); err != nil {
		return err
	}
	return PersistExclusive(dir, intent.RequestID+".intent.json", intent)
}

func PersistExclusive(dir, name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("audit record unavailable or prior operation requires reconciliation")
	}
	if _, err = f.Write(append(encoded, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr = d.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func Read[P Authority](dir, requestID string) (Intent[P], *Receipt, error) {
	var intent Intent[P]
	info, err := os.Lstat(dir)
	if err != nil || !filepath.IsAbs(dir) || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return intent, nil, fmt.Errorf("existing absolute private audit directory required")
	}
	id, err := uuid.Parse(requestID)
	if err != nil || id.String() != requestID {
		return intent, nil, fmt.Errorf("canonical operation UUID required")
	}
	read := func(name string, value any) error {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
			return fmt.Errorf("unsafe audit record")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, value)
	}
	if err := read(requestID+".intent.json", &intent); err != nil {
		return intent, nil, err
	}
	if intent.RequestID != requestID || intent.Plan.RecoveryEventID() == "" || !intent.ExternalResultReviewed {
		return intent, nil, fmt.Errorf("audit intent correlation mismatch")
	}
	var receipt Receipt
	if err := read(requestID+".receipt.json", &receipt); os.IsNotExist(err) {
		return intent, nil, nil
	} else if err != nil {
		return intent, nil, err
	}
	if receipt.RequestID != requestID || receipt.EventID != intent.Plan.RecoveryEventID() {
		return intent, nil, fmt.Errorf("audit correlation mismatch")
	}
	return intent, &receipt, nil
}
