package admission

import "errors"

var (
	// ErrWaitTimeout 在有限等待时间内未获取到槽位。
	ErrWaitTimeout = errors.New("admission wait timeout")
	// ErrTryRejected 非阻塞准入时槽位已满。
	ErrTryRejected = errors.New("admission try rejected")
)
