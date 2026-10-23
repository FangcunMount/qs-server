package messagingruntime

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

// NativeLimitReader observes host governance, never SDK witnesses or Broker
// confirmation state. Only the host's original configured primary is allowed.
type NativeLimitReader interface {
	Binding() transaction.Binding
	ReadNativeLimit(context.Context) (bool, error)
}

func nativeNoticeKey(b transaction.Binding, revision string, evidence [32]byte) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte("m3-native-limit-notice-v1\x00"))
	for _, value := range []string{b.Owner, b.StoreID, b.WriterEpoch, revision} {
		nativeNoticeField(h, []byte(value))
	}
	nativeNoticeField(h, evidence[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
func nativeNoticeField(h hash.Hash, value []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(value)
}
func nativeNoticeBinding(b transaction.Binding, revision string) bool {
	if !b.Valid() || len(b.Owner) > 128 || len(b.StoreID) > 128 || len(b.WriterEpoch) > 128 || revision != "m3-1" {
		return false
	}
	switch b.Owner {
	case "iam.mysql", "qs.mysql", "qs.mongo", "ai.mysql":
		return true
	}
	return false
}
func nativeNoticeBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, 2*time.Second)
}

// Resolution metadata is governance proof, never business/witness authority.
// Active reads use the owner/status index; imports validate resolved history
// without reopening it or scanning all historical rows on each observation.
func nativeNoticeResolutionValid(state string, at *time.Time, receipt []byte) bool {
	switch state {
	case "blocked":
		return at == nil && receipt == nil
	case "resolved":
		if at == nil || at.IsZero() || len(receipt) != 32 {
			return false
		}
		var digest [32]byte
		copy(digest[:], receipt)
		return digest != [32]byte{}
	default:
		return false
	}
}
