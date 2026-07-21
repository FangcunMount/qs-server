package evaluationinput

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitivesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// FactorCatalogEntry is the minimal factor metadata frozen for Interpretation replay
// when full ModelPayload is omitted (MC-R017 ReportInput schema v3).
type FactorCatalogEntry struct {
	Code         string   `json:"code"`
	Title        string   `json:"title,omitempty"`
	MaxScore     *float64 `json:"max_score,omitempty"`
	IsTotalScore bool     `json:"is_total_score,omitempty"`
}

// NormingFreeze carries norm lookup tables needed for behavioral norm replay.
type NormingFreeze struct {
	NormTables *norm.NormTables `json:"norm_tables,omitempty"`
}

// TypologyRoutingFreeze is the minimal DefinitionV2 report route needed for replay.
type TypologyRoutingFreeze struct {
	DecisionKind    string `json:"decision_kind"`
	ReportKind      string `json:"report_kind"`
	AdapterKey      string `json:"adapter_key"`
	TemplateID      string `json:"template_id,omitempty"`
	TemplateVersion string `json:"template_version,omitempty"`
}

// ReportInputFreezeOptions controls how evaluation report input is frozen at commit.
type ReportInputFreezeOptions struct {
	Assets          *interpretationassets.Assets
	ModelRef        ModelRef
	AlgorithmFamily modelcatalog.AlgorithmFamily
	FactorCatalog   []FactorCatalogEntry
	TypologySource  *typology.Source
	TypologyRouting *TypologyRoutingFreeze
	Norming         *NormingFreeze
}

// FactorCatalogFromScale builds minimal factor metadata from a scale snapshot.
func FactorCatalogFromScale(scale *scalesnapshot.ScaleSnapshot) []FactorCatalogEntry {
	if scale == nil {
		return nil
	}
	out := make([]FactorCatalogEntry, 0, len(scale.Factors))
	for _, factor := range scale.Factors {
		out = append(out, FactorCatalogEntry{
			Code: factor.Code, Title: factor.Title, MaxScore: factor.MaxScore, IsTotalScore: factor.IsTotalScore,
		})
	}
	return out
}

// FactorCatalogFromBehavioral builds minimal factor metadata from a behavioral snapshot.
func FactorCatalogFromBehavioral(snapshot *behavioralsnapshot.Snapshot) []FactorCatalogEntry {
	if snapshot == nil {
		return nil
	}
	return FactorCatalogFromScale(snapshot.ToScaleSnapshot())
}

// CanFreezeMinimalReportInput reports whether schema v3 (assets + catalog, no payload)
// is sufficient for Interpretation replay on this family.
func CanFreezeMinimalReportInput(opts ReportInputFreezeOptions) bool {
	if opts.Assets == nil || !opts.Assets.IsMaterialized() {
		return false
	}
	if opts.ModelRef.Kind == "" || opts.ModelRef.Code == "" || opts.ModelRef.Version == "" {
		return false
	}
	switch opts.AlgorithmFamily {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return len(opts.FactorCatalog) > 0
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return opts.TypologyRouting != nil && opts.TypologyRouting.DecisionKind != "" &&
			(len(opts.Assets.Profiles) > 0 || len(opts.Assets.Outcomes) > 0)
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return opts.Norming != nil && opts.Norming.NormTables != nil && len(opts.FactorCatalog) > 0
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return len(opts.FactorCatalog) > 0
	default:
		return false
	}
}

func scaleSnapshotFromCatalog(model ModelRef, catalog []FactorCatalogEntry, assets *interpretationassets.Assets) *scalesnapshot.ScaleSnapshot {
	if len(catalog) == 0 && (assets == nil || !assets.IsMaterialized()) {
		return nil
	}
	factors := make([]scalesnapshot.FactorSnapshot, 0, len(catalog))
	for _, item := range catalog {
		factors = append(factors, scalesnapshot.FactorSnapshot{
			Code: item.Code, Title: item.Title, MaxScore: item.MaxScore, IsTotalScore: item.IsTotalScore,
		})
	}
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code: model.Code, ScaleVersion: model.Version, Title: model.Title, Factors: factors,
	}
	if assets != nil && assets.IsMaterialized() {
		cloned := *assets
		snapshot.InterpretationAssets = &cloned
	}
	return snapshot
}

func typologyPayloadFromFreeze(model ModelRef, assets *interpretationassets.Assets, source *typology.Source) TypologyModelPayload {
	src := typology.Source{}
	if source != nil {
		src = *source
	}
	return TypologyModelPayload{Payload: &typology.Payload{
		Code: model.Code, Version: model.Version, Title: model.Title,
		Algorithm: binding.Algorithm(model.Algorithm),
		Outcomes:  typologyOutcomesFromAssets(assets),
		Source:    src,
	}}
}

