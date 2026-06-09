package main

import (
	"github.com/FangcunMount/qs-server/internal/worker"
)

// 生产发布（worker）：cd.yml 在 CI 通过后由 plan-services 纳入本服务；
// ServerD runner 经 SSH 将镜像 tarball 推到 SVRD 并执行 remote-deploy.sh。

func main() {
	worker.NewApp("qs-worker").Run()
}
