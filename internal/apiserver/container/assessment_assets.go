package container

import (
	"fmt"

	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	assessmentassets "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/assets"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/aliyunoss"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

// InitAssessmentImageService wires private OSS-backed MBTI outcome assets
// independently from WeChat QR-code availability.
func (c *Container) InitAssessmentImageService(assetOptions *apiserveroptions.AssessmentAssetsOptions, ossOptions *genericoptions.OSSOptions) error {
	if c == nil || assetOptions == nil || !assetOptions.Enabled {
		return nil
	}
	if ossOptions == nil || !ossOptions.Enabled {
		return fmt.Errorf("assessment image assets require enabled OSS")
	}
	store := c.AssessmentAssetStore
	if store == nil {
		store = c.QRCodeObjectStore
	}
	if store == nil {
		created, err := aliyunoss.NewObjectStore(ossOptions)
		if err != nil {
			return fmt.Errorf("initialize assessment image object store: %w", err)
		}
		store = created
	}
	if c.QRCodeObjectStore == nil {
		c.QRCodeObjectStore = store
	}
	c.AssessmentAssetStore = store
	c.AssessmentAssetKeyPrefix = assetOptions.ObjectKeyPrefix
	if c.AssessmentModelModule == nil || c.AssessmentModelModule.ModelRepo == nil {
		return fmt.Errorf("assessment model repository is not initialized")
	}
	c.AssessmentImageService = assessmentassets.Service{
		Models: c.AssessmentModelModule.ModelRepo, Authorizer: modelcatalogApp.SnapshotAuthorizer{}, Store: store,
		Config: assessmentassets.Config{ObjectKeyPrefix: assetOptions.ObjectKeyPrefix, PublicURLPrefix: assetOptions.PublicURLPrefix, MaxUploadBytes: assetOptions.MaxUploadBytes},
	}
	return nil
}
