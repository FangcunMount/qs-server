package main

import (
	"github.com/FangcunMount/qs-server/internal/worker"
)

// 生产发布（worker）：CD 在 ServerD runner 执行；SVRD_HOST 为本机 Tailscale IP 时走本地 bootstrap。
func main() {
	worker.NewApp("qs-worker").Run()
}
