package process

import (
	"context"
	"log/slog"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	grpcclientintegration "github.com/FangcunMount/qs-server/internal/collection-server/integration/grpcclient"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
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

	options, err := eventtransport.NewSubscriberOptions(0, authzSync.Delivery.EffectiveMaxAttempts(), eventtransport.TerminalFailedMessageHandler(slog.Default(), "collection-server-iam-authz-sync"))
	if err != nil {
		log.Warnf("Failed to configure collection authz version subscriber: %v", err)
		return nil
	}
	subscriber, err := eventtransport.NewSubscriber(eventtransport.SubscriberConfig{
		Provider: authzSync.Provider, NSQLookupdAddr: authzSync.NSQLookupdAddr, RabbitMQURL: authzSync.RabbitMQURL,
	}, options)
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
