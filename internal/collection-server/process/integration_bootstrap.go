package process

import (
	grpcclientintegration "github.com/FangcunMount/qs-server/internal/collection-server/integration/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"google.golang.org/grpc/credentials"
)

func (s *server) initializeIntegrations(_ resourceOutput, containerOutput containerOutput) (integrationOutput, error) {
	var output integrationOutput
	if containerOutput.container == nil {
		return output, nil
	}

	var perRPC credentials.PerRPCCredentials
	if h := containerOutput.container.IAMModule.ServiceAuthHelper(); h != nil {
		perRPC = h
	}

	signer, err := delegatedsubject.NewSignerFromOptions(s.config.DelegatedSubject)
	if err != nil {
		return integrationOutput{}, err
	}

	grpcManager, err := grpcclientintegration.CreateGRPCClientManager(
		s.config.GRPCClient.Endpoint,
		s.config.GRPCClient.Timeout,
		s.config.GRPCClient.Insecure,
		s.config.GRPCClient.TLSCertFile,
		s.config.GRPCClient.TLSKeyFile,
		s.config.GRPCClient.TLSCAFile,
		s.config.GRPCClient.TLSServerName,
		s.config.GRPCClient.InflightWaitMs,
		containerOutput.container.GRPCDownstreamGate(),
		perRPC,
		signer,
	)
	if err != nil {
		return integrationOutput{}, err
	}
	output.grpcClients.grpcManager = grpcManager

	grpcRegistry := grpcclientintegration.NewRegistry(grpcManager)
	containerOutput.container.InitializeRuntimeClients(grpcRegistry.ClientBundle())
	if err := containerOutput.container.Initialize(); err != nil {
		return integrationOutput{}, err
	}
	return output, nil
}
