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
// CD 发布冒烟注释：验证 ACR → ServerD → 生产机全链路（2026-06-07）。

func main() {
	collection.NewApp("collection-server").Run()
}
