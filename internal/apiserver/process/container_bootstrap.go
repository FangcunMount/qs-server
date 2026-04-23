package process

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
)

type containerStageDeps struct {
	newContainer func() *container.Container
	newIAMModule func(context.Context) (*container.IAMModule, error)
	initialize   func(*container.Container) error
}

type integrationStageDeps struct {
	container             *container.Container
	initializeWeChat      func(*container.Container) error
	startAuthzVersionSync func(*container.Container) messaging.Subscriber
}

func (s *server) initializeContainer(resources resourceOutput) (containerOutput, error) {
	return bootstrapContainerStage(s.buildContainerStageDeps(resources))
}

func (s *server) buildContainerStageDeps(resources resourceOutput) containerStageDeps {
	if s == nil {
		return containerStageDeps{}
	}

	deps := containerStageDeps{
		newContainer: func() *container.Container {
			return newContainerFromResourceStage(resources)
		},
		initialize: func(c *container.Container) error {
			if c == nil {
				return nil
			}
			return c.Initialize()
		},
	}
	if s.config != nil {
		deps.newIAMModule = func(ctx context.Context) (*container.IAMModule, error) {
			return container.NewIAMModule(ctx, s.config.IAMOptions)
		}
	}
	return deps
}

func bootstrapContainerStage(deps containerStageDeps) (containerOutput, error) {
	if deps.newContainer == nil {
		return containerOutput{}, nil
	}

	output := containerOutput{
		container: deps.newContainer(),
	}
	if output.container == nil {
		return output, nil
	}

	if deps.newIAMModule != nil {
		iamModule, err := deps.newIAMModule(context.Background())
		if err != nil {
			return containerOutput{}, err
		}
		output.container.IAMModule = iamModule
	}
	if deps.initialize != nil {
		if err := deps.initialize(output.container); err != nil {
			return containerOutput{}, err
		}
		return output, nil
	}
	if err := output.container.Initialize(); err != nil {
		return containerOutput{}, err
	}
	return output, nil
}

func (s *server) initializeIntegrations(containerOutput containerOutput) (integrationOutput, error) {
	return bootstrapIntegrationStage(s.buildIntegrationStageDeps(containerOutput))
}

func (s *server) buildIntegrationStageDeps(containerOutput containerOutput) integrationStageDeps {
	if s == nil || containerOutput.container == nil {
		return integrationStageDeps{}
	}

	return integrationStageDeps{
		container:             containerOutput.container,
		initializeWeChat:      s.initializeWeChatServices,
		startAuthzVersionSync: s.startAuthzVersionSync,
	}
}

func bootstrapIntegrationStage(deps integrationStageDeps) (integrationOutput, error) {
	if deps.container == nil {
		return integrationOutput{}, nil
	}
	if deps.initializeWeChat != nil {
		if err := deps.initializeWeChat(deps.container); err != nil {
			return integrationOutput{}, err
		}
	}
	output := integrationOutput{}
	if deps.startAuthzVersionSync != nil {
		output.authzVersionSubscriber = deps.startAuthzVersionSync(deps.container)
	}
	return output, nil
}

func (s *server) initializeWeChatServices(c *container.Container) error {
	if s == nil || s.config == nil || s.config.WeChatOptions == nil || c == nil {
		return nil
	}
	if err := c.InitQRCodeService(s.config.WeChatOptions, s.config.OSSOptions); err != nil {
		return err
	}
	c.InitMiniProgramTaskNotificationService(s.config.WeChatOptions)
	return nil
}

func (s *server) startAuthzVersionSync(c *container.Container) messaging.Subscriber {
	if s == nil || s.config == nil || c == nil || c.IAMModule == nil {
		return nil
	}
	loader := c.IAMModule.AuthzSnapshotLoader()
	authzSync := s.config.IAMOptions.AuthzSync
	if loader == nil || authzSync == nil || !authzSync.Enabled {
		return nil
	}

	subscriber, err := authzSync.NewSubscriber()
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create authz version subscriber",
			"component", "apiserver",
			"error", err.Error(),
		)
		return nil
	}

	channelPrefix := authzSync.ChannelPrefix
	if channelPrefix == "" {
		channelPrefix = "qs-authz-sync"
	}
	channel := iamauth.DefaultVersionSyncChannel(channelPrefix + "-apiserver")
	if err := iamauth.SubscribeVersionChanges(context.Background(), subscriber, authzSync.Topic, channel, loader); err != nil {
		_ = subscriber.Close()
		logger.L(context.Background()).Warnw("Failed to subscribe IAM authz version sync",
			"component", "apiserver",
			"error", err.Error(),
			"channel", channel,
			"topic", authzSync.Topic,
		)
		return nil
	}
	return subscriber
}
