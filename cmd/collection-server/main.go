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
// 生产发布（collection）：cd.yml → ServerD runner → SSH(SVRB) → remote-deploy.sh。
// CD 日志会打印 deploy runner / target 的 hostname 与 primary_ip，用于核对是否打到 serverB。

func main() {
	collection.NewApp("collection-server").Run()
}
