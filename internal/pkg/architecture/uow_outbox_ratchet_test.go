package architecture

import (
	"os"
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

func TestStatisticsApplicationDoesNotDependOnMySQLInfrastructure(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"gorm.io/gorm",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql",
		"github.com/FangcunMount/qs-server/internal/pkg/database/mysql",
	}
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/application/statistics"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, importPath := range forbidden {
			if strings.Contains(text, importPath) {
				t.Fatalf("%s must depend on statistics application ports instead of %s", mustRel(t, root, path), importPath)
			}
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

func TestAssessmentEventfulSaveCompatibilityEntrypointsStayContained(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]struct{}{
		"internal/apiserver/domain/evaluation/assessment/repository.go":      {},
		"internal/apiserver/infra/cache/assessment_detail_cache.go":          {},
		"internal/apiserver/infra/mysql/evaluation/assessment_repository.go": {},
	}
	walkGoFiles(t, filepath.Join(root, "internal/apiserver"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") || (!strings.Contains(text, "SaveWithEvents(") && !strings.Contains(text, "SaveWithAdditionalEvents(")) {
			return
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowed[rel]; !ok {
			t.Fatalf("%s must use application UoW + outbox stager instead of eventful repository save compatibility methods", rel)
		}
	})
}

func TestEvaluationAssemblerWiresAssessmentTransactionalOutbox(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal/apiserver/container/assembler/evaluation.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evaluation assembler: %v", err)
	}
	text := string(data)
	required := []string{
		"engine.WithTransactionalOutbox(txRunner, assessmentOutboxStore)",
		"assessmentApp.NewSubmissionServiceWithTransactionalOutbox(",
		"assessmentApp.NewManagementServiceWithTransactionalOutbox(",
	}
	for _, token := range required {
		if !strings.Contains(text, token) {
			t.Fatalf("evaluation production assembler must wire assessment transactional outbox with %q", token)
		}
	}
}

func TestMongoReportEventfulSaveCompatibilityEntrypointsStayContained(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]struct{}{
		"internal/apiserver/application/evaluation/engine/pipeline/report_durable_saver.go": {},
		"internal/apiserver/domain/evaluation/report/repository.go":                         {},
		"internal/apiserver/infra/mongo/evaluation/repo.go":                                 {},
	}
	walkGoFiles(t, filepath.Join(root, "internal/apiserver"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") || !strings.Contains(text, "SaveWithTesteeAndEvents(") {
			return
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowed[rel]; !ok {
			t.Fatalf("%s must use ReportDurableSaver instead of calling SaveWithTesteeAndEvents directly", rel)
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
