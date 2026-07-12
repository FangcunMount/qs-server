//go:build refactor_target

package interpretation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetInterpretationApplicationIsOrganizedByActors(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	applicationRoot := filepath.Join(root, "internal", "apiserver", "application", "interpretation")
	for _, actor := range []string{"automation", "participant", "administration", "clinician", "operations"} {
		info, err := os.Stat(filepath.Join(applicationRoot, actor))
		if err != nil || !info.IsDir() {
			t.Fatalf("missing Interpretation actor application package %q", actor)
		}
	}

	for _, mechanismPackage := range []string{"generation", "input", "reporting"} {
		if _, err := os.Stat(filepath.Join(applicationRoot, mechanismPackage)); err == nil {
			t.Fatalf("Interpretation application must not expose mechanism package %q", mechanismPackage)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestTargetInterpretationQueryAndAuditDebtDoesNotReturn(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "apiserver", "application", "interpretation", "automation", "input", "preview_outcome_adapter.go")); err == nil {
		t.Fatal("preview outcome adapter must remain test-only")
	}
	checks := map[string][]string{
		filepath.Join(root, "internal", "apiserver", "infra", "mongo", "interpretation", "artifact_read_model.go"): {"interpret_reports", "mergeCurrentAndArchivedReportRows", "listReportsFromStore", "listArchives("},
		filepath.Join(root, "internal", "apiserver", "application", "interpretation", "operations", "service.go"):  {"Permissions []string", "PermissionAudit"},
	}
	for path, forbidden := range checks {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				t.Fatalf("forbidden Interpretation debt %q remains in %s", token, path)
			}
		}
	}
}

func TestTargetInterpretationApplicationHasNoActorNeutralFacades(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "internal", "apiserver", "application", "interpretation"),
		filepath.Join(root, "internal", "apiserver", "container", "modules", "interpretation"),
		filepath.Join(root, "internal", "apiserver", "transport", "grpc", "service"),
		filepath.Join(root, "internal", "apiserver", "container", "modules", "modelcatalog", "preview"),
	}
	forbidden := []string{
		"OutcomeReportService",
		"ReportQueryService",
		"LifecycleQueryService",
		"GenerateByAssessmentID(",
		"FromLegacyOutcome(",
		"ExpandAudienceProfileBuilders(",
	}
	for _, dir := range paths {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, token := range forbidden {
				if strings.Contains(string(data), token) {
					t.Fatalf("actor-neutral Interpretation application capability %q remains in %s", token, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTargetProductionDoesNotReferenceLegacyInterpretReports(t *testing.T) {
	root := repoRoot(t)
	productionRoot := filepath.Join(root, "internal")
	err := filepath.WalkDir(productionRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "interpret_reports") {
			t.Fatalf("production legacy report collection reference remains in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTargetStatisticsResolvesCurrentReportOnlyThroughCatalog(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "infra", "statistics", "report_scan_source.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "report_query_catalog") {
		t.Fatal("statistics report scan must use report_query_catalog")
	}
	for _, forbidden := range []string{"interpret_report_artifacts", "archived_reports", "loadCurrentReportMeta", "loadArchivedReportMeta"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("statistics report scan bypasses catalog through %q", forbidden)
		}
	}
}

func TestTargetParticipantReportGRPCDoesNotLeakUnknownErrors(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "internal", "apiserver", "transport", "grpc", "service", "participant_report.go"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "status.Error(codes.Internal, err.Error())") {
			t.Fatalf("participant report gRPC can leak an internal error through %s", path)
		}
	}
}

func TestTargetOnlyActiveProfileLinkMiddlewareRemains(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "collection-server", "transport", "rest", "middleware", "iam_middleware.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"func ProfileLinkMiddleware(", "func OptionalProfileLinkMiddleware("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("unused profile-link middleware returned: %s", forbidden)
		}
	}
	if !strings.Contains(text, "func TesteeProfileLinkMiddleware(") {
		t.Fatal("active testee profile-link middleware is missing")
	}
}
