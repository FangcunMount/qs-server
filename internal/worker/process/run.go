package process

import workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"

func Run(cfg *workerconfig.Config) error {
	server, err := createServer(cfg)
	if err != nil {
		return err
	}
	return server.PrepareRun().Run()
}
