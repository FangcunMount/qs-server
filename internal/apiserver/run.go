package apiserver

import "github.com/yshujie/questionnaire-scale/internal/apiserver/config"

// Run 运行指定的 APIServer。这个函数不应该退出。
func Run(cfg *config.Config) error {
	server, err := createAPIServer(cfg)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run()
}
