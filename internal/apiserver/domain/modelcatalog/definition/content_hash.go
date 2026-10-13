package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
)

// CanonicalContentHash fingerprints the canonical authoring layers frozen in a
// published DefinitionV2. Derived DecisionSpec / InterpretationAssets are excluded.
func CanonicalContentHash(def *Definition) (string, error) {
	if def == nil {
		return "", nil
	}
	data, err := canonicalContentBytesV1(def)
	if err != nil {
		// Keep the prior JSON error type/message while encoding through the fixed v1 DTO.
		return "", &json.MarshalerError{Type: reflect.TypeOf(Definition{}), Err: err}
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
