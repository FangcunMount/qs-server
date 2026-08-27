// Package input owns the immutable server-side input snapshot used by every
// attempt of one AI explanation generation.
package input

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

type Snapshot struct {
	schemaVersion string
	canonicalJSON []byte
	fingerprint   aiexplanation.Fingerprint
}

func NewSnapshot(canonicalJSON []byte) (Snapshot, error) {
	return RestoreSnapshot(aiexplanation.InputSchemaVersionV1, canonicalJSON, aiexplanation.NewFingerprint(canonicalJSON))
}

func RestoreSnapshot(schemaVersion string, canonicalJSON []byte, fingerprint aiexplanation.Fingerprint) (Snapshot, error) {
	if schemaVersion != aiexplanation.InputSchemaVersionV1 {
		return Snapshot{}, fmt.Errorf("unsupported AI explanation input schema version: %s", schemaVersion)
	}
	canonicalJSON = bytes.TrimSpace(canonicalJSON)
	if len(canonicalJSON) == 0 || canonicalJSON[0] != '{' || !json.Valid(canonicalJSON) {
		return Snapshot{}, fmt.Errorf("AI explanation input snapshot must be one JSON object")
	}
	if err := fingerprint.Validate(); err != nil {
		return Snapshot{}, err
	}
	if fingerprint != aiexplanation.NewFingerprint(canonicalJSON) {
		return Snapshot{}, fmt.Errorf("AI explanation input snapshot fingerprint mismatch")
	}
	return Snapshot{
		schemaVersion: schemaVersion,
		canonicalJSON: append([]byte(nil), canonicalJSON...),
		fingerprint:   fingerprint,
	}, nil
}

func (s Snapshot) Validate() error {
	_, err := RestoreSnapshot(s.schemaVersion, s.canonicalJSON, s.fingerprint)
	return err
}

func (s Snapshot) SchemaVersion() string { return s.schemaVersion }

func (s Snapshot) CanonicalJSON() []byte { return append([]byte(nil), s.canonicalJSON...) }

func (s Snapshot) Fingerprint() aiexplanation.Fingerprint { return s.fingerprint }
