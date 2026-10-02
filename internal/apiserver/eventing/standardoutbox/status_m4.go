package standardoutbox

import (
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	sdkoutbox "github.com/FangcunMount/reliable-messaging/outbox"
)

// StatusCount is the SDK standard storage observation. Database reads and
// store names remain host responsibilities.
type StatusCount = sdkoutbox.StandardStatusCount

func BuildStatusSnapshot(store string, now time.Time, counts []StatusCount) (outboxport.StatusSnapshot, error) {
	return sdkoutbox.BuildStandardStatusSnapshot(store, now, counts)
}

func IsUnfinishedState(value string) bool { return sdkoutbox.IsStandardUnfinishedState(value) }
