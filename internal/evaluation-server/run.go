package evaluation

import "github.com/yshujie/questionnaire-scale/internal/evaluation-server/config"

// Run 运行指定的 Evaluation Server。此函数不应退出。
func Run(cfg *config.Config) error {
	server, err := createEvaluationServer(cfg)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run()
}
