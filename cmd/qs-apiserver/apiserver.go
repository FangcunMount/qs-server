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

// 部署流程：cd.yml → ServerD runner → SSH(SVRD) → remote-deploy.sh。
func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
