package main

import (
	"context"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"testing"
)

type snapshotStub struct {
	snapshot *authz.Snapshot
	err      error
	subject  string
}

func (s *snapshotStub) LoadFresh(_ context.Context, subject string) (*authz.Snapshot, error) {
	s.subject = subject
	return s.snapshot, s.err
}
func TestMaintenanceAuthorizationRequiresFreshWildcard(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot *authz.Snapshot
		err      error
		allowed  bool
	}{
		{name: "missing"},
		{name: "IAM failure", err: fmt.Errorf("unavailable")},
		{name: "role name only", snapshot: &authz.Snapshot{AuthzVersion: 1, DirectRoles: []string{"qs:admin"}}},
		{name: "legacy conditional", snapshot: &authz.Snapshot{AuthzVersion: 1, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeObjectCheckRequired}}}},
		{name: "unversioned", snapshot: &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}},
		{name: "current admin", allowed: true, snapshot: &authz.Snapshot{AuthzVersion: 12, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loader := &snapshotStub{snapshot: tc.snapshot, err: tc.err}
			ctx, err := authorizeMaintenance(context.Background(), loader, 9)
			if (err == nil) != tc.allowed {
				t.Fatalf("authorization: %v", err)
			}
			if loader.subject != "9" {
				t.Fatal("wrong audited user")
			}
			if tc.allowed {
				if snapshot, ok := authz.FromContext(ctx); !ok || snapshot != tc.snapshot {
					t.Fatal("authoritative snapshot lost")
				}
			}
		})
	}
}
func TestMaintenanceWritesRequireMTLSConfiguration(t *testing.T) {
	values := map[string]string{"TESTEE_STORE_IAM_ADDRESS": "iam:9090", "TESTEE_STORE_IAM_CA": "ca.pem", "TESTEE_STORE_IAM_CERT": "client.pem", "TESTEE_STORE_IAM_KEY": "client.key"}
	get := func(key string) string { return values[key] }
	options, err := maintenanceIAMOptions(get)
	if err != nil || !options.GRPC.TLS.Enabled {
		t.Fatal("valid mTLS rejected")
	}
	for key, value := range values {
		delete(values, key)
		if _, err := maintenanceIAMOptions(get); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
		values[key] = value
	}
}
