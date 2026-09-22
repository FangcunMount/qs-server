package evaluationinput

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

const MBTIPoleCatalogSchema = "mbti-pole-catalog/v1"

// MBTIPoleCatalog is presentation-safe metadata from the exact evaluation model.
// It contains neither scoring inputs nor answers, and never loads a catalog head.
type MBTIPoleCatalog struct {
	SchemaVersion string         `json:"schema_version"`
	Axes          []MBTIPoleAxis `json:"axes"`
}

type MBTIPoleAxis struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	LeftPole  string  `json:"left_pole"`
	RightPole string  `json:"right_pole"`
	MinScore  float64 `json:"min_score"`
	MaxScore  float64 `json:"max_score"`
	Threshold float64 `json:"threshold"`
}

// Required numbers must not silently become zero when a stored block is damaged.
func (a *MBTIPoleAxis) UnmarshalJSON(data []byte) error {
	type axis MBTIPoleAxis
	var value axis
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"min_score", "max_score", "threshold"} {
		var number *float64
		if err := json.Unmarshal(fields[name], &number); err != nil {
			return fmt.Errorf("invalid MBTI pole %s", name)
		}
		if number == nil {
			return fmt.Errorf("missing MBTI pole %s", name)
		}
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = MBTIPoleAxis(value)
	return nil
}

// RequiresMBTIPoleCatalog identifies the exact supported immutable MBTI model.
func RequiresMBTIPoleCatalog(model ModelRef) bool {
	return model.Kind == EvaluationModelKindTypology && model.Code == "MBTI_OEJTS" && model.Version == "v64-report-202608-v1" && model.Algorithm == string(modelcatalog.AlgorithmPersonalityTypology)
}

func (c *MBTIPoleCatalog) Validate() error {
	if c == nil || c.SchemaVersion != MBTIPoleCatalogSchema || len(c.Axes) != 4 {
		return fmt.Errorf("invalid MBTI pole catalog version or axes")
	}
	codes := []string{"EI", "SN", "TF", "JP"}
	left := []string{"I", "S", "F", "J"}
	right := []string{"E", "N", "T", "P"}
	for i, a := range c.Axes {
		if a.Code != codes[i] || a.Name == "" || a.LeftPole != left[i] || a.RightPole != right[i] {
			return fmt.Errorf("invalid MBTI pole identity or order")
		}
		for _, v := range []float64{a.MinScore, a.MaxScore, a.Threshold} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("nonfinite MBTI pole boundary")
			}
		}
		if a.MinScore >= a.Threshold || a.Threshold >= a.MaxScore {
			return fmt.Errorf("invalid MBTI pole boundary")
		}
	}
	return nil
}

func cloneMBTIPoles(c *MBTIPoleCatalog) *MBTIPoleCatalog {
	if c == nil {
		return nil
	}
	return &MBTIPoleCatalog{SchemaVersion: c.SchemaVersion, Axes: append([]MBTIPoleAxis(nil), c.Axes...)}
}
