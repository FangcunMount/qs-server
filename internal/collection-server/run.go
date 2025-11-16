package collection

import "github.com/FangcunMount/qs-server/internal/collection-server/config"

// Run 运行指定的 Collection Server。此函数不应退出。
func Run(cfg *config.Config) error {
	server, err := createCollectionServer(cfg)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run()
}
