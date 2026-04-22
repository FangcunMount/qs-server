package qrcode

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
)

var errFakeGenerate = errors.New("fake generate failure")

type fakeQRCodeGenerator struct {
	scene string
	page  string
}

var _ wechatPort.QRCodeGenerator = (*fakeQRCodeGenerator)(nil)

func (f *fakeQRCodeGenerator) GenerateQRCode(_ context.Context, _, _, _ string, _ int) (io.Reader, error) {
	return nil, errFakeGenerate
}

func (f *fakeQRCodeGenerator) GenerateUnlimitedQRCode(
	_ context.Context,
	_, _, scene, page string,
	_ int,
	_ bool,
	_ map[string]int,
	_ bool,
) (io.Reader, error) {
	f.scene = scene
	f.page = page
	return nil, errFakeGenerate
}

func TestGenerateQuestionnaireQRCodeUsesQuestionnaireParamKey(t *testing.T) {
	t.Parallel()

	fakeGen := &fakeQRCodeGenerator{}
	svc := &service{
		qrCodeGen: fakeGen,
		config: &Config{
			AppID:       "wx-app",
			AppSecret:   "secret",
			PagePath:    "pages/questionnaire/fill/index",
			WeChatAppID: "",
		},
	}

	err := func() error {
		_, err := svc.GenerateQuestionnaireQRCode(context.Background(), "PHQ9", "v1")
		return err
	}()
	if !errors.Is(err, errFakeGenerate) {
		t.Fatalf("expected generator error, got %v", err)
	}
	if fakeGen.page != "pages/questionnaire/fill/index" {
		t.Fatalf("expected questionnaire fill page, got %q", fakeGen.page)
	}
	if fakeGen.scene != "q=PHQ9&v=v1" {
		t.Fatalf("expected questionnaire scene to use q param, got %q", fakeGen.scene)
	}
	if strings.Contains(fakeGen.scene, "code=") {
		t.Fatalf("questionnaire scene should not use legacy code param, got %q", fakeGen.scene)
	}
}

func TestGenerateQuestionnaireQRCodeFallsBackToQuestionnaireCodeOnlyWhenSceneTooLong(t *testing.T) {
	t.Parallel()

	fakeGen := &fakeQRCodeGenerator{}
	svc := &service{
		qrCodeGen: fakeGen,
		config: &Config{
			AppID:     "wx-app",
			AppSecret: "secret",
			PagePath:  "pages/questionnaire/fill/index",
		},
	}

	longCode := "QUESTIONNAIRECODE1234567890"
	longVersion := "VERSION1234567890"

	err := func() error {
		_, err := svc.GenerateQuestionnaireQRCode(context.Background(), longCode, longVersion)
		return err
	}()
	if !errors.Is(err, errFakeGenerate) {
		t.Fatalf("expected generator error, got %v", err)
	}
	expectedScene := "q=" + longCode
	if fakeGen.scene != expectedScene {
		t.Fatalf("expected fallback scene %q, got %q", expectedScene, fakeGen.scene)
	}
	if len(fakeGen.scene) > 32 {
		t.Fatalf("expected fallback scene within wechat limit, got length %d (%q)", len(fakeGen.scene), fakeGen.scene)
	}
}
