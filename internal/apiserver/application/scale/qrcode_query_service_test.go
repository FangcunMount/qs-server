package scale

import (
	"context"
	"testing"
)

func TestScaleQRCodeQueryServiceDelegatesToGenerator(t *testing.T) {
	generator := &scaleQRCodeGeneratorStub{url: "https://example.test/s.png"}
	service := NewQRCodeQueryService(generator)

	got, err := service.GetQRCode(context.Background(), "S-1")
	if err != nil {
		t.Fatalf("GetQRCode() error = %v", err)
	}
	if got != generator.url {
		t.Fatalf("GetQRCode() = %q, want %q", got, generator.url)
	}
	if generator.code != "S-1" {
		t.Fatalf("generator code = %q, want S-1", generator.code)
	}
}

func TestScaleQRCodeQueryServiceRejectsMissingGenerator(t *testing.T) {
	service := NewQRCodeQueryService(nil)

	if _, err := service.GetQRCode(context.Background(), "S-1"); err == nil {
		t.Fatal("GetQRCode() error = nil, want error")
	}
}

type scaleQRCodeGeneratorStub struct {
	url  string
	code string
}

func (s *scaleQRCodeGeneratorStub) GenerateScaleQRCode(_ context.Context, code string) (string, error) {
	s.code = code
	return s.url, nil
}
