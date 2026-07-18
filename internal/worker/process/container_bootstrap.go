package process

import "github.com/FangcunMount/qs-server/internal/worker/container"

func (s *server) initializeContainer(resources resourceOutput) (containerOutput, error) {
	workerContainer, err := container.NewContainer(
		s.config.Options,
		s.logger,
		resources.redisRuntime.opsHandle,
		resources.redisRuntime.locks,
		resources.eventCatalog,
	)
	if err != nil {
		return containerOutput{}, err
	}
	return containerOutput{container: workerContainer}, nil
}
