package collection

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	serverprocess "github.com/FangcunMount/qs-server/internal/collection-server/process"
)

var runProcess = serverprocess.Run

// Run 运行指定的 Collection Server。此函数不应退出。
func Run(cfg *config.Config) error {
	return runProcess(cfg)
}
