package personalitykind

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

const LegacyPersonalityKind = "personality"

// AssessmentRewrite describes a safe Assessment/Outcome kind normalization.
type AssessmentRewrite struct {
	Eligible      bool
	Reason        string
	FromKind      string
	ToKind        string
	ToSubKind     string
	KeepAlgorithm string
}

// EvaluateAssessmentPersonalityKindRewrite reports whether rewriting
// evaluation_model_kind/model_kind from personality → typology is safe.
//
// Does NOT rewrite algorithm (mbti/sbti/bigfive remain; dual-identity covers them).
// Empty algorithm is eligible for kind rewrite only (empty fill is a separate gate).
func EvaluateAssessmentPersonalityKindRewrite(kind, algorithm string) AssessmentRewrite {
	out := AssessmentRewrite{
		FromKind:      kind,
		ToKind:        string(modelcatalog.KindTypology),
		ToSubKind:     string(modelcatalog.SubKindTypology),
		KeepAlgorithm: algorithm,
	}
	if kind != LegacyPersonalityKind {
		out.Reason = "not_legacy_personality_kind"
		return out
	}
	if algorithm != "" && !isTypologyAlgorithm(algorithm) {
		out.Reason = "non_typology_algorithm"
		return out
	}
	out.Eligible = true
	out.Reason = "personality_kind_to_typology"
	return out
}
