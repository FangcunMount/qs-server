package process

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	grpcclientintegration "github.com/FangcunMount/qs-server/internal/collection-server/integration/grpcclient"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
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

	grpcManager, err := grpcclientintegration.CreateGRPCClientManager(
		s.config.GRPCClient.Endpoint,
		s.config.GRPCClient.Timeout,
		s.config.GRPCClient.Insecure,
		s.config.GRPCClient.TLSCertFile,
		s.config.GRPCClient.TLSKeyFile,
		s.config.GRPCClient.TLSCAFile,
		s.config.GRPCClient.TLSServerName,
		s.config.GRPCClient.MaxInflight,
		perRPC,
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
	output.iamSync.authzVersionSubscriber = s.startAuthzVersionSync(containerOutput.container)
	return output, nil
}

func (s *server) startAuthzVersionSync(c *container.Container) messaging.Subscriber {
	if s == nil || c == nil || c.IAMModule == nil {
		return nil
	}
	loader := c.IAMModule.AuthzSnapshotLoader()
	authzSync := s.config.IAMOptions.AuthzSync
	if loader == nil || authzSync == nil || !authzSync.Enabled {
		return nil
	}

	subscriber, err := authzSync.NewSubscriber()
	if err != nil {
		log.Warnf("Failed to create collection authz version subscriber: %v", err)
		return nil
	}
	channelPrefix := authzSync.ChannelPrefix
	if channelPrefix == "" {
		channelPrefix = "qs-authz-sync"
	}
	channel := iamauth.DefaultVersionSyncChannel(channelPrefix + "-collection")
	if err := iamauth.SubscribeVersionChanges(context.Background(), subscriber, authzSync.Topic, channel, loader); err != nil {
		_ = subscriber.Close()
		log.Warnf("Failed to subscribe collection authz version sync: %v", err)
		return nil
	}
	return subscriber
}
