//go:build reliable_messaging_m4

package standardoutbox

import (
	"fmt"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

var standardUnfinishedStates = []string{"pending", "retry_wait", "publishing", "quarantined"}

type StatusCount struct {
	State           string
	Count           int64
	OldestCreatedAt *time.Time
}

// BuildStatusSnapshot does not reuse the legacy status helper: standard
// retry_wait and quarantined rows must remain visible under their real names.
func BuildStatusSnapshot(store string, now time.Time, counts []StatusCount) (outboxport.StatusSnapshot, error) {
	if store == "" || now.IsZero() {
		return outboxport.StatusSnapshot{}, fmt.Errorf("standard status requires store and observation time")
	}
	byState := make(map[string]StatusCount, len(counts))
	for _, row := range counts {
		if !standardState(row.State) || row.Count < 0 || (row.Count > 0 && (row.OldestCreatedAt == nil || row.OldestCreatedAt.IsZero())) {
			return outboxport.StatusSnapshot{}, fmt.Errorf("invalid standard outbox status row %q", row.State)
		}
		if _, exists := byState[row.State]; exists {
			return outboxport.StatusSnapshot{}, fmt.Errorf("duplicate standard outbox state %q", row.State)
		}
		byState[row.State] = row
	}
	buckets := make([]outboxport.StatusBucket, 0, len(standardUnfinishedStates))
	for _, state := range standardUnfinishedStates {
		row := byState[state]
		var age float64
		if row.OldestCreatedAt != nil {
			age = now.Sub(*row.OldestCreatedAt).Seconds()
			if age < 0 {
				age = 0
			}
		}
		buckets = append(buckets, outboxport.StatusBucket{
			Status: state, Count: row.Count, OldestCreatedAt: row.OldestCreatedAt, OldestAgeSeconds: age,
		})
	}
	return outboxport.StatusSnapshot{Store: store, GeneratedAt: now, Buckets: buckets}, nil
}

func standardState(value string) bool {
	for _, state := range standardUnfinishedStates {
		if value == state {
			return true
		}
	}
	return false
}
