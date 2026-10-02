package aibridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func referenceArtifactEvent(t *testing.T) Event {
	t.Helper()
	raw, err := os.ReadFile("../../../pkg/contract/testdata/mbti-three-topic-artifact.json")
	if err != nil {
		t.Fatal(err)
	}
	return Event{SessionID: "00000000-0000-4000-8000-000000000001", Status: "completed", ArtifactJSON: string(raw)}
}

func TestOriginalPythonReferenceArtifactPreserved(t *testing.T) {
	event := referenceArtifactEvent(t)
	a, err := ValidateArtifact(event)
	if err != nil {
		t.Fatal(err)
	}
	var original Artifact
	if err = json.Unmarshal([]byte(event.ArtifactJSON), &original); err != nil {
		t.Fatal(err)
	}
	if *a != original || a.SchemaVersion != "qs-ai-artifact/v2" {
		t.Fatal("original Python envelope changed")
	}
	old, _ := json.Marshal(Artifact{SchemaVersion: "qs-ai-artifact/v1"})
	if strings.Contains(string(old), "reference_material") {
		t.Fatal("legacy envelope gained new fields")
	}
}

func TestReferenceIntegrityFailuresEvenWithUpdatedDigest(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"schema":           func(m map[string]any) { m["schema_version"] = "mbti-reference-selection/v2" },
		"model":            func(m map[string]any) { m["model_code"] = "OTHER" },
		"model version":    func(m map[string]any) { m["model_version"] = "latest" },
		"type":             func(m map[string]any) { m["type_code"] = "XXXX" },
		"missing topic":    func(m map[string]any) { m["entries"] = m["entries"].([]any)[:8] },
		"duplicate entry":  func(m map[string]any) { e := m["entries"].([]any); e[1] = e[0] },
		"wrong pole":       func(m map[string]any) { m["entries"].([]any)[0].(map[string]any)["pole"] = "E" },
		"missing source":   func(m map[string]any) { m["entries"].([]any)[0].(map[string]any)["source_ids"] = []string{"missing"} },
		"duplicate source": func(m map[string]any) { s := m["sources"].([]any); m["sources"] = append(s, s[0]) },
		"unsafe URL":       func(m map[string]any) { m["sources"].([]any)[0].(map[string]any)["url"] = "javascript:alert(1)" },
		"credential URL": func(m map[string]any) {
			m["sources"].([]any)[0].(map[string]any)["url"] = "https://user:password@example.invalid"
		},
		"invalid date":   func(m map[string]any) { m["sources"].([]any)[0].(map[string]any)["accessed_on"] = "2026-02-30" },
		"html":           func(m map[string]any) { m["entries"].([]any)[0].(map[string]any)["content"] = "<script>bad</script>" },
		"empty boundary": func(m map[string]any) { m["entries"].([]any)[0].(map[string]any)["usage_boundary"] = "" },
		"unknown field":  func(m map[string]any) { m["strength"] = "high" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			event := referenceArtifactEvent(t)
			var a Artifact
			if err := json.Unmarshal([]byte(event.ArtifactJSON), &a); err != nil {
				t.Fatal(err)
			}
			var material map[string]any
			if err := json.Unmarshal([]byte(a.ReferenceMaterialJSON), &material); err != nil {
				t.Fatal(err)
			}
			mutate(material)
			encoded, err := json.Marshal(material)
			if err != nil {
				t.Fatal(err)
			}
			a.ReferenceMaterialJSON = string(encoded)
			sum := sha256.Sum256(encoded)
			a.ReferenceMaterialFingerprint = "sha256:" + hex.EncodeToString(sum[:])
			raw, _ := json.Marshal(a)
			event.ArtifactJSON = string(raw)
			if _, err := ValidateArtifact(event); err == nil {
				t.Fatal("invalid references accepted")
			}
		})
	}
	for _, name := range []string{"changed body without digest", "empty", "legacy envelope", "legacy content", "unresolved reference", "wrong topic", "wrong evidence"} {
		t.Run(name, func(t *testing.T) {
			event := referenceArtifactEvent(t)
			var a Artifact
			if err := json.Unmarshal([]byte(event.ArtifactJSON), &a); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "changed body without digest":
				a.ReferenceMaterialJSON = strings.Replace(a.ReferenceMaterialJSON, "usage_boundary", "modified_boundary", 1)
			case "empty":
				a.ReferenceMaterialJSON = ""
			case "legacy envelope":
				a.SchemaVersion = "qs-ai-artifact/v1"
			case "legacy content":
				a.ContentJSON = `{"schema_version":"ai-explanation-output/v1"}`
			case "unresolved reference":
				a.ContentJSON = strings.Replace(a.ContentJSON, "reference:personality.ei.i", "reference:personality.missing", 1)
			case "wrong topic":
				a.ContentJSON = strings.Replace(a.ContentJSON, "reference:personality.ei.i", "reference:career.ei.i", 1)
			case "wrong evidence":
				var content map[string]any
				if err := json.Unmarshal([]byte(a.ContentJSON), &content); err != nil {
					t.Fatal(err)
				}
				content["sections"].([]any)[0].(map[string]any)["insights"].([]any)[0].(map[string]any)["evidence_refs"] = []map[string]string{{"kind": "dimension", "ref": "dimension:JP"}}
				encoded, _ := json.Marshal(content)
				a.ContentJSON = string(encoded)
			}
			sum := sha256.Sum256([]byte(a.ContentJSON))
			a.ContentFingerprint = "sha256:" + hex.EncodeToString(sum[:])
			raw, _ := json.Marshal(a)
			event.ArtifactJSON = string(raw)
			if _, err := ValidateArtifact(event); err == nil {
				t.Fatal("corrupt envelope or link accepted")
			}
		})
	}
}
