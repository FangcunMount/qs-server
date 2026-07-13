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

func TestValidateRawSectionAcceptsHyphenatedFlagKeys(t *testing.T) {
	schema := FieldSchema{
		"capabilities": {
			"catalog": {
				"questionnaire": {
					"enabled": nil, "ttl_seconds": nil, "ttl_jitter_ratio": nil,
					"max_entries": nil, "singleflight": nil, "signal_evict_enabled": nil,
				},
			},
			"actor": {"testee": {"enabled": nil, "ttl": nil, "negative_ttl": nil, "negative": nil}},
		},
	}
	err := ValidateRawSection(map[string]any{
		"cache": map[string]any{
			"capabilities": map[string]any{
				"catalog": map[string]any{
					"questionnaire": map[string]any{
						"enabled": true, "ttl-seconds": 180, "ttl-jitter-ratio": 0.1,
						"max-entries": 256, "singleflight": true, "signal-evict-enabled": true,
					},
				},
				"actor": map[string]any{
					"testee": map[string]any{"enabled": true, "ttl": "30m", "negative-ttl": "5m", "negative": true},
				},
			},
		},
	}, "cache", schema)
	if err != nil {
		t.Fatalf("hyphenated flag keys error = %v", err)
	}
}
