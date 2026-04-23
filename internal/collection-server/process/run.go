package process

import collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"

func Run(cfg *collectionconfig.Config) error {
	server, err := createServer(cfg)
	if err != nil {
		return err
	}
	return server.PrepareRun().Run()
}
