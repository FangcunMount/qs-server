package cancelerr

import (
	"context"
	"errors"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsRecognizesCanceledErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "context canceled", err: context.Canceled},
		{name: "wrapped context canceled", err: cberrors.Wrap(context.Canceled, "failed to count accessible testees")},
		{name: "grpc canceled", err: status.Error(codes.Canceled, "context canceled")},
		{name: "wrapped grpc canceled", err: cberrors.Wrap(status.Error(codes.Canceled, "context canceled"), "get scale failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !Is(tt.err) {
				t.Fatalf("Is(%v) = false, want true", tt.err)
			}
		})
	}
}

func TestIsIgnoresNonCanceledErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "nil", err: nil},
		{name: "plain error", err: errors.New("boom")},
		{name: "grpc internal", err: status.Error(codes.Internal, "boom")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Is(tt.err) {
				t.Fatalf("Is(%v) = true, want false", tt.err)
			}
		})
	}
}
