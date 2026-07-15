package assets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	objectstorage "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0,
	0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestUploadMBTIOutcomeImageStoresImmutableURL(t *testing.T) {
	store := &memoryStore{}
	service := Service{
		Models: modelRepoStub{model: mbtiDraft()}, Authorizer: allowAuthorizer{}, Store: store,
		Config: Config{ObjectKeyPrefix: "assessment-assets/typology", PublicURLPrefix: "https://qs.example/api/v1/assessment-assets/typology", MaxUploadBytes: 1024},
	}
	result, err := service.UploadMBTIOutcomeImage(context.Background(), modelcatalog.ActorContext{}, UploadInput{ModelCode: "MBTI_DEMO", OutcomeCode: "INTJ", Content: onePixelPNG})
	if err != nil {
		t.Fatalf("UploadMBTIOutcomeImage: %v", err)
	}
	if result.ContentType != "image/png" || result.Size != int64(len(onePixelPNG)) {
		t.Fatalf("result = %#v", result)
	}
	if !strings.HasPrefix(result.ImageURL, "https://qs.example/api/v1/assessment-assets/typology/MBTI_DEMO/INTJ/") || !strings.HasSuffix(result.ImageURL, ".png") {
		t.Fatalf("ImageURL = %q", result.ImageURL)
	}
	if len(store.objects) != 1 {
		t.Fatalf("stored object count = %d", len(store.objects))
	}
}

func TestUploadMBTIOutcomeImageRejectsInvalidOrOversizedContent(t *testing.T) {
	service := Service{Models: modelRepoStub{model: mbtiDraft()}, Authorizer: allowAuthorizer{}, Store: &memoryStore{}, Config: Config{ObjectKeyPrefix: "assets", PublicURLPrefix: "https://qs.example/assets", MaxUploadBytes: 8}}
	if _, err := service.UploadMBTIOutcomeImage(context.Background(), modelcatalog.ActorContext{}, UploadInput{ModelCode: "MBTI_DEMO", OutcomeCode: "INTJ", Content: []byte("not-image")}); err == nil {
		t.Fatal("expected invalid image error")
	}
	if _, err := ReadAllLimited(bytes.NewReader(make([]byte, 9)), 8); err == nil {
		t.Fatal("expected oversized read error")
	}
}

func TestUploadMBTIOutcomeImageRequiresPermissionAndEditableDraft(t *testing.T) {
	config := Config{ObjectKeyPrefix: "assets", PublicURLPrefix: "https://qs.example/assets", MaxUploadBytes: 1024}
	denied := Service{Models: modelRepoStub{model: mbtiDraft()}, Authorizer: denyAuthorizer{}, Store: &memoryStore{}, Config: config}
	if _, err := denied.UploadMBTIOutcomeImage(context.Background(), modelcatalog.ActorContext{}, UploadInput{ModelCode: "MBTI_DEMO", OutcomeCode: "INTJ", Content: onePixelPNG}); err == nil {
		t.Fatal("expected authorization error")
	}
	published := mbtiDraft()
	published.Status = domain.ModelStatusPublished
	notDraft := Service{Models: modelRepoStub{model: published}, Authorizer: allowAuthorizer{}, Store: &memoryStore{}, Config: config}
	if _, err := notDraft.UploadMBTIOutcomeImage(context.Background(), modelcatalog.ActorContext{}, UploadInput{ModelCode: "MBTI_DEMO", OutcomeCode: "INTJ", Content: onePixelPNG}); err == nil {
		t.Fatal("expected published model rejection")
	}
}

type modelRepoStub struct{ model *domain.AssessmentModel }

func (s modelRepoStub) Create(context.Context, *domain.AssessmentModel) error { return nil }
func (s modelRepoStub) Update(context.Context, *domain.AssessmentModel) error { return nil }
func (s modelRepoStub) FindByCode(context.Context, string) (*domain.AssessmentModel, error) {
	return s.model, nil
}
func (s modelRepoStub) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, nil
}
func (s modelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}
func (s modelRepoStub) Delete(context.Context, string) error { return nil }

type allowAuthorizer struct{}

func (allowAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

type denyAuthorizer struct{}

func (denyAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return fmt.Errorf("permission denied")
}

type memoryStore struct{ objects map[string][]byte }

func (s *memoryStore) Put(_ context.Context, key, _ string, body []byte) error {
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[key] = append([]byte(nil), body...)
	return nil
}
func (s *memoryStore) Get(context.Context, string) (*objectstorage.ObjectReader, error) {
	return nil, objectstorage.ErrObjectNotFound
}

func mbtiDraft() *domain.AssessmentModel {
	return &domain.AssessmentModel{Code: "MBTI_DEMO", Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Algorithm: binding.AlgorithmMBTI, Status: domain.ModelStatusDraft}
}

var _ io.Reader = (*bytes.Reader)(nil)
