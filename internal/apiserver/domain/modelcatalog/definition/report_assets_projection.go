package definition

import (
	"errors"
	"reflect"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

var ErrUnknownReportAssetsProjection = errors.New("unknown report assets projection version")

// ReportAssetsProjectionFrom is a pure projection. V1 freezes the complete old
// Mongo assets encoding shape; V2 retains the complete canonical assets. This
// function does not select runtime reads, write layers or assign record versions.
func ReportAssetsProjectionFrom(def *Definition, version interpretationassets.ProjectionVersion) (interpretationassets.Assets, error) {
	assets := InterpretationAssetsFrom(def)
	switch version {
	case interpretationassets.ProjectionLegacyV1:
		return decodeReportAssetsV1(encodeReportAssetsV1(assets)), nil
	case interpretationassets.ProjectionCanonicalV2:
		return assets, nil
	default:
		return interpretationassets.Assets{}, ErrUnknownReportAssetsProjection
	}
}

type ReportAssetsProjectionStatus string

const (
	ReportAssetsProjectionMatched    ReportAssetsProjectionStatus = "matched"
	ReportAssetsProjectionMismatched ReportAssetsProjectionStatus = "mismatched"
	ReportAssetsProjectionUnassigned ReportAssetsProjectionStatus = "unassigned"
	ReportAssetsProjectionUnknown    ReportAssetsProjectionStatus = "unknown"
	ReportAssetsProjectionMissing    ReportAssetsProjectionStatus = "missing_definition"
)

// CheckReportAssetsProjection compares the entire projected asset value with
// today's effective stored-first read. It is an observation only: missing or
// unknown versions never become a successful match and never alter that read.
func CheckReportAssetsProjection(def *Definition, version interpretationassets.ProjectionVersion) ReportAssetsProjectionStatus {
	if version == interpretationassets.ProjectionUnassigned {
		return ReportAssetsProjectionUnassigned
	}
	if !version.IsKnown() {
		return ReportAssetsProjectionUnknown
	}
	if def == nil {
		return ReportAssetsProjectionMissing
	}
	expected, _ := ReportAssetsProjectionFrom(def, version)
	if !reflect.DeepEqual(normalizeReportAssets(expected), normalizeReportAssets(def.ResolvedInterpretationAssets())) {
		return ReportAssetsProjectionMismatched
	}
	return ReportAssetsProjectionMatched
}

// The frozen v1 encoding DTO deliberately does not depend on today's Mongo PO
// or public Definition serializer. In that encoding TypeProfile had no Rarity.
// Every other profile, outcome and report-routing field remains represented.
type reportAssetsV1 struct {
	Outcomes []reportOutcomeV1
	Profiles []reportProfileV1
	Report   reportSpecV1
}

type reportOutcomeV1 struct{ OutcomeCode, Title, Summary, Description string }
type reportProfileV1 struct {
	OutcomeCode, Pattern                       string
	Traits, Strengths, Weaknesses, Suggestions []string
	ImageURL, Image                            string
	IsSpecial                                  bool
	Trigger, Commentary                        string
}
type reportSpecV1 struct{ Sections []reportSectionV1 }
type reportSectionV1 struct {
	Code, Title                                                  string
	SourceRefs                                                   []string
	Kind, AdapterKey, TemplateID, TemplateVersion, CategoryLabel string
}

func encodeReportAssetsV1(assets interpretationassets.Assets) reportAssetsV1 {
	out := reportAssetsV1{}
	for _, item := range assets.Outcomes {
		out.Outcomes = append(out.Outcomes, reportOutcomeV1{item.OutcomeCode, item.Title, item.Summary, item.Description})
	}
	for _, item := range assets.Profiles {
		out.Profiles = append(out.Profiles, reportProfileV1{
			OutcomeCode: item.OutcomeCode, Pattern: item.Pattern,
			Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...),
			Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...),
			ImageURL: item.ImageURL, Image: item.Image, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary,
		})
	}
	for _, item := range assets.ReportSpec.Sections {
		out.Report.Sections = append(out.Report.Sections, reportSectionV1{
			Code: item.Code, Title: item.Title, SourceRefs: append([]string(nil), item.SourceRefs...),
			Kind: item.Kind, AdapterKey: item.AdapterKey, TemplateID: item.TemplateID,
			TemplateVersion: item.TemplateVersion, CategoryLabel: item.CategoryLabel,
		})
	}
	return out
}

func decodeReportAssetsV1(encoded reportAssetsV1) interpretationassets.Assets {
	out := interpretationassets.Assets{}
	for _, item := range encoded.Outcomes {
		out.Outcomes = append(out.Outcomes, interpretationassets.OutcomePresentation{
			OutcomeCode: item.OutcomeCode, Title: item.Title, Summary: item.Summary, Description: item.Description,
		})
	}
	for _, item := range encoded.Profiles {
		out.Profiles = append(out.Profiles, interpretationassets.TypeProfilePresentation{
			OutcomeCode: item.OutcomeCode, Pattern: item.Pattern,
			Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...),
			Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...),
			ImageURL: item.ImageURL, Image: item.Image, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary,
		})
	}
	for _, item := range encoded.Report.Sections {
		out.ReportSpec.Sections = append(out.ReportSpec.Sections, interpretationassets.ReportSection{
			Code: item.Code, Title: item.Title, SourceRefs: append([]string(nil), item.SourceRefs...),
			Kind: item.Kind, AdapterKey: item.AdapterKey, TemplateID: item.TemplateID,
			TemplateVersion: item.TemplateVersion, CategoryLabel: item.CategoryLabel,
		})
	}
	return out
}

// Only nil versus empty collections are normalized. Order, duplicate codes,
// routing, all copy and all Rarity fields remain exact.
func normalizeReportAssets(assets interpretationassets.Assets) interpretationassets.Assets {
	out := assets
	out.Outcomes = append([]interpretationassets.OutcomePresentation(nil), assets.Outcomes...)
	out.Profiles = append([]interpretationassets.TypeProfilePresentation(nil), assets.Profiles...)
	for i := range out.Profiles {
		out.Profiles[i].Traits = append([]string(nil), assets.Profiles[i].Traits...)
		out.Profiles[i].Strengths = append([]string(nil), assets.Profiles[i].Strengths...)
		out.Profiles[i].Weaknesses = append([]string(nil), assets.Profiles[i].Weaknesses...)
		out.Profiles[i].Suggestions = append([]string(nil), assets.Profiles[i].Suggestions...)
	}
	out.ReportSpec.Sections = append([]interpretationassets.ReportSection(nil), assets.ReportSpec.Sections...)
	for i := range out.ReportSpec.Sections {
		out.ReportSpec.Sections[i].SourceRefs = append([]string(nil), assets.ReportSpec.Sections[i].SourceRefs...)
	}
	return out
}
