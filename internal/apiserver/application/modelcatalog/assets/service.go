// Package assets owns private, immutable image objects referenced by typology
// model definitions. It deliberately does not mutate DefinitionV2: callers
// save the returned image_url through the normal authoring workflow.
package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	assessmentasset "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentasset"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

var safeSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type Config struct {
	ObjectKeyPrefix string
	PublicURLPrefix string
	MaxUploadBytes  int64
}

type Service struct {
	Models     modelcatalogport.ModelRepository
	Authorizer modelcatalog.Authorizer
	Store      assessmentasset.ObjectStore
	Config     Config
}

type UploadInput = modelcatalog.OutcomeImageUploadInput

func (s Service) MaxUploadBytes() int64 { return s.Config.MaxUploadBytes }

func (s Service) UploadOutcomeImage(ctx context.Context, actor modelcatalog.ActorContext, input UploadInput) (*modelcatalog.OutcomeImageUploadResult, error) {
	if s.Models == nil || s.Authorizer == nil || s.Store == nil || s.Config.MaxUploadBytes <= 0 {
		return nil, errors.WithCode(code.ErrInternalServerError, "assessment image assets are not configured")
	}
	if !safeSegment.MatchString(input.ModelCode) || !safeSegment.MatchString(input.OutcomeCode) {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code and outcome code must use letters, numbers, _ or -")
	}
	if len(input.Content) == 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "image file is required")
	}
	if int64(len(input.Content)) > s.Config.MaxUploadBytes {
		return nil, errors.WithCode(code.ErrInvalidArgument, "image file exceeds %d bytes", s.Config.MaxUploadBytes)
	}
	model, err := s.Models.FindByCode(ctx, input.ModelCode)
	if err != nil {
		return nil, err
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionEditDefinition, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
		return nil, err
	}
	if model.Kind != domain.KindTypology || model.IsArchived() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "only editable typology models may upload outcome images")
	}
	contentType, extension, err := validateImage(input.Content)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "%s", err)
	}
	digest := sha256.Sum256(input.Content)
	filename := hex.EncodeToString(digest[:]) + "." + extension
	objectKey := path.Join(strings.Trim(s.Config.ObjectKeyPrefix, "/"), input.ModelCode, input.OutcomeCode, filename)
	if err := s.Store.Put(ctx, objectKey, contentType, input.Content); err != nil {
		return nil, fmt.Errorf("store outcome image: %w", err)
	}
	// Upload only stores an immutable object and returns its URL. Mutating the
	// model head (including forking published → draft) happens when the URL is
	// saved into DefinitionV2 through the normal authoring workflow.
	return &modelcatalog.OutcomeImageUploadResult{
		ImageURL:    strings.TrimRight(s.Config.PublicURLPrefix, "/") + "/" + path.Join(input.ModelCode, input.OutcomeCode, filename),
		ContentType: contentType,
		Size:        int64(len(input.Content)),
	}, nil
}

func validateImage(content []byte) (contentType, extension string, err error) {
	if isWebP(content) {
		return "image/webp", "webp", nil
	}
	contentType = http.DetectContentType(content)
	if contentType != "image/png" && contentType != "image/jpeg" {
		return "", "", fmt.Errorf("image must be PNG, JPEG, or WebP")
	}
	if _, _, decodeErr := image.DecodeConfig(bytes.NewReader(content)); decodeErr != nil {
		return "", "", fmt.Errorf("image content is invalid")
	}
	if contentType == "image/png" {
		return contentType, "png", nil
	}
	return contentType, "jpg", nil
}

func isWebP(content []byte) bool {
	return len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP"
}

// ReadAllLimited rejects oversized multipart files before retaining the full
// body in memory. It is exported for the REST adapter and focused tests.
func ReadAllLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("image file exceeds %d bytes", maxBytes)
	}
	return content, nil
}

var _ modelcatalog.OutcomeImageService = Service{}
