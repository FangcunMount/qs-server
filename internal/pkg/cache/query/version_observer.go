package query

import "time"

type VersionObserver interface {
	ObserveVersion(operation, result string, duration time.Duration)
	ObserveSuccess()
	ObserveFailure(error)
}
