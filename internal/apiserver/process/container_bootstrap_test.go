package process

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
)

func TestBootstrapContainerStageInitializesContainerAndIAMModule(t *testing.T) {
	t.Parallel()

	wantContainer := &container.Container{}
	wantIAM := &container.IAMModule{}
	var initialized *container.Container

	got, err := bootstrapContainerStage(containerStageDeps{
		newContainer: func() *container.Container { return wantContainer },
		newIAMModule: func(context.Context) (*container.IAMModule, error) { return wantIAM, nil },
		initialize: func(c *container.Container) error {
			initialized = c
			return nil
		},
	})
	if err != nil {
		t.Fatalf("bootstrapContainerStage() error = %v", err)
	}
	if got.container != wantContainer {
		t.Fatalf("container = %#v, want %#v", got.container, wantContainer)
	}
	if got.container.IAMModule != wantIAM {
		t.Fatalf("IAMModule = %#v, want %#v", got.container.IAMModule, wantIAM)
	}
	if initialized != wantContainer {
		t.Fatalf("initialized container = %#v, want %#v", initialized, wantContainer)
	}
}

func TestBootstrapContainerStageReturnsIAMError(t *testing.T) {
	t.Parallel()

	_, err := bootstrapContainerStage(containerStageDeps{
		newContainer: func() *container.Container { return &container.Container{} },
		newIAMModule: func(context.Context) (*container.IAMModule, error) { return nil, errors.New("iam boom") },
	})
	if err == nil || err.Error() != "iam boom" {
		t.Fatalf("bootstrapContainerStage() error = %v, want iam boom", err)
	}
}

func TestBootstrapContainerStageReturnsInitializeError(t *testing.T) {
	t.Parallel()

	_, err := bootstrapContainerStage(containerStageDeps{
		newContainer: func() *container.Container { return &container.Container{} },
		initialize:   func(*container.Container) error { return errors.New("init boom") },
	})
	if err == nil || err.Error() != "init boom" {
		t.Fatalf("bootstrapContainerStage() error = %v, want init boom", err)
	}
}

func TestBootstrapIntegrationStageInitializesWeChatAndAuthzSync(t *testing.T) {
	t.Parallel()

	var order []string
	subscriber := &fakeSubscriber{}
	containerOutput := containerOutput{container: &container.Container{}}

	output, err := bootstrapIntegrationStage(integrationStageDeps{
		container: containerOutput.container,
		initializeWeChat: func(c *container.Container) error {
			if c != containerOutput.container {
				t.Fatalf("wechat container = %#v, want %#v", c, containerOutput.container)
			}
			order = append(order, "wechat")
			return nil
		},
		startAuthzVersionSync: func(c *container.Container) messaging.Subscriber {
			if c != containerOutput.container {
				t.Fatalf("authz container = %#v, want %#v", c, containerOutput.container)
			}
			order = append(order, "authz")
			return subscriber
		},
	})
	if err != nil {
		t.Fatalf("bootstrapIntegrationStage() error = %v", err)
	}
	if output.authzVersionSubscriber != subscriber {
		t.Fatalf("authzVersionSubscriber = %#v, want %#v", output.authzVersionSubscriber, subscriber)
	}
	if want := []string{"wechat", "authz"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("integration order = %#v, want %#v", order, want)
	}
}

func TestInitializeWeChatServicesNoopWithoutConfig(t *testing.T) {
	t.Parallel()

	if err := (&server{}).initializeWeChatServices(&container.Container{}); err != nil {
		t.Fatalf("initializeWeChatServices() error = %v, want nil", err)
	}
}
