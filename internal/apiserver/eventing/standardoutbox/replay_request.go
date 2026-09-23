package standardoutbox

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// ReplayRequest is the complete business input bound to one manual replay
// request ID. AuthorizedAt is intentionally absent: retries of a lost response
// must recover the original result even when their local clocks differ.
type ReplayRequest struct {
	OrgID     int64
	RequestID string
	Store     string
	Reason    string
	Targets   []ReplayTarget
}

type ReplayTarget struct {
	EventID              string
	ExpectedFailureCount uint64
}

type ReplayResult struct {
	EventID    string
	Authorized bool
	Reason     string
}

// Fingerprint preserves target order because the operator receives ordered
// per-target results. It excludes timestamps and retains the exact reason text.
func (r ReplayRequest) Fingerprint() ([32]byte, error) {
	var empty [32]byte
	if r.OrgID <= 0 || !validReplayText(r.RequestID, 64) || !validReplayText(r.Store, 64) ||
		!validReplayText(r.Reason, 1024) || strings.TrimSpace(r.Reason) == "" ||
		len(r.Targets) == 0 || len(r.Targets) > 100 {
		return empty, errors.New("invalid replay request header")
	}
	seen := make(map[string]struct{}, len(r.Targets))
	for _, target := range r.Targets {
		if !validReplayText(target.EventID, 128) || target.ExpectedFailureCount == 0 {
			return empty, errors.New("invalid replay target")
		}
		if _, duplicate := seen[target.EventID]; duplicate {
			return empty, errors.New("duplicate replay target")
		}
		seen[target.EventID] = struct{}{}
	}
	// A typed structure gives stable field ordering without map serialization.
	encoded, err := json.Marshal(struct {
		Version   string
		OrgID     int64
		RequestID string
		Store     string
		Reason    string
		Targets   []ReplayTarget
	}{"qs-standard-replay/v1", r.OrgID, r.RequestID, r.Store, r.Reason, r.Targets})
	if err != nil {
		return empty, err
	}
	return sha256.Sum256(encoded), nil
}

func validReplayText(s string, maxBytes int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= maxBytes && utf8.ValidString(s)
}
