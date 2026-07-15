package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	objectstorage "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/gin-gonic/gin"
)

type assessmentImageStoreStub struct {
	key    string
	reader *objectstorage.ObjectReader
	err    error
}

func (*assessmentImageStoreStub) Put(context.Context, string, string, []byte) error { return nil }

func (s *assessmentImageStoreStub) Get(_ context.Context, key string) (*objectstorage.ObjectReader, error) {
	s.key = key
	if s.err != nil {
		return nil, s.err
	}
	return s.reader, nil
}

func TestAssessmentImageHandlerStreamsPrivateObjectWithStableCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	filename := strings.Repeat("a", 64) + ".webp"
	store := &assessmentImageStoreStub{reader: &objectstorage.ObjectReader{
		Body:          io.NopCloser(strings.NewReader("image-data")),
		ContentType:   "image/webp",
		ContentLength: int64(len("image-data")),
	}}
	handler := NewAssessmentImageHandler(store, "assessment-assets/typology")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-assets/typology/MBTI_DEMO/INTJ/"+filename, nil)
	c.Params = gin.Params{{Key: "model", Value: "MBTI_DEMO"}, {Key: "outcome", Value: "INTJ"}, {Key: "filename", Value: filename}}
	handler.GetOutcomeImage(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.key != "assessment-assets/typology/MBTI_DEMO/INTJ/"+filename {
		t.Fatalf("object key = %q", store.key)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("content type = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", got)
	}
	if got := recorder.Body.String(); got != "image-data" {
		t.Fatalf("body = %q", got)
	}
}

func TestAssessmentImageHandlerRejectsUnsafePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAssessmentImageHandler(&assessmentImageStoreStub{}, "assessment-assets/typology")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-assets/typology/../INTJ/not-an-asset.txt", nil)
	c.Params = gin.Params{{Key: "model", Value: ".."}, {Key: "outcome", Value: "INTJ"}, {Key: "filename", Value: "not-an-asset.txt"}}
	handler.GetOutcomeImage(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
