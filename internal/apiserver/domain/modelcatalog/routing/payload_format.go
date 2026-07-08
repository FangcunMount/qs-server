package routing

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

const (
	// v2 production 载荷格式。
	PayloadFormatAssessmentScaleV1         = "assessmentmodel.scale.v1"
	PayloadFormatPersonalityTypologyV1     = "assessmentmodel.personality.typology.v1"
	PayloadFormatBehavioralRatingDefaultV1 = "assessmentmodel.behavioral_rating.default.v1"
	// PayloadFormatBehavioralRatingBrief2V1 is legacy-decode-only; new drafts use DefaultV1.
	PayloadFormatBehavioralRatingBrief2V1 = "assessmentmodel.behavioral_rating.brief2.v1"
	PayloadFormatCognitiveDefaultV1       = "assessmentmodel.cognitive.default.v1"
	// PayloadFormatCognitiveSPMV1 is legacy-decode-only; new drafts use DefaultV1.
	PayloadFormatCognitiveSPMV1 = "assessmentmodel.cognitive.spm.v1"

	// Legacy read-only payload formats (migration / outbox drain).
	PayloadFormatScaleV1 = "ruleset.scale.v1"
	// Deprecated: legacy-decode-only MBTI format; production draft uses personality.typology.v1.
	PayloadFormatMBTIV1 = "ruleset.mbti.v1"
	// Deprecated: legacy-decode-only SBTI format; production draft uses personality.typology.v1.
	PayloadFormatSBTIV1 = "ruleset.sbti.v1"

	PayloadFormatScaleV1Legacy = "evaluationinput.scale.v1"
	PayloadFormatMBTIV1Legacy  = "evaluationinput.mbti.v1"
	PayloadFormatSBTIV1Legacy  = "evaluationinput.sbti.v1"
)

func IsScalePayloadFormat(format string) bool {
	switch format {
	case PayloadFormatAssessmentScaleV1,
		PayloadFormatScaleV1, PayloadFormatScaleV1Legacy:
		return true
	default:
		return false
	}
}

// IsMBTIPayloadFormat 报告旧版 MBTI 载荷格式 仅。
// v2 类型学载荷 必须 be distinguished 按 算法From类型学载荷。
func IsMBTIPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatMBTIV1, PayloadFormatMBTIV1Legacy:
		return true
	default:
		return false
	}
}

// IsSBTIPayloadFormat 报告旧版 SBTI 载荷格式 仅。
// v2 类型学载荷 必须 be distinguished 按 算法From类型学载荷。
func IsSBTIPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatSBTIV1, PayloadFormatSBTIV1Legacy:
		return true
	default:
		return false
	}
}

func IsPersonalityTypologyPayloadFormat(format string) bool {
	return format == PayloadFormatPersonalityTypologyV1
}

type typologyAlgorithmEnvelope struct {
	Algorithm identity.Algorithm `json:"algorithm"`
}

// AlgorithmFromTypologyPayload reads 算法 身份 从 v2 类型学载荷。
func AlgorithmFromTypologyPayload(payload []byte) (identity.Algorithm, error) {
	var envelope typologyAlgorithmEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("decode typology payload algorithm: %w", err)
	}
	if envelope.Algorithm == "" {
		return "", fmt.Errorf("typology payload algorithm is empty")
	}
	return envelope.Algorithm, nil
}

// PayloadFormatForBehavioralRating returns the family draft/publish format (norming mechanism).
// Algorithm is ignored; legacy brief2.v1 remains decodable via IsBehavioralRatingPayloadFormat.
func PayloadFormatForBehavioralRating(_ identity.Algorithm) string {
	return PayloadFormatBehavioralRatingDefaultV1
}

// PayloadFormatForCognitive returns the family draft/publish format (task_performance mechanism).
// Algorithm is ignored; legacy spm.v1 remains decodable via IsCognitivePayloadFormat.
func PayloadFormatForCognitive(_ identity.Algorithm) string {
	return PayloadFormatCognitiveDefaultV1
}

// IsBehavioralRatingPayloadFormat 报告是否 格式 是 supported behavioral_rating 载荷。
func IsBehavioralRatingPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatBehavioralRatingDefaultV1, PayloadFormatBehavioralRatingBrief2V1:
		return true
	default:
		return false
	}
}

// IsCognitivePayloadFormat 报告是否 格式 是 supported cognitive 载荷。
func IsCognitivePayloadFormat(format string) bool {
	switch format {
	case PayloadFormatCognitiveDefaultV1, PayloadFormatCognitiveSPMV1:
		return true
	default:
		return false
	}
}

// DraftPayloadFormatForModel 返回draft/publish 载荷格式 用于 模型家族 和 算法。
func DraftPayloadFormatForModel(kind identity.Kind, algorithm identity.Algorithm) string {
	switch kind {
	case identity.KindBehavioralRating:
		return PayloadFormatForBehavioralRating(algorithm)
	case identity.KindCognitive:
		return PayloadFormatForCognitive(algorithm)
	default:
		return ""
	}
}
