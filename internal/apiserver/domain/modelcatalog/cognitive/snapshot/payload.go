package snapshot

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	taskperf "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/task_performance"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

// Snapshot is a published cognitive execution payload (default.v1 or spm.v1).
type Snapshot struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
	SPM                  *SPMProfile
}

// SPMProfile carries SPM-specific configuration beyond score_range scoring.
type SPMProfile struct {
	TimeLimitSeconds int
	ItemSetCodes     []string
	NormTableVersion string
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

// ParseDefinitionPayload decodes a cognitive payload body into a runtime snapshot.
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
}

// ParsePublishedPayload decodes a published snapshot using its payload format label.
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
	factors := factor.ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.SPM != nil {
		factors = taskperf.ApplyNormMetadata(factors, taskperf.MetadataContext{
			NormTableVersion: body.SPM.NormTableVersion,
			ItemSetCodes:     append([]string(nil), body.SPM.ItemSetCodes...),
		})
	}
	out.Factors = factors
	if body.SPM != nil {
		out.SPM = &SPMProfile{
			TimeLimitSeconds: body.SPM.TimeLimitSeconds,
			ItemSetCodes:     append([]string(nil), body.SPM.ItemSetCodes...),
			NormTableVersion: body.SPM.NormTableVersion,
		}
	}
	return out, nil
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// ToScaleSnapshot projects cognitive factors into the scale execution shape.
func (s *Snapshot) ToScaleSnapshot() *scalesnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	return scalesnapshot.BuildFromModelFactors(
		s.Code, s.Version, s.Title, s.QuestionnaireCode, s.QuestionnaireVersion, s.Status, s.Factors,
	)
}
