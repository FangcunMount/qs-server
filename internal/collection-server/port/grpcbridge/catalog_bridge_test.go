package grpcbridge

import (
	"context"
	"errors"
	"testing"
)

type stubScaleReader struct {
	getScale func(context.Context, string) (*ScaleOutput, error)
}

func (s stubScaleReader) GetScale(ctx context.Context, code string) (*ScaleOutput, error) {
	return s.getScale(ctx, code)
}

func (s stubScaleReader) ListScales(context.Context, int32, int32, string, string, string, []string, []string, []string, []string) (*ListScalesOutput, error) {
	return nil, nil
}

func (s stubScaleReader) ListHotScales(context.Context, int32, int32) (*ListHotScalesOutput, error) {
	return nil, nil
}

func (s stubScaleReader) GetScaleCategories(context.Context) (*ScaleCategoriesOutput, error) {
	return nil, nil
}

func TestScaleCatalogReaderNilReceiver(t *testing.T) {
	t.Parallel()

	var reader *ScaleCatalogReader
	out, err := reader.GetScale(context.Background(), "scl-1")
	if err != nil || out != nil {
		t.Fatalf("nil reader: got (%v, %v), want (nil, nil)", out, err)
	}
}

func TestScaleCatalogReaderNilInner(t *testing.T) {
	t.Parallel()

	reader := NewScaleCatalogReader(nil)
	out, err := reader.GetScale(context.Background(), "scl-1")
	if err != nil || out != nil {
		t.Fatalf("nil inner: got (%v, %v), want (nil, nil)", out, err)
	}
}

func TestScaleCatalogReaderPropagatesError(t *testing.T) {
	t.Parallel()

	want := errors.New("upstream failed")
	reader := NewScaleCatalogReader(stubScaleReader{
		getScale: func(context.Context, string) (*ScaleOutput, error) {
			return nil, want
		},
	})
	_, err := reader.GetScale(context.Background(), "scl-1")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestScaleCatalogReaderMapsResponse(t *testing.T) {
	t.Parallel()

	reader := NewScaleCatalogReader(stubScaleReader{
		getScale: func(context.Context, string) (*ScaleOutput, error) {
			return &ScaleOutput{Code: "scl-1", Title: "demo"}, nil
		},
	})
	out, err := reader.GetScale(context.Background(), "scl-1")
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Code != "scl-1" || out.Title != "demo" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestCatalogBridgeNilRaw(t *testing.T) {
	t.Parallel()

	reader := stubScaleReader{
		getScale: func(context.Context, string) (*ScaleOutput, error) {
			return nil, nil
		},
	}
	out, err := CallBridge(reader, func() (*ScaleOutput, error) { return nil, nil }, toScaleResponse)
	if err != nil || out != nil {
		t.Fatalf("nil raw: got (%v, %v), want (nil, nil)", out, err)
	}
}
