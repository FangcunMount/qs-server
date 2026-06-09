package main

import (
	"github.com/FangcunMount/qs-server/internal/apiserver"
	_ "github.com/FangcunMount/qs-server/internal/apiserver/docs"
)

// @title QS API Server
// @version 1.0
// @description Questionnaire Scale API server (REST & gRPC)
// @BasePath /api/v1
// @schemes http https
//
// 生产发布（apiserver）：cd.yml → ServerD runner → SSH(SVRA) → remote-deploy.sh。
// CD 日志会打印 deploy runner / target 的 hostname 与 primary_ip，用于核对是否打到 serverA。

func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
