package process

import grpcclientintegration "github.com/FangcunMount/qs-server/internal/worker/integration/grpcclient"

func (s *server) initializeIntegrations(containerOutput containerOutput) (integrationOutput, error) {
	if containerOutput.container == nil {
		return integrationOutput{}, nil
	}

	grpcManager, err := grpcclientintegration.CreateGRPCClientManager(s.config.GRPC)
	if err != nil {
		return integrationOutput{}, err
	}
	output := integrationOutput{grpcClients: grpcClientsOutput{grpcManager: grpcManager}}

	grpcRegistry := grpcclientintegration.NewRegistry(grpcManager)
	containerOutput.container.InitializeRuntimeClients(grpcRegistry.ClientBundle())
	if err := containerOutput.container.Initialize(); err != nil {
		return output, err
	}
	return output, nil
}