func typologyOutcomesFromAssets(assets *interpretationassets.Assets) []typology.Outcome {
	if assets == nil {
		return nil
	}
	byCode := make(map[string]typology.Outcome, len(assets.Outcomes)+len(assets.Profiles))
	order := make([]string, 0, len(assets.Outcomes)+len(assets.Profiles))
	for _, item := range assets.Outcomes {
		if item.OutcomeCode == "" {
			continue
		}
		byCode[item.OutcomeCode] = typology.Outcome{
			Code: item.OutcomeCode, Name: item.Title, OneLiner: item.Summary, Summary: item.Summary,
		}
		order = append(order, item.OutcomeCode)
	}
	for _, profile := range assets.Profiles {
		if profile.OutcomeCode == "" {
			continue
		}
		existing := byCode[profile.OutcomeCode]
		byCode[profile.OutcomeCode] = typology.Outcome{
			Code: profile.OutcomeCode, Name: existing.Name, OneLiner: existing.OneLiner,
			Summary: firstNonEmpty(profile.Commentary, existing.Summary),
			Traits:  profile.Traits, Strengths: profile.Strengths, Weaknesses: profile.Weaknesses,
			Suggestions: profile.Suggestions, ImageURL: profile.ImageURL, Pattern: profile.Pattern,
			Image: profile.Image, IsSpecial: profile.IsSpecial, Trigger: profile.Trigger,
			Commentary: profile.Commentary,
			Rarity: typology.Rarity{
				Percent: profile.Rarity.Percent, Label: profile.Rarity.Label, OneInX: profile.Rarity.OneInX,
			},
		}
		if existing.Code == "" {
			order = append(order, profile.OutcomeCode)
		}
	}
	out := make([]typology.Outcome, 0, len(order))
	for _, code := range order {
		out = append(out, byCode[code])
	}
	return out
}

func behavioralPayloadFromFreeze(model ModelRef, catalog []FactorCatalogEntry, assets *interpretationassets.Assets, norming *NormingFreeze) BehavioralRatingModelPayload {
	factors := make([]behavioralsnapshot.FactorSnapshot, 0, len(catalog))
	for _, item := range catalog {
		factors = append(factors, behavioralsnapshot.FactorSnapshot{
			Code: item.Code, Title: item.Title, MaxScore: item.MaxScore, IsTotalScore: item.IsTotalScore,
		})
	}
	snapshot := &behavioralsnapshot.Snapshot{
		Code: model.Code, Version: model.Version, Title: model.Title, Factors: factors,
	}
	if norming != nil && norming.NormTables != nil {
		snapshot.Norming = &behavioralsnapshot.NormingProfile{NormTables: norming.NormTables}
	}
	return BehavioralRatingModelPayload{Snapshot: snapshot}
}

func cognitivePayloadFromFreeze(model ModelRef, catalog []FactorCatalogEntry) CognitiveModelPayload {
	factors := make([]cognitivesnapshot.FactorSnapshot, 0, len(catalog))
	for _, item := range catalog {
		factors = append(factors, cognitivesnapshot.FactorSnapshot{Code: item.Code, Title: item.Title, MaxScore: item.MaxScore, IsTotalScore: item.IsTotalScore})
	}
	return CognitiveModelPayload{Snapshot: &cognitivesnapshot.Snapshot{Code: model.Code, Version: model.Version, Title: model.Title, Factors: factors}}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func snapshotFromMinimalReportInput(model ModelRef, decoded decodedReportInput) (*InputSnapshot, error) {
	snapshot := &InputSnapshot{InterpretationAssets: decoded.InterpretationAssets, TypologyRouting: decoded.TypologyRouting}
	ref := model
	if decoded.ModelRef.Kind != "" {
		ref = decoded.ModelRef
	}
	var payload ModelPayload
	switch ref.Kind {
	case EvaluationModelKindScale:
		scale := scaleSnapshotFromCatalog(ref, decoded.FactorCatalog, decoded.InterpretationAssets)
		if scale == nil {
			return nil, fmt.Errorf("report input v3 is missing factor catalog for kind %s", model.Kind)
		}
		payload = ScaleModelPayload{Scale: scale}
	case EvaluationModelKindTypology:
		if decoded.TypologyRouting == nil {
			return nil, fmt.Errorf("report input v3 is missing typology routing")
		}
		payload = typologyPayloadFromFreeze(ref, decoded.InterpretationAssets, decoded.TypologySource)
	case EvaluationModelKindBehavioralRating:
		if len(decoded.FactorCatalog) == 0 || decoded.Norming == nil || decoded.Norming.NormTables == nil {
			return nil, fmt.Errorf("report input v3 is missing norm catalog for kind %s", model.Kind)
		}
		payload = behavioralPayloadFromFreeze(ref, decoded.FactorCatalog, decoded.InterpretationAssets, decoded.Norming)
	case EvaluationModelKindCognitive:
		if len(decoded.FactorCatalog) == 0 {
			return nil, fmt.Errorf("report input v3 is missing factor catalog for kind %s", model.Kind)
		}
		payload = cognitivePayloadFromFreeze(ref, decoded.FactorCatalog)
	default:
		return nil, fmt.Errorf("report input v3 is unsupported for kind %s", model.Kind)
	}
	snapshot.Model = &ModelSnapshot{
		Kind: ref.Kind, SubKind: ref.SubKind, Algorithm: ref.Algorithm,
		Code: ref.Code, Version: ref.Version, Title: ref.Title, Payload: payload,
	}
	snapshot.ModelPayload = payload
	return snapshot, nil
}

func TypologyReportRoutingFromSnapshot(input *InputSnapshot) (TypologyRoutingFreeze, bool) {
	if input == nil || input.TypologyRouting == nil || input.TypologyRouting.DecisionKind == "" {
		return TypologyRoutingFreeze{}, false
	}
	return *input.TypologyRouting, true
}
