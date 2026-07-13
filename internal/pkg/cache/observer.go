package cache

import "time"

type Operation string
type Result string

const (
	OperationGet        Operation = "get"
	OperationSet        Operation = "set"
	OperationInvalidate Operation = "invalidate"
	OperationSourceLoad Operation = "source_load"
	OperationPayloadRaw Operation = "payload_raw"
	OperationPayloadSet Operation = "payload_stored"

	ResultHit   Result = "hit"
	ResultMiss  Result = "miss"
	ResultOK    Result = "ok"
	ResultError Result = "error"
)

type Event struct {
	Operation Operation
	Result    Result
	Duration  time.Duration
	Size      int
	Err       error
}

// Observer receives cache-kernel events for one pre-bound capability.
type Observer interface {
	Observe(Event)
}

func Observe(observer Observer, event Event) {
	if observer != nil {
		observer.Observe(event)
	}
}
