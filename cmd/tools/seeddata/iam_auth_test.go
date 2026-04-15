package main

import (
	"encoding/json"
	"testing"
)

func TestIAMLoginRequest_MarshalTenantIDAsNumber(t *testing.T) {
	credBytes, err := json.Marshal(iamLoginCredentials{
		Username: "seed-admin",
		Password: "secret",
		TenantID: 1,
	})
	if err != nil {
		t.Fatalf("marshal iam credentials: %v", err)
	}

	reqBody, err := json.Marshal(iamLoginRequest{
		Method:      "password",
		Credentials: credBytes,
		DeviceID:    "seeddata",
	})
	if err != nil {
		t.Fatalf("marshal iam login request: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(reqBody, &payload); err != nil {
		t.Fatalf("unmarshal iam login request: %v", err)
	}

	creds, ok := payload["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected credentials object, got %#v", payload["credentials"])
	}

	if _, ok := creds["tenant_id"].(float64); !ok {
		t.Fatalf("expected tenant_id to marshal as number, got %#v", creds["tenant_id"])
	}
	if got := creds["tenant_id"].(float64); got != 1 {
		t.Fatalf("expected tenant_id=1, got %v", got)
	}
}
