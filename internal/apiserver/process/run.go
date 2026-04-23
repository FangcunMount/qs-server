package process

import apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"

func Run(cfg *apiserverconfig.Config) error {
	server, err := createServer(cfg)
	if err != nil {
		return err
	}
	return server.PrepareRun().Run()
}
