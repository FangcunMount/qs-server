package process

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
)

func (s *server) initializeContainer(resources resourceOutput) (containerOutput, error) {
	collectionContainer, err := container.NewContainer(s.config.Options, resources.redisRuntime.opsHandle, resources.redisRuntime.locks, resources.redisRuntime.familyStatus)
	if err != nil {
		return containerOutput{}, err
	}
	output := containerOutput{
		container: collectionContainer,
	}
	if output.container == nil {
		return output, nil
	}

	iamModule, err := container.NewIAMModule(context.Background(), s.config.IAMOptions)
	if err != nil {
		return containerOutput{}, err
	}
	output.container.IAMModule = iamModule
	return output, nil
}
