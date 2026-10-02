package aibridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func mbtiArtifactEvent(t *testing.T, mutate func(map[string]any)) Event {
	t.Helper()
	raw, err := os.ReadFile("../../../pkg/contract/testdata/mbti-three-topic-output.json")
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(content)
	}
	raw, err = json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	event := artifactEvent(t)
	var artifact Artifact
	if err := json.Unmarshal([]byte(event.ArtifactJSON), &artifact); err != nil {
		t.Fatal(err)
	}
	artifact.ContentJSON = string(raw)
	hash := sha256.Sum256(raw)
	artifact.ContentFingerprint = "sha256:" + hex.EncodeToString(hash[:])
	artifact.OutputValidatorVersion = "qs-ai-output-mbti-three-topic/v1"
	raw, err = json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	event.ArtifactJSON = string(raw)
	return event
}

func TestMBTIThreeTopicArtifactContract(t *testing.T) {
	t.Parallel()
	event := mbtiArtifactEvent(t, nil)
	artifact, err := ValidateArtifact(event)
	if err != nil || artifact == nil {
		t.Fatalf("three-topic result rejected: %v", err)
	}
	// The result envelope remains v1; output v2 does not rewrite old envelopes.
	if artifact.SchemaVersion != "qs-ai-artifact/v1" {
		t.Fatal("artifact envelope changed")
	}
	for name, mutate := range map[string]func(map[string]any){
		"scene":             func(c map[string]any) { c["scene_contract_version"] = "mbti-single-assessment/v1" },
		"missing topic":     func(c map[string]any) { c["sections"] = c["sections"].([]any)[:2] },
		"duplicate topic":   func(c map[string]any) { c["sections"].([]any)[1].(map[string]any)["topic"] = "personality" },
		"unordered topics":  func(c map[string]any) { s := c["sections"].([]any); s[0], s[1] = s[1], s[0] },
		"extra measurement": func(c map[string]any) { c["score"] = 80 },
		"no reference": func(c map[string]any) {
			c["sections"].([]any)[0].(map[string]any)["insights"].([]any)[0].(map[string]any)["reference_refs"] = []string{}
		},
		"reference as measured fact": func(c map[string]any) {
			c["sections"].([]any)[0].(map[string]any)["insights"].([]any)[0].(map[string]any)["basis"] = "report_fact"
		},
		"invented source URL": func(c map[string]any) {
			c["sections"].([]any)[0].(map[string]any)["insights"].([]any)[0].(map[string]any)["source_url"] = "https://example.invalid"
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateArtifact(mbtiArtifactEvent(t, mutate)); err == nil {
				t.Fatal("malformed output accepted even with a valid content fingerprint")
			}
		})
	}
	event = mbtiArtifactEvent(t, nil)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(event.ArtifactJSON), &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["output_validator_version"] = "qs-ai-output/v1"
	raw, _ := json.Marshal(envelope)
	event.ArtifactJSON = string(raw)
	if _, err := ValidateArtifact(event); err == nil {
		t.Fatal("new output accepted with a legacy validator")
	}
}
