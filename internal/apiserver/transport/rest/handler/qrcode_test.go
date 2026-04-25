package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/gin-gonic/gin"
)

type fakeQRCodeObjectStore struct {
	key    string
	reader *objectstorageport.ObjectReader
	err    error
}

func (f *fakeQRCodeObjectStore) Put(context.Context, string, string, []byte) error {
	return nil
}

func (f *fakeQRCodeObjectStore) Get(_ context.Context, key string) (*objectstorageport.ObjectReader, error) {
	f.key = key
	if f.err != nil {
		return nil, f.err
	}
	if f.reader == nil {
		return nil, objectstorageport.ErrObjectNotFound
	}
	return f.reader, nil
}

func TestQRCodeHandlerStreamsFromObjectStore(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	store := &fakeQRCodeObjectStore{
		reader: &objectstorageport.ObjectReader{
			Body:          io.NopCloser(strings.NewReader("png-data")),
			ContentType:   "image/png",
			ContentLength: int64(len("png-data")),
			CacheControl:  "public, max-age=604800",
		},
	}
	handler := NewQRCodeHandler(store, "qrcode")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/qrcodes/questionnaire_PHQ9_v1.png", nil)
	c.Request = req
	c.Params = gin.Params{{Key: "filename", Value: "questionnaire_PHQ9_v1.png"}}

	handler.GetQRCodeImage(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if store.key != "qrcode/questionnaire_PHQ9_v1.png" {
		t.Fatalf("expected object key %q, got %q", "qrcode/questionnaire_PHQ9_v1.png", store.key)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected content type image/png, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=604800" {
		t.Fatalf("expected cache-control propagated, got %q", got)
	}
	if got := rec.Body.String(); got != "png-data" {
		t.Fatalf("expected body %q, got %q", "png-data", got)
	}
}
