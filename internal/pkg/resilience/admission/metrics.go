package admission

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

// ObserveHTTPGateWait 记录 HTTP 槽位等待时长。
func ObserveHTTPGateWait(duration time.Duration) {
	resilience.ObserveHTTPGateWait(duration)
}

// ObserveGRPCInflightWait 记录 gRPC inflight 槽位等待时长。
func ObserveGRPCInflightWait(duration time.Duration) {
	resilience.ObserveGRPCInflightWait(duration)
}
