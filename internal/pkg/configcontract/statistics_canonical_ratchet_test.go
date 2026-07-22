package configcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatisticsRuntimeDoesNotReintroduceRetiredV1 protects the V2-only
// cutover. Historical migrations and tests that assert route absence are
// intentionally outside this production-source scan.
func TestStatisticsRuntimeDoesNotReintroduceRetiredV1(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"statisticsv2",
		"statistics_v2",
		"ProjectBehaviorEvent",
		"behavior_footprint",
		"assessment_episode",
		"analytics_pending_event",
		"statistics_journey_daily",
		"statistics_content_daily",
		"statistics_plan_daily",
		"behavior_journey_scan",
		"behavior_pending_reconcile",
		"version_mode",
	}

	for _, relative := range []string{"internal/apiserver", "api/grpc"} {
		base := filepath.Join(root, relative)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || strings.HasSuffix(path, "_test.go") || strings.Contains(path, string(filepath.Separator)+"docs"+string(filepath.Separator)) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, retired := range forbidden {
				if strings.Contains(text, retired) {
					t.Errorf("retired Statistics token %q found in production source %s", retired, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
