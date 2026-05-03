package eventruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestEventSystemDoesNotImportRemovedEventConfig(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if strings.Contains(text, removedEventConfigImportPath()) {
			t.Fatalf("%s must use eventcatalog/eventruntime instead of removed eventconfig", rel)
		}
		if strings.Contains(text, workerApplicationImportPath()) {
			t.Fatalf("%s must use worker integration eventing instead of removed worker/application", rel)
		}
		if strings.HasPrefix(rel, "internal/worker/integration/messaging/") &&
			strings.Contains(text, workerContainerImportPath()) {
			t.Fatalf("%s must depend on narrow messaging interfaces instead of worker/container", rel)
		}
		if strings.HasPrefix(rel, "internal/worker/integration/messaging/") &&
			strings.Contains(text, workerHandlersImportPath()) {
			t.Fatalf("%s must use eventcodec instead of worker/handlers", rel)
		}
		if !strings.HasPrefix(rel, "internal/worker/handlers/") &&
			usesLegacyWorkerHandlerRegistry(text) {
			t.Fatalf("%s must use an explicit handlers.Registry instead of global handler registry helpers", rel)
		}
		if strings.HasPrefix(rel, "internal/worker/handlers/") &&
			!strings.HasSuffix(rel, "_test.go") &&
			usesInitRegisteredWorkerHandler(text) {
			t.Fatalf("%s must use the explicit handler catalog instead of init-time registration", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

func TestDurableOutboxEventsAreNotDirectPublished(t *testing.T) {
	root := repoRoot(t)
	catalog := loadEventCatalog(t)
	durableTokens := durableOutboxEventTokens(t, catalog)
	allowedDirectPublishFiles := bestEffortDirectPublishFiles()

	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if rel == "internal/apiserver/application/eventing/publish.go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if !strings.Contains(text, "PublishCollectedEvents(") {
			return nil
		}
		if _, ok := allowedDirectPublishFiles[rel]; !ok {
			t.Fatalf("%s uses PublishCollectedEvents; direct publish is only allowed for reviewed best-effort application paths", rel)
		}
		for eventType, tokens := range durableTokens {
			for _, token := range tokens {
				if strings.Contains(text, token) {
					t.Fatalf("%s direct-publishes durable_outbox event %q via token %q; stage durable events through outbox", rel, eventType, token)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

func durableOutboxEventTokens(t *testing.T, catalog *eventcatalog.Catalog) map[string][]string {
	t.Helper()
	tokens := map[string][]string{
		eventcatalog.AnswerSheetSubmitted: {
			eventcatalog.AnswerSheetSubmitted,
			"AnswerSheetSubmitted",
			"NewAnswerSheetSubmittedEvent",
		},
		eventcatalog.AssessmentSubmitted: {
			eventcatalog.AssessmentSubmitted,
			"AssessmentSubmitted",
			"NewAssessmentSubmittedEvent",
		},
		eventcatalog.AssessmentInterpreted: {
			eventcatalog.AssessmentInterpreted,
			"AssessmentInterpreted",
			"NewAssessmentInterpretedEvent",
		},
		eventcatalog.AssessmentFailed: {
			eventcatalog.AssessmentFailed,
			"AssessmentFailed",
			"NewAssessmentFailedEvent",
		},
		eventcatalog.ReportGenerated: {
			eventcatalog.ReportGenerated,
			"ReportGenerated",
			"NewReportGeneratedEvent",
		},
		eventcatalog.FootprintEntryOpened: {
			eventcatalog.FootprintEntryOpened,
			"FootprintEntryOpened",
			"NewFootprintEntryOpenedEvent",
		},
		eventcatalog.FootprintIntakeConfirmed: {
			eventcatalog.FootprintIntakeConfirmed,
			"FootprintIntakeConfirmed",
			"NewFootprintIntakeConfirmedEvent",
		},
		eventcatalog.FootprintTesteeProfileCreated: {
			eventcatalog.FootprintTesteeProfileCreated,
			"FootprintTesteeProfileCreated",
			"NewFootprintTesteeProfileCreatedEvent",
		},
		eventcatalog.FootprintCareRelationshipEstablished: {
			eventcatalog.FootprintCareRelationshipEstablished,
			"FootprintCareRelationshipEstablished",
			"NewFootprintCareRelationshipEstablishedEvent",
		},
		eventcatalog.FootprintCareRelationshipTransferred: {
			eventcatalog.FootprintCareRelationshipTransferred,
			"FootprintCareRelationshipTransferred",
			"NewFootprintCareRelationshipTransferredEvent",
		},
		eventcatalog.FootprintAnswerSheetSubmitted: {
			eventcatalog.FootprintAnswerSheetSubmitted,
			"FootprintAnswerSheetSubmitted",
			"NewFootprintAnswerSheetSubmittedEvent",
		},
		eventcatalog.FootprintAssessmentCreated: {
			eventcatalog.FootprintAssessmentCreated,
			"FootprintAssessmentCreated",
			"NewFootprintAssessmentCreatedEvent",
		},
		eventcatalog.FootprintReportGenerated: {
			eventcatalog.FootprintReportGenerated,
			"FootprintReportGenerated",
			"NewFootprintReportGeneratedEvent",
		},
	}

	cfg := catalog.Config()
	if cfg == nil {
		t.Fatalf("catalog config is nil")
	}
	for eventType := range cfg.Events {
		if !catalog.IsDurableOutbox(eventType) {
			continue
		}
		if len(tokens[eventType]) == 0 {
			t.Fatalf("durable_outbox event %q is missing architecture scan tokens", eventType)
		}
	}
	for eventType := range tokens {
		if !catalog.IsDurableOutbox(eventType) {
			t.Fatalf("architecture scan token %q is not configured as durable_outbox", eventType)
		}
	}
	return tokens
}

func bestEffortDirectPublishFiles() map[string]struct{} {
	return map[string]struct{}{
		"internal/apiserver/application/plan/enrollment_service.go":                {},
		"internal/apiserver/application/plan/lifecycle_service.go":                 {},
		"internal/apiserver/application/plan/lifecycle_transition_workflow.go":     {},
		"internal/apiserver/application/plan/task_management_service.go":           {},
		"internal/apiserver/application/plan/task_scheduler_service.go":            {},
		"internal/apiserver/application/scale/factor_service.go":                   {},
		"internal/apiserver/application/scale/lifecycle_service.go":                {},
		"internal/apiserver/application/survey/questionnaire/lifecycle_service.go": {},
	}
}

func removedEventConfigImportPath() string {
	return "github.com/FangcunMount/qs-server/internal/pkg/" + "eventconfig"
}

func workerContainerImportPath() string {
	return "github.com/FangcunMount/qs-server/internal/worker/" + "container"
}

func workerApplicationImportPath() string {
	return "github.com/FangcunMount/qs-server/internal/worker/" + "application"
}

func workerHandlersImportPath() string {
	return "github.com/FangcunMount/qs-server/internal/worker/" + "handlers"
}

func usesLegacyWorkerHandlerRegistry(text string) bool {
	legacyCalls := []string{
		"handlers." + "GetFactory(",
		"handlers." + "ListRegistered(",
		"handlers." + "CreateAll(",
	}
	for _, call := range legacyCalls {
		if strings.Contains(text, call) {
			return true
		}
	}
	return false
}

func usesInitRegisteredWorkerHandler(text string) bool {
	return strings.Contains(text, "func init(") || strings.Contains(text, "Register(")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "../../.."))
}

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("rel %q: %v", path, err)
	}
	return rel
}
