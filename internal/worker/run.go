package worker

import (
	"github.com/FangcunMount/qs-server/internal/worker/config"
)

// Run 启动 Worker 服务
func Run(cfg *config.Config) error {
	// 创建 Worker 服务器
	server, err := createWorkerServer(cfg)
	if err != nil {
		return err
	}

	// 准备并运行
	return server.PrepareRun().Run()
}
