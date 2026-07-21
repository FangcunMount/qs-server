package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CanonicalContentHash fingerprints the canonical authoring layers frozen in a
// published DefinitionV2. Derived DecisionSpec / InterpretationAssets are excluded.
func CanonicalContentHash(def *Definition) (string, error) {
	if def == nil {
		return "", nil
	}
	canonical := Definition{
		Measure:     def.Measure,
		Calibration: def.Calibration,
		Execution:   def.Execution,
		Conclusions: def.Conclusions,
		Outcomes:    def.Outcomes,
		ReportMap:   def.ReportMap,
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
