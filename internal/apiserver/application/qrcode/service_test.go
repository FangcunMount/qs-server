package qrcode

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
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

type successQRCodeGenerator struct {
	scene string
	page  string
	body  string
}

var _ wechatPort.QRCodeGenerator = (*successQRCodeGenerator)(nil)

func (f *successQRCodeGenerator) GenerateQRCode(_ context.Context, _, _, _ string, _ int) (io.Reader, error) {
	return strings.NewReader(f.body), nil
}

func (f *successQRCodeGenerator) GenerateUnlimitedQRCode(
	_ context.Context,
	_, _, scene, page string,
	_ int,
	_ bool,
	_ map[string]int,
	_ bool,
) (io.Reader, error) {
	f.scene = scene
	f.page = page
	return strings.NewReader(f.body), nil
}

type fakeObjectStore struct {
	key         string
	contentType string
	body        []byte
	getReader   *objectstorageport.ObjectReader
	err         error
}

var _ objectstorageport.PublicObjectStore = (*fakeObjectStore)(nil)

func (f *fakeObjectStore) Put(_ context.Context, key string, contentType string, body []byte) error {
	f.key = key
	f.contentType = contentType
	f.body = append([]byte(nil), body...)
	if f.err != nil {
		return f.err
	}
	return nil
}

func (f *fakeObjectStore) Get(_ context.Context, _ string) (*objectstorageport.ObjectReader, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.getReader == nil {
		return nil, objectstorageport.ErrObjectNotFound
	}
	return f.getReader, nil
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

func TestGenerateQuestionnaireQRCodeUploadsToObjectStore(t *testing.T) {
	t.Parallel()

	fakeGen := &successQRCodeGenerator{body: "png-data"}
	store := &fakeObjectStore{}
	svc := &service{
		qrCodeGen:   fakeGen,
		objectStore: store,
		config: &Config{
			AppID:           "wx-app",
			AppSecret:       "secret",
			PagePath:        "pages/questionnaire/fill/index",
			ObjectKeyPrefix: "qrcode",
			PublicURLPrefix: "https://qs.example.com/api/v1/qrcodes",
		},
	}

	got, err := svc.GenerateQuestionnaireQRCode(context.Background(), "PHQ9", "v1")
	if err != nil {
		t.Fatalf("expected upload success, got error %v", err)
	}
	expectedURL := "https://qs.example.com/api/v1/qrcodes/questionnaire_PHQ9_v1.png"
	if got != expectedURL {
		t.Fatalf("expected returned URL %q, got %q", expectedURL, got)
	}
	if store.key != "qrcode/questionnaire_PHQ9_v1.png" {
		t.Fatalf("expected object key %q, got %q", "qrcode/questionnaire_PHQ9_v1.png", store.key)
	}
	if store.contentType != "image/png" {
		t.Fatalf("expected content type image/png, got %q", store.contentType)
	}
	if string(store.body) != "png-data" {
		t.Fatalf("expected uploaded body %q, got %q", "png-data", string(store.body))
	}
}
