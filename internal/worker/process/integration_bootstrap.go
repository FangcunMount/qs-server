package process

import grpcclientintegration "github.com/FangcunMount/qs-server/internal/worker/integration/grpcclient"

func (s *server) initializeIntegrations(containerOutput containerOutput) (integrationOutput, error) {
	if containerOutput.container == nil {
		return integrationOutput{}, nil
	}

	grpcManager, err := grpcclientintegration.CreateGRPCClientManager(
		s.config.GRPC,
		30,
	)
	if err != nil {
		return integrationOutput{}, err
	}

	grpcRegistry := grpcclientintegration.NewRegistry(grpcManager, containerOutput.container)
	if err := grpcRegistry.RegisterClients(); err != nil {
		_ = grpcManager.Close()
		return integrationOutput{}, err
	}
	if err := containerOutput.container.Initialize(); err != nil {
		_ = grpcManager.Close()
		return integrationOutput{}, err
	}
	return integrationOutput{
		grpcClients: grpcClientsOutput{grpcManager: grpcManager},
	}, nil
}
