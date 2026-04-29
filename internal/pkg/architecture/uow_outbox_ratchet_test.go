package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationDoesNotDependOnConcreteMySQLUnitOfWork(t *testing.T) {
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/application"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if strings.Contains(text, "*mysql.UnitOfWork") {
			t.Fatalf("%s must depend on application transaction.Runner instead of concrete *mysql.UnitOfWork", mustRel(t, root, path))
		}
	})
}

func TestOutboxStagingCompatibilityEntrypointsStayContained(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]struct{}{
		"internal/apiserver/infra/mysql/eventoutbox/store.go":          {},
		"internal/apiserver/infra/mongo/eventoutbox/store.go":          {},
		"internal/apiserver/infra/mongo/answersheet/durable_submit.go": {},
		"internal/apiserver/infra/mongo/evaluation/repo.go":            {},
	}
	walkGoFiles(t, filepath.Join(root, "internal/apiserver"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") || !strings.Contains(text, "StageEventsTx(") {
			return
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowed[rel]; !ok {
			t.Fatalf("%s must stage durable events through context-aware outbox stagers instead of StageEventsTx", rel)
		}
	})
}

func TestContainerUsesDurableOutboxRelayConstructor(t *testing.T) {
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/container/assembler"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if strings.Contains(text, "NewOutboxRelay(") {
			t.Fatalf("%s must use NewDurableOutboxRelay for durable outbox relays", mustRel(t, root, path))
		}
	})
}
