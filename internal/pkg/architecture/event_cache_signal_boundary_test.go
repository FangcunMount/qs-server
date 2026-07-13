package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovedEventAndCacheSignalPathsDoNotReturn(t *testing.T) {
	root := repoRoot(t)
	removedDirectories := []string{
		"internal/pkg/eventcatalog",
		"internal/pkg/eventruntime",
		"internal/pkg/eventobservability",
		"internal/pkg/eventpayload",
		"internal/pkg/eventoutcome",
		"internal/pkg/eventcodec",
		"internal/pkg/cachesignal",
		"pkg/event",
	}
	for _, rel := range removedDirectories {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		walkGoFiles(t, path, func(file, _ string) {
			t.Fatalf("removed path contains Go forwarding file: %s", mustRel(t, root, file))
		})
	}

	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventcatalog",
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventruntime",
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventobservability",
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventpayload",
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventoutcome",
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventcodec",
		"github.com/FangcunMount/qs-server/internal/pkg/" + "cachesignal",
		"github.com/FangcunMount/qs-server/pkg/" + "event",
	}
	for _, rel := range []string{"internal", "pkg"} {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		walkGoFiles(t, path, func(file, text string) {
			if strings.HasSuffix(file, "_test.go") {
				return
			}
			for _, forbidden := range forbiddenImports {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s imports removed path %s", mustRel(t, root, file), forbidden)
				}
			}
		})
	}
}

func TestEventingSharedPackagesKeepDependencyDirection(t *testing.T) {
	root := repoRoot(t)
	processImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver",
		"github.com/FangcunMount/qs-server/internal/collection-server",
		"github.com/FangcunMount/qs-server/internal/worker",
	}
	for _, rel := range []string{
		"internal/pkg/eventing/catalog",
		"internal/pkg/eventing/runtime",
		"internal/pkg/eventing/observe",
		"internal/pkg/eventing/payload",
		"internal/pkg/eventing/outcome",
	} {
		walkGoFiles(t, filepath.Join(root, filepath.FromSlash(rel)), func(file, text string) {
			if strings.HasSuffix(file, "_test.go") {
				return
			}
			for _, forbidden := range processImports {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s imports process package %s", mustRel(t, root, file), forbidden)
				}
			}
		})
	}

	walkGoFiles(t, filepath.Join(root, "internal", "pkg", "eventing", "catalog"), func(file, text string) {
		if strings.Contains(text, "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime") {
			t.Fatalf("%s makes catalog depend on runtime", mustRel(t, root, file))
		}
	})
	for _, rel := range []string{"observe", "payload", "outcome"} {
		walkGoFiles(t, filepath.Join(root, "internal", "pkg", "eventing", rel), func(file, text string) {
			if strings.Contains(text, "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime") {
				t.Fatalf("%s makes leaf contract package depend on runtime", mustRel(t, root, file))
			}
		})
	}
}

func TestCacheSignalContractStaysTransportAgnostic(t *testing.T) {
	root := repoRoot(t)
	forbiddenImports := []string{
		"github.com/FangcunMount/component-base/pkg/signaling",
		"github.com/FangcunMount/qs-server/internal/pkg/redisruntime",
		"github.com/FangcunMount/qs-server/internal/pkg/options",
		"github.com/FangcunMount/qs-server/internal/pkg/reportstatus",
		"github.com/FangcunMount/qs-server/internal/apiserver",
		"github.com/FangcunMount/qs-server/internal/collection-server",
		"github.com/prometheus/client_golang",
		"github.com/redis/go-redis",
	}
	walkGoFiles(t, filepath.Join(root, "internal", "pkg", "cache", "signal"), func(file, text string) {
		if strings.HasSuffix(file, "_test.go") {
			return
		}
		for _, forbidden := range forbiddenImports {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s imports signal transport/runtime package %s", mustRel(t, root, file), forbidden)
			}
		}
	})

	forbiddenTokens := []string{"ConfigFrom" + "ReportStatus", "As" + "StandaloneClient"}
	walkGoFiles(t, filepath.Join(root, "internal"), func(file, text string) {
		if strings.HasSuffix(file, "_test.go") {
			return
		}
		for _, token := range forbiddenTokens {
			if strings.Contains(text, token) {
				t.Fatalf("%s reintroduces removed signal adapter %s", mustRel(t, root, file), token)
			}
		}
	})
}
