package process

import "github.com/FangcunMount/qs-server/internal/worker/container"

func (s *server) initializeContainer(resources resourceOutput) (containerOutput, error) {
	return containerOutput{
		container: container.NewContainer(
			s.config.Options,
			s.logger,
			resources.redisRuntime.lockHandle,
			resources.redisRuntime.lockManager,
		),
	}, nil
}
