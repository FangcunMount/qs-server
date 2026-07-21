package reporttemplate

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

// ResolveVersion returns the explicit version or the compatibility default.
func ResolveVersion(explicit policy.TemplateVersion) policy.TemplateVersion {
	if explicit.IsEmpty() {
		return policy.TemplateVersionV1
	}
	return explicit
}

// ResolveFromAssets reads the first explicit version from frozen interpretation assets.
func ResolveFromAssets(assets interpretationassets.Assets) policy.TemplateVersion {
	for _, section := range assets.ReportSpec.Sections {
		if version := policy.TemplateVersion(section.TemplateVersion); !version.IsEmpty() {
			return version
		}
	}
	return policy.TemplateVersionV1
}
