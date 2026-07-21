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
	if resources.handles.dbManager != nil {
		if db, err := resources.handles.dbManager.GetMongoDatabase(); err == nil {
			workerContainer.SetMongoDatabase(db)
		} else if s.logger != nil {
			s.logger.Warn("attention projection disabled", "error", err.Error())
		}
	}
	return containerOutput{container: workerContainer}, nil
}
