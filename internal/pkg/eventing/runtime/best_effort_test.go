package eventruntime

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/stretchr/testify/require"
)

func TestBestEffortPublisherRejectsDurableEventsSharingTheSameLogicalTopic(t *testing.T) {
	wire := &capturedWirePublisher{}
	publisher, err := NewBestEffortPublisher(loadEventCatalog(t), wire)
	require.NoError(t, err)
	for _, kind := range []string{eventcatalog.QuestionnaireChanged, eventcatalog.AssessmentModelChanged, eventcatalog.TaskOpened, eventcatalog.TaskCompleted, eventcatalog.TaskExpired, eventcatalog.TaskCanceled} {
		require.NoError(t, publisher.Publish(t.Context(), event.New(kind, "fixture", "1", map[string]string{"id": "1"})))
	}
	require.Len(t, wire.bodies, 6)
	for _, kind := range []string{eventcatalog.TaskOpenedReminderRequested, eventcatalog.EvaluationRequested, eventcatalog.AnswerSheetSubmitted, "unknown"} {
		require.Error(t, publisher.Publish(t.Context(), event.New(kind, "fixture", "1", map[string]string{"id": "1"})))
	}
	require.Len(t, wire.bodies, 6, "durable event must not reach the NORMAL wire port")
	_, err = NewBestEffortPublisher(nil, wire)
	require.Error(t, err)
	_, err = NewBestEffortPublisher(loadEventCatalog(t), nil)
	require.Error(t, err)
}

func TestBestEffortTopicsComeFromTheActualCatalogAndExcludeTransactionOnlyTopics(t *testing.T) {
	topics, err := BestEffortTopics(loadEventCatalog(t))
	require.NoError(t, err)
	require.Equal(t, []string{"qs.plan.task", "qs.survey.lifecycle"}, topics)
	_, err = BestEffortTopics(nil)
	require.Error(t, err)
	_, err = BestEffortTopics(eventcatalog.NewCatalog(nil))
	require.Error(t, err)
}
