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

func TestOutboxStagingCompatibilityEntrypointsAreRemoved(t *testing.T) {
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/apiserver"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") || !strings.Contains(text, "StageEventsTx(") {
			return
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		t.Fatalf("%s must stage durable events through context-aware outbox stagers instead of StageEventsTx", rel)
	})
}

func TestRemovedEventCompatibilitySymbolsDoNotReturn(t *testing.T) {
	root := repoRoot(t)
	checks := map[string][]string{
		"internal/pkg/eventing/catalog/types.go": {
			"AssessmentSubmitted", "AssessmentEvaluated", "AssessmentInterpreted",
			"AssessmentInterpretedOutcome", "AssessmentFailed", "\n\tReportGenerated ", "ReportGeneratedOutcome",
		},
		"internal/apiserver/infra/mongo/eventoutbox/store.go": {
			"func NewStore(", "func WithPriorityEventTypes(", "StageEventsTx(",
		},
		"internal/apiserver/infra/mysql/eventoutbox/store.go": {
			"func NewStore(", "func WithPriorityEventTypes(", "StageEventsTx(",
		},
		"internal/apiserver/application/eventing/outbox.go": {
			"func NewOutboxRelay(", "func NewDurableOutboxRelay(",
		},
		"internal/apiserver/application/eventing/post_commit.go": {
			"NewReadyIndexPostCommitDispatcher", "readyIndexPostCommitDispatcher",
		},
	}
	for rel, forbidden := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(data)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s reintroduces removed event compatibility symbol %q", rel, token)
			}
		}
	}
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
	path := filepath.Join(root, "internal", "apiserver", "container", "modules", "evaluation", "assemble.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evaluation assembler: %v", err)
	}
	text := string(data)
	required := []string{
		"execute.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore)",
		"evaluationintake.NewService(",
		"evaluationoperator.NewRecoveryService(",
	}
	for _, token := range required {
		if !strings.Contains(text, token) {
			t.Fatalf("evaluation production assembler must wire assessment transactional outbox with %q", token)
		}
	}
}

func TestSurveyAssemblerUsesTransactionalSubmissionDurableStore(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "modules", "survey", "assemble.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read survey assembler: %v", err)
	}
	text := string(data)
	required := []string{
		"asApp.NewTransactionalSubmissionDurableStore(",
		"asApp.NewSubmissionService(repo, durableStore, questionnaireRepo, batchValidator, reader)",
		"profile.Stager",
		"profile.PostCommit",
	}
	for _, token := range required {
		if !strings.Contains(text, token) {
			t.Fatalf("survey production assembler must wire transactional submission durable store with %q", token)
		}
	}
	if strings.Contains(text, "asApp.NewSubmissionService(sub.Repo, baseRepo, quesRepo, batchValidator)") {
		t.Fatalf("survey production assembler must not pass repository-owned durable store to submission service")
	}
	if strings.Contains(text, "NewProjectionHook") || strings.Contains(text, "NewStoreWithTopicResolver") {
		t.Fatalf("survey assembler must not own publish hooks or outbox stores")
	}
}

func TestMongoReportEventfulSaveCompatibilityEntrypointsAreRemoved(t *testing.T) {
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/apiserver"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") || !strings.Contains(text, "SaveWithTesteeAndEvents(") {
			return
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		t.Fatalf("%s still contains SaveWithTesteeAndEvents; report persistence must use ReportDurableSaver", rel)
	})
}

func TestInterpretationAssemblerExclusivelyWiresExecution(t *testing.T) {
	root := repoRoot(t)
	evalPath := filepath.Join(root, "internal", "apiserver", "container", "modules", "evaluation", "assemble.go")
	evalData, err := os.ReadFile(evalPath)
	if err != nil {
		t.Fatalf("read evaluation assembler: %v", err)
	}
	evalText := string(evalData)
	for _, token := range []string{
		"normalized.ReportDurableSaver",
		"normalized.ReportBuilderRegistry",
	} {
		if strings.Contains(evalText, token) {
			t.Fatalf("evaluation assembler must not receive report write capability %q", token)
		}
	}

	reportPath := filepath.Join(root, "internal", "apiserver", "container", "modules", "interpretation", "assemble.go")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report assembler: %v", err)
	}
	reportText := string(reportData)
	for _, token := range []string{
		"interpretationexecution.NewStarter(",
		"interpretationexecution.NewExecutor(",
		"interpretationautomation.NewService(",
	} {
		if !strings.Contains(reportText, token) {
			t.Fatalf("interpretation module must own report write orchestration %q", token)
		}
	}
}

func TestBusinessModulesDoNotOwnEventTransportRuntime(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"NewOutboxRelayWithOptions(",
		"NewDurableOutboxRelay",
		"NewReconciler(",
		"NewImmediateDispatcher(",
		"NewStoreWithTopicResolver(",
		"BeforePublishHooks:",
	}
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/container/modules"), func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s must obtain a narrow event profile from EventSubsystem instead of owning %q", mustRel(t, root, path), token)
			}
		}
	})
}

func TestAnswerSheetRepositoryDoesNotProxyOutbox(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "internal/apiserver/infra/mongo/answersheet")
	forbidden := []string{"outboxStore", "ClaimDueEvents(", "MarkEventPublished(", "OutboxStatusSnapshot(", "NewRepositoryWithTopicResolver("}
	walkGoFiles(t, dir, func(path string, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s still exposes repository-owned outbox compatibility token %q", mustRel(t, root, path), token)
			}
		}
	})
}
