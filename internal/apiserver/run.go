package apiserver

import "github.com/fangcun-mount/qs-server/internal/apiserver/config"

// Run 运行指定的 APIServer。此函数不应退出。
func Run(cfg *config.Config) error {
	server, err := createAPIServer(cfg)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run()
}
