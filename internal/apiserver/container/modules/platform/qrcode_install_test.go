package platform

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

type qrInstallHost struct {
	// Unused host methods deliberately have no implementation. Optional QR
	// assembly must not acquire a Mongo connection or other unrelated service.
	InstallHost
	state   IntegrationState
	applied bool
}

func (h *qrInstallHost) PlatformState() IntegrationState           { return h.state }
func (*qrInstallHost) WeChatAppService() *iam.WeChatAppService     { return nil }
func (*qrInstallHost) Printf(string, ...any)                       {}
func (h *qrInstallHost) ApplyPlatformState(state IntegrationState) { h.state, h.applied = state, true }

type qrGeneratorFixture struct{ wechatmini.QRCodeGenerator }

func TestQRCodeInstallPreservesOptionalConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		generator wechatmini.QRCodeGenerator
		options   *options.WeChatOptions
	}{
		{name: "no generator"},
		{name: "no options", generator: qrGeneratorFixture{}},
		{name: "missing credentials", generator: qrGeneratorFixture{}, options: &options.WeChatOptions{PagePath: "pages/entry"}},
		{name: "missing page", generator: qrGeneratorFixture{}, options: &options.WeChatOptions{AppID: "app", AppSecret: "test-secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := &qrInstallHost{state: IntegrationState{QRCodeGenerator: tc.generator}}
			if err := InitQRCodeServiceFrom(host, tc.options, nil); err != nil {
				t.Fatal(err)
			}
			if host.applied || host.state.QRCodeService != nil {
				t.Fatal("unconfigured QR must remain optional")
			}
		})
	}
}

func TestQRCodeInstallUsesOrdinaryServiceWithoutMongo(t *testing.T) {
	host := &qrInstallHost{state: IntegrationState{QRCodeGenerator: qrGeneratorFixture{}}}
	config := &options.WeChatOptions{AppID: "app", AppSecret: "test-secret", PagePath: "pages/entry"}
	if err := InitQRCodeServiceFrom(host, config, nil); err != nil {
		t.Fatal(err)
	}
	if !host.applied || host.state.QRCodeService == nil {
		t.Fatal("configured QR service must be installed")
	}
}
