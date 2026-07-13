package options

import "testing"

func TestValidateRawSectionRejectsUnknownNestedField(t *testing.T) {
	err := ValidateRawSection(map[string]any{
		"cache": map[string]any{"capabilities": map[string]any{"unknown": true}},
	}, "cache", FieldSchema{"capabilities": {"known": nil}})
	if err == nil {
		t.Fatal("unknown nested field was accepted")
	}
}

func TestValidateRawSectionAcceptsKnownFields(t *testing.T) {
	err := ValidateRawSection(map[string]any{
		"cache": map[string]any{"capabilities": map[string]any{"known": true}},
	}, "cache", FieldSchema{"capabilities": {"known": nil}})
	if err != nil {
		t.Fatalf("known nested field error = %v", err)
	}
}
