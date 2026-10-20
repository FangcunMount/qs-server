package retryschedule

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
)

var ErrLegacyOwnership = errors.New("legacy future responsibility is not eligible for transfer")

const handoffPrefix = "m3_future_owner:"

// The original row retains all bytes and records the exact destination plan.
// The marker fits the existing 128-byte error-code field. It is never a witness.
func HandoffMarker(f Frozen) string {
	if !f.Valid() {
		return ""
	}
	digest := f.Digest()
	return handoffPrefix + hex.EncodeToString(digest[:])
}
func IsHandoffMarker(code string) bool {
	if !strings.HasPrefix(code, handoffPrefix) {
		return false
	}
	raw := strings.TrimPrefix(code, handoffPrefix)
	digest, err := hex.DecodeString(raw)
	return err == nil && len(digest) == 32 && raw == hex.EncodeToString(digest)
}

// LegacySource contains the original row facts. Adapters must additionally
// check its deadline against the DATABASE clock when competing with old Relay.
type LegacySource struct {
	Input                                                message.Input
	Fingerprint                                          []byte
	State, Code                                          string
	NextAttemptAt                                        time.Time
	Version, Attempts, Failures                          uint64
	HasClaim, HasLease, HasConfirmation, HasManualReplay bool
}

// MatchLegacy returns true only for an exact previously transferred row.
// Unknown/attempted/due work remains owned by the old chain and must be drained.
func MatchLegacy(f Frozen, source LegacySource, precision time.Duration) (bool, error) {
	if !f.Valid() || (precision != time.Microsecond && precision != time.Millisecond) || source.Version >= math.MaxInt64 || source.Attempts != 0 || source.Failures != 0 || source.HasClaim || source.HasLease || source.HasConfirmation || source.HasManualReplay {
		return false, ErrLegacyOwnership
	}
	actual, err := message.New(source.Input)
	fp := actual.Fingerprint()
	if err != nil || actual.Fingerprint() != f.Message().Fingerprint() || !bytes.Equal(fp[:], source.Fingerprint) {
		return false, ErrLegacyOwnership
	}
	due := f.DueAt().Truncate(precision)

	if !source.NextAttemptAt.Equal(due) {
		return false, ErrLegacyOwnership
	}
	if source.State == "held" && source.Code == HandoffMarker(f) {
		return true, nil
	}
	if source.State != "pending" || source.Code != "" {
		return false, ErrLegacyOwnership
	}
	return false, nil
}
