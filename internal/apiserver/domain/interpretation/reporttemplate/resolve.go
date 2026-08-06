package reporttemplate

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

// FrozenRoute is the immutable report-template selection stored on an Outcome.
type FrozenRoute struct {
	TemplateID      string
	TemplateVersion policy.TemplateVersion
}

// ResolveFromAssets requires one explicit, consistent route across every frozen
// report section. Historical outcomes are governed before this boundary;
// runtime reconstruction never guesses a compatibility release.
func ResolveFromAssets(assets interpretationassets.Assets) (FrozenRoute, error) {
	if len(assets.ReportSpec.Sections) == 0 {
		return FrozenRoute{}, fmt.Errorf("frozen report sections are required")
	}
	var resolved FrozenRoute
	for index, section := range assets.ReportSpec.Sections {
		templateID := strings.TrimSpace(section.TemplateID)
		if templateID == "" {
			return FrozenRoute{}, fmt.Errorf("frozen report section %d template id is required", index)
		}
		version := policy.TemplateVersion(section.TemplateVersion)
		if version.IsEmpty() {
			return FrozenRoute{}, fmt.Errorf("frozen report section %d template version is required", index)
		}
		if resolved.TemplateID != "" && resolved.TemplateID != templateID {
			return FrozenRoute{}, fmt.Errorf("frozen report section template ids conflict: %s != %s", resolved.TemplateID, templateID)
		}
		if !resolved.TemplateVersion.IsEmpty() && resolved.TemplateVersion != version {
			return FrozenRoute{}, fmt.Errorf("frozen report section template versions conflict: %s != %s", resolved.TemplateVersion, version)
		}
		resolved = FrozenRoute{TemplateID: templateID, TemplateVersion: version}
	}
	return resolved, nil
}
