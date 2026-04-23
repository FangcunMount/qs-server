package apiserver

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	serverprocess "github.com/FangcunMount/qs-server/internal/apiserver/process"
)

var runProcess = serverprocess.Run

// Run 运行指定的 APIServer。此函数不应退出。
func Run(cfg *config.Config) error {
	return runProcess(cfg)
}
