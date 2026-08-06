package rendering

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
)

type releaseManifestKey struct {
	templateID string
	version    policy.TemplateVersion
}

type releaseManifestCatalog struct {
	items map[releaseManifestKey]domainreporttemplate.ReleaseManifest
}

func NewBuiltinReleaseManifestCatalog() (domainreporttemplate.ManifestCatalog, error) {
	manifests, err := BuiltinReleaseManifests()
	if err != nil {
		return nil, err
	}
	catalog := &releaseManifestCatalog{items: make(map[releaseManifestKey]domainreporttemplate.ReleaseManifest, len(manifests))}
	for _, manifest := range manifests {
		key := releaseManifestKey{templateID: manifest.TemplateID, version: manifest.TemplateVersion}
		if _, duplicate := catalog.items[key]; duplicate {
			return nil, fmt.Errorf("duplicate report template release manifest: %s@%s", key.templateID, key.version)
		}
		catalog.items[key] = manifest.Clone()
	}
	return catalog, nil
}

func (c *releaseManifestCatalog) ResolveManifest(templateID string, version policy.TemplateVersion) (domainreporttemplate.ReleaseManifest, bool) {
	if c == nil {
		return domainreporttemplate.ReleaseManifest{}, false
	}
	manifest, ok := c.items[releaseManifestKey{templateID: templateID, version: version}]
	return manifest.Clone(), ok
}
