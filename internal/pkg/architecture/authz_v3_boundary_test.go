package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthzV3OnlyBoundaryDoesNotRegress(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"api/grpc/iam/authz/v2",
		"authzv2",
		"legacy_scoped",
		"shadow_v3",
		"enforce_v3",
		"CasbinDomain",
		"authz-casbin-domain",
	}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				t.Fatalf("%s contains retired AuthZ dependency or mode %q", mustRel(t, root, path), token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}

	assertArchitectureFileContains(t, root, "internal/pkg/iamauth/snapshot_loader.go", "api/grpc/iam/authz/v3")
	assertArchitectureFileContains(t, root, "internal/pkg/iamauth/object_checker.go", "api/grpc/iam/authz/v3")
	assertArchitectureFileContains(t, root, "internal/apiserver/application/authz/snapshot.go", "AuthorizationModeObjectCheckRequired")
}

func TestRetiredAuthzCutoverEntrypointsStayAbsent(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		".github/workflows/authz-consumer-control.yml",
		"scripts/cd/authz-consumer-control.sh",
		"scripts/cd/test-authz-consumer-control.sh",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			if err != nil {
				t.Fatal(err)
			}
			t.Fatalf("retired AuthZ cutover entrypoint exists: %s", rel)
		}
	}
	for _, rel := range []string{
		".github/actionlint.yaml",
		".github/workflows/cd.yml",
		".github/workflows/ci.yml",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"AUTHZ_CUTOVER_AUTO_DEPLOY_PAUSED",
			"authz-consumer-control",
		} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("%s contains retired AuthZ cutover token %q", rel, forbidden)
			}
		}
	}
}

func assertArchitectureFileContains(t *testing.T, root, rel, token string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if !strings.Contains(string(data), token) {
		t.Fatalf("%s is missing required AuthZ v3 token %q", rel, token)
	}
}
