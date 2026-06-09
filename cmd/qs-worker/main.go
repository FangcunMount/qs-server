package main

import (
	"github.com/FangcunMount/qs-server/internal/worker"
)

// 生产发布（worker）：cd.yml → ServerD runner → SSH(SVRD) → remote-deploy.sh。
// CD 日志会打印 deploy runner / target 的 hostname 与 primary_ip，用于核对是否打到 serverD。

func main() {
	worker.NewApp("qs-worker").Run()
}
