package definition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalContentHashV1GoldenCorpus(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"scale", "typology", "behavioral_rating", "cognitive"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("testdata", "hash_v1", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Definition     *Definition `json:"definition"`
				CanonicalBytes string      `json:"canonical_bytes"`
				CanonicalHash  string      `json:"canonical_hash"`
			}
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			actual, err := CanonicalContentHash(fixture.Definition)
			if err != nil {
				t.Fatal(err)
			}
			if actual != fixture.CanonicalHash {
				t.Fatalf("persisted v1 hash changed: got %s want %s", actual, fixture.CanonicalHash)
			}
			sum := sha256.Sum256([]byte(fixture.CanonicalBytes))
			if hex.EncodeToString(sum[:]) != fixture.CanonicalHash {
				t.Fatal("golden bytes do not match captured digest")
			}
			encoded, err := canonicalContentBytesV1(fixture.Definition)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, []byte(fixture.CanonicalBytes)) {
				t.Fatal("v1 byte ordering, nil/empty values, escaping or zero derived markers changed")
			}
		})
	}
}
