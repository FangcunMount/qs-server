package main

import (
	"github.com/FangcunMount/qs-server/internal/worker"
)

// CD 发布冒烟注释：验证 ACR → ServerD → 生产机全链路（2026-06-07）。

func main() {
	worker.NewApp("qs-worker").Run()
}
