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
// 生产发布（apiserver）：cd.yml 在 CI 通过后由 plan-services 纳入本服务；
// ServerD runner 经 SSH 将镜像 tarball 推到 SVRA 并执行 remote-deploy.sh。

func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
