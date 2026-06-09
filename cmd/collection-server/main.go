package main

import (
	collection "github.com/FangcunMount/qs-server/internal/collection-server"
	_ "github.com/FangcunMount/qs-server/internal/collection-server/docs"
)

// @title Collection Server
// @version 1.0
// @description Questionnaire collection/BFF layer
// @BasePath /api/v1
// @schemes http https
//
// 生产发布（collection）：cd.yml 在 CI 通过后由 plan-services 纳入本服务；
// ServerD runner 经 SSH 将镜像 tarball 推到 SVRB 并执行 remote-deploy.sh。

func main() {
	collection.NewApp("collection-server").Run()
}
