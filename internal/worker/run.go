package worker

import (
	"github.com/FangcunMount/qs-server/internal/worker/config"
	serverprocess "github.com/FangcunMount/qs-server/internal/worker/process"
)

var runProcess = serverprocess.Run

// Run 启动 Worker 服务
func Run(cfg *config.Config) error {
	return runProcess(cfg)
}
