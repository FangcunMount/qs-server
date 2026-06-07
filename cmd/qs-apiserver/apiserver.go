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
// CD 发布冒烟注释：验证 ACR → ServerD → 生产机全链路（2026-06-07）。

func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
