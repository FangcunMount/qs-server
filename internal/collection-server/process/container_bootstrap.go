package process

import (
	"context"
	"strings"
	"time"

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
	if s.config.GenericServerRunOptions != nil && strings.EqualFold(strings.TrimSpace(s.config.GenericServerRunOptions.Mode), "release") {
		timeout := 5 * time.Second
		if opts := s.config.IAMOptions; opts != nil && opts.GRPC != nil && opts.GRPC.Timeout > 0 {
			timeout = opts.GRPC.Timeout
		}
		validationCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := iamModule.ValidateRequiredRuntime(validationCtx); err != nil {
			_ = iamModule.Close()
			return containerOutput{}, err
		}
	}
	output.container.IAMModule = iamModule
	return output, nil
}
