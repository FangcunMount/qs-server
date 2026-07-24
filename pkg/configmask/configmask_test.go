package configmask

import (
	"strings"
	"testing"
)

func TestSanitizeMasksNestedSensitiveFields(t *testing.T) {
	input := map[string]interface{}{
		"mysql": map[string]interface{}{
			"password": "secret-password",
		},
		"jwt": map[string]interface{}{
			"key": "jwt-signing-key",
		},
		"delegated-subject": map[string]interface{}{
			"current-key":  "delegated-current-key",
			"previous-key": "delegated-previous-key",
		},
		"secure": map[string]interface{}{
			"tls": map[string]interface{}{
				"key-file": "/etc/ssl/private/server.key",
			},
		},
	}

	sanitized := Sanitize(input).(map[string]interface{})
	mysql := sanitized["mysql"].(map[string]interface{})
	jwt := sanitized["jwt"].(map[string]interface{})
	delegatedSubject := sanitized["delegated-subject"].(map[string]interface{})
	secure := sanitized["secure"].(map[string]interface{})
	tls := secure["tls"].(map[string]interface{})

	if mysql["password"] == "secret-password" {
		t.Fatalf("expected mysql password to be masked")
	}
	if jwt["key"] == "jwt-signing-key" {
		t.Fatalf("expected jwt key to be masked")
	}
	if delegatedSubject["current-key"] == "delegated-current-key" {
		t.Fatalf("expected delegated current key to be masked")
	}
	if delegatedSubject["previous-key"] == "delegated-previous-key" {
		t.Fatalf("expected delegated previous key to be masked")
	}
	if tls["key-file"] != "/etc/ssl/private/server.key" {
		t.Fatalf("expected tls key-file path to remain visible, got %v", tls["key-file"])
	}
}

func TestMaskEnvValue(t *testing.T) {
	if got := MaskEnvValue("QS_APISERVER_MONGODB_PASSWORD", "super-secret"); got == "super-secret" {
		t.Fatalf("expected env password to be masked")
	}

	if got := MaskEnvValue("QS_APISERVER_MONGODB_HOST", "mongo:27017"); got != "mongo:27017" {
		t.Fatalf("expected non-sensitive env value to stay unchanged, got %q", got)
	}
	if got := MaskEnvValue("QS_APISERVER_DELEGATED_SUBJECT_CURRENT_KEY", "delegated-current-key"); got == "delegated-current-key" {
		t.Fatalf("expected delegated current key env value to be masked")
	}
}

func TestStringProducesMaskedJSON(t *testing.T) {
	output := String(map[string]interface{}{
		"redis": map[string]interface{}{
			"password": "redis-password",
		},
	})

	if strings.Contains(output, "redis-password") {
		t.Fatalf("expected masked output, got %s", output)
	}
	if !strings.Contains(output, "\"password\"") {
		t.Fatalf("expected json output, got %s", output)
	}
}
