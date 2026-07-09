package snapshot

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	scoringsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	taskperf "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance"
)

// Snapshot 是published cognitive 执行载荷 (default.v1 或 spm.v1)。
type Snapshot struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
}

type (
	FactorSnapshot        = factor.FactorSnapshot
	InterpretRuleSnapshot = factor.ScoreRangeRule
)

type definitionPayload struct {
	factor.DefinitionBody
	SPM *spmExtension `json:"spm,omitempty"`
}

type spmExtension struct {
	TimeLimitSeconds int      `json:"time_limit_seconds,omitempty"`
	ItemSetCodes     []string `json:"item_set_codes,omitempty"`
	NormTableVersion string   `json:"norm_table_version,omitempty"`
}

// ParseDefinitionPayload de编码 cognitive 载荷 body 为 运行时 快照。
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
}

// ParsePublishedPayload de编码 已发布快照 using its 载荷格式 label。
func ParsePublishedPayload(payloadFormat, modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	switch payloadFormat {
	case "", "assessmentmodel.cognitive.default.v1", "assessmentmodel.cognitive.spm.v1":
		return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
	default:
		return nil, fmt.Errorf("unsupported cognitive payload format: %s", payloadFormat)
	}
}

func parseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode cognitive payload: %w", err)
	}
	out := &Snapshot{
		Code:    modelCode,
		Version: modelVersion,
		Title:   title,
		Status:  status,
	}
	factors := factor.ParseLegacyFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.SPM != nil {
		factors = taskperf.ApplyNormMetadataToLegacyFactors(factors, taskperf.MetadataContext{
			NormTableVersion: body.SPM.NormTableVersion,
			ItemSetCodes:     append([]string(nil), body.SPM.ItemSetCodes...),
		})
	}
	out.Factors = factor.SnapshotsFromLegacyFactors(factors)
	return out, nil
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// ToScaleSnapshot 投影cognitive 因子 为 scale execution 结构。
func (s *Snapshot) ToScaleSnapshot() *scoringsnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	return scoringsnapshot.BuildFromCanonicalFactors(scoringsnapshot.ExecutionEnvelope{
		Code:                 s.Code,
		ScaleVersion:         s.Version,
		Title:                s.Title,
		QuestionnaireCode:    s.QuestionnaireCode,
		QuestionnaireVersion: s.QuestionnaireVersion,
		Status:               s.Status,
	}, s.Factors)
}
