package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const ProjectionHashSchemaV1 = "definition-projection/v1"

// CanonicalContentHash fingerprints authoring layers that drive runtime projection.
// Derived DecisionSpec / InterpretationAssets are excluded (MC-R017).
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

// PayloadProjectionHash fingerprints published compatibility payload bytes.
func PayloadProjectionHash(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
