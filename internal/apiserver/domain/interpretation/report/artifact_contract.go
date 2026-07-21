package report

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// CrossMechanismArtifactContract validates content that every successful report
// must satisfy regardless of builder mechanism.
func CrossMechanismArtifactContract(content Content) error {
	if isContentEmpty(content) {
		return fmt.Errorf("report content must not be empty")
	}
	if !modelIdentityComplete(content.Model) {
		return fmt.Errorf("report model identity must include kind, code and version")
	}
	if err := validateModelExtraKind(content.Model, content.ModelExtra); err != nil {
		return err
	}
	return nil
}

// BuilderSpecificDraftContract validates mechanism-specific minimum content.
// It must not assume every report has a total score or clinical conclusion.
func BuilderSpecificDraftContract(builderIdentity string, content Content) error {
	switch builderIdentity {
	case BuilderIdentityFactorScoring:
		if content.PrimaryScore == nil {
			return fmt.Errorf("factor scoring report requires primary score")
		}
		if len(content.Dimensions) == 0 {
			return fmt.Errorf("factor scoring report requires dimensions")
		}
	case BuilderIdentityNormProfile:
		if content.PrimaryScore == nil {
			return fmt.Errorf("norm profile report requires primary score")
		}
		if len(content.Dimensions) == 0 {
			return fmt.Errorf("norm profile report requires dimensions")
		}
		if !hasNormReference(content.Dimensions) {
			return fmt.Errorf("norm profile report requires norm reference on at least one dimension")
		}
	case BuilderIdentityTypology:
		if content.ModelExtra == nil || content.ModelExtra.IsEmpty() {
			return fmt.Errorf("typology report requires model extra")
		}
		if content.ModelExtra.TypeCode == "" && content.ModelExtra.TypeName == "" {
			return fmt.Errorf("typology report requires type identity in model extra")
		}
		if len(content.Dimensions) == 0 {
			return fmt.Errorf("typology report requires dimensions")
		}
	case BuilderIdentityTaskPerformance:
		if content.PrimaryScore == nil && !hasAbilityDimension(content.Dimensions) {
			return fmt.Errorf("task performance report requires primary score or ability dimensions")
		}
	default:
		return fmt.Errorf("unsupported builder identity %q", builderIdentity)
	}
	return nil
}

func isContentEmpty(content Content) bool {
	if !content.Model.IsEmpty() {
		return false
	}
	if content.PrimaryScore != nil || content.Level != nil {
		return false
	}
	if content.Conclusion != "" {
		return false
	}
	if len(content.Dimensions) > 0 || len(content.Suggestions) > 0 {
		return false
	}
	if content.ModelExtra != nil && !content.ModelExtra.IsEmpty() {
		return false
	}
	return true
}

func modelIdentityComplete(model ModelIdentity) bool {
	return model.Kind != "" && model.Code != "" && model.Version != ""
}

func validateModelExtraKind(model ModelIdentity, extra *ModelExtra) error {
	if extra == nil || extra.IsEmpty() {
		return nil
	}
	switch model.Kind {
	case string(modelcatalog.KindTypology):
		if extra.Kind != "personality_type" && extra.Kind != "trait_profile" {
			return fmt.Errorf("model extra kind %q does not match typology model", extra.Kind)
		}
	case string(modelcatalog.KindScale), string(modelcatalog.KindBehavioralRating), string(modelcatalog.KindCognitive):
		if extra.Kind == "personality_type" || extra.Kind == "trait_profile" {
			return fmt.Errorf("model extra kind %q does not match %s model", extra.Kind, model.Kind)
		}
	}
	return nil
}

func hasNormReference(dimensions []DimensionInterpret) bool {
	for _, dimension := range dimensions {
		if dimension.NormReference() != nil {
			return true
		}
	}
	return false
}

func hasAbilityDimension(dimensions []DimensionInterpret) bool {
	for _, dimension := range dimensions {
		if dimension.Kind() == DimensionKindAbility {
			return true
		}
	}
	return false
}
