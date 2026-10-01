package qrcode

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
)

type entryQRRecorder struct {
	path, scene string
	calls       int
	err         error
}

func (r *entryQRRecorder) GenerateQRCode(_ context.Context, _, _, path string, _ int) (io.Reader, error) {
	r.path = path
	r.calls++
	return strings.NewReader("png"), r.err
}
func (r *entryQRRecorder) GenerateUnlimitedQRCode(_ context.Context, _, _, scene, _ string, _ int, _ bool, _ map[string]int, _ bool) (io.Reader, error) {
	r.scene = scene
	r.calls++
	return strings.NewReader("png"), r.err
}

type entryImageRecorder struct{ name string }

func (r *entryImageRecorder) StorePNG(_ context.Context, name string, _ []byte) (string, error) {
	r.name = name
	return "https://example.test/" + name, nil
}

func TestLongEntryUsesPathAndPreservesOriginalIdentity(t *testing.T) {
	for _, kind := range []string{"questionnaire", "scale"} {
		t.Run(kind, func(t *testing.T) {
			qr := &entryQRRecorder{}
			store := &entryImageRecorder{}
			s := NewService(qr, &Config{AppID: "app", AppSecret: "secret", PagePath: "pages/assessment/fill/index"}, nil, store)
			code := "d675225410b64fb496781ae88ca0d295"
			var err error
			if kind == "questionnaire" {
				_, err = s.GenerateQuestionnaireQRCode(context.Background(), code, "2.0.1")
			} else {
				_, err = s.GenerateScaleQRCode(context.Background(), code)
			}
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(qr.path)
			if err != nil {
				t.Fatal(err)
			}
			if qr.calls != 1 || qr.scene != "" || u.Path != "pages/assessment/fill/index" || u.Query().Get("q") != code {
				t.Fatalf("incorrect entry: %+v", qr)
			}
			expected := "scale_" + code + ".png"
			if kind == "questionnaire" {
				if u.Query().Get("v") != "2.0.1" {
					t.Fatal("original version lost")
				}
				expected = "questionnaire_" + code + "_2.0.1.png"
			}
			if store.name != expected {
				t.Fatalf("storage identity changed: %s", store.name)
			}
		})
	}
}
func TestSceneLengthBoundaryAndUncertainResultNeverRetries(t *testing.T) {
	for _, n := range []int{32, 33} {
		t.Run(strings.Repeat("x", n), func(t *testing.T) {
			failure := errors.New("response outcome unknown")
			qr := &entryQRRecorder{err: failure}
			s := &service{qrCodeGen: qr, config: &Config{PagePath: "pages/assessment/fill/index"}}
			_, err := s.generateEntryQRCode(context.Background(), "app", "secret", strings.Repeat("x", n), url.Values{"q": {"original"}})
			if !errors.Is(err, failure) || qr.calls != 1 {
				t.Fatalf("unknown outcome retried: calls=%d err=%v", qr.calls, err)
			}
			if (qr.scene != "") != (n == 32) {
				t.Fatalf("wrong API at boundary %d", n)
			}
		})
	}
}
func TestShortQuestionnaireKeepsUnlimitedScene(t *testing.T) {
	qr := &entryQRRecorder{}
	s := NewService(qr, &Config{AppID: "app", AppSecret: "secret", PagePath: "pages/assessment/fill/index"}, nil, &entryImageRecorder{})
	if _, err := s.GenerateQuestionnaireQRCode(context.Background(), "short", "2.0.1"); err != nil {
		t.Fatal(err)
	}
	if qr.scene != "q=short&v=2.0.1" || qr.path != "" || qr.calls != 1 {
		t.Fatalf("short entry changed: %+v", qr)
	}
}
