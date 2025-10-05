package evaluation

import "github.com/fangcun-mount/qs-server/internal/evaluation-server/config"

// Run 运行指定的 Evaluation Server。此函数不应退出。
func Run(cfg *config.Config) error {
	server, err := createEvaluationServer(cfg)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run()
}
