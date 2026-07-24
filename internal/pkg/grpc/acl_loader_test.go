package grpc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseACLConfigFileRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name: "unknown field",
			content: `default_policy: deny
unknown: true
services: []
`,
			wantError: "field unknown not found",
		},
		{
			name: "invalid policy",
			content: `default_policy: allow
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Method]
`,
			wantError: "default_policy must be",
		},
		{
			name: "empty rules",
			content: `default_policy: deny
services: []
`,
			wantError: "at least one service rule",
		},
		{
			name: "duplicate identity",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Method]
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Other]
`,
			wantError: "service_name \"svc.svc\" is duplicated",
		},
		{
			name: "wildcard method",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/*]
`,
			wantError: "invalid allowed_methods",
		},
		{
			name: "malformed full method",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Method/Extra]
`,
			wantError: "invalid allowed_methods",
		},
		{
			name: "duplicate method",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods:
      - /pkg.Service/Method
      - /pkg.Service/Method
`,
			wantError: "duplicate method entry",
		},
		{
			name: "method duplicated across allow and deny",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Method]
    denied_methods: [/pkg.Service/Method]
`,
			wantError: "duplicate method entry",
		},
		{
			name: "disabled rule with wildcard",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Method]
  - service_name: disabled.svc
    enabled: false
    allowed_methods: [/pkg.Service/*]
`,
			wantError: "invalid allowed_methods",
		},
		{
			name: "enabled rule without allow list",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: []
`,
			wantError: "must allow at least one method",
		},
		{
			name: "multiple documents",
			content: `default_policy: deny
services:
  - service_name: svc.svc
    enabled: true
    allowed_methods: [/pkg.Service/Method]
---
default_policy: deny
services:
  - service_name: other.svc
    enabled: true
    allowed_methods: [/pkg.Service/Other]
`,
			wantError: "exactly one YAML document",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "acl.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write ACL fixture: %v", err)
			}
			_, err := parseACLConfigFile(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("parseACLConfigFile() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestNewServerFailsWhenEnabledACLDoesNotLoad(t *testing.T) {
	t.Parallel()

	cfg := NewConfig()
	cfg.ACL.Enabled = true
	cfg.ACL.ConfigFile = filepath.Join(t.TempDir(), "missing.yaml")
	cfg.ACL.DefaultPolicy = "deny"

	server, err := NewServer(cfg, nil)
	if err == nil {
		if server != nil {
			server.Stop()
		}
		t.Fatal("NewServer() error = nil, want ACL startup failure")
	}
	if !strings.Contains(err.Error(), "initialize gRPC ACL") {
		t.Fatalf("NewServer() error = %v, want ACL initialization context", err)
	}
}

func TestNewServerBuildsWithProductionACL(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	cfg := NewConfig()
	cfg.ACL.Enabled = true
	cfg.ACL.ConfigFile = filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "configs", "grpc-acl.prod.yaml"))
	cfg.ACL.DefaultPolicy = "deny"

	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer() with production ACL error = %v", err)
	}
	server.Stop()
}
