package loadguard

import "time"

// Policy 控制读路径并发合并、回源超时与进程内陈旧降级。
type Policy struct {
	Singleflight bool
	StaleOnError bool
	LoadTimeout  time.Duration
}
