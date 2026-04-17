package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/pkg/event"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestHandleStatisticsAssessmentSubmittedSkipsDuplicateEvents(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	handler := handleStatisticsAssessmentSubmitted(&Dependencies{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		RedisCache: client,
	})

	payload := marshalAssessmentSubmittedEvent(t, "evt-submitted-1", domainAssessment.AssessmentSubmittedData{
		OrgID:             1,
		AssessmentID:      101,
		TesteeID:          201,
		QuestionnaireCode: "PHQ9",
		SubmittedAt:       time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC),
	})

	if err := handler(context.Background(), domainAssessment.EventTypeSubmitted, payload); err != nil {
		t.Fatalf("first handler call failed: %v", err)
	}
	if err := handler(context.Background(), domainAssessment.EventTypeSubmitted, payload); err != nil {
		t.Fatalf("second handler call failed: %v", err)
	}

	count := dailyMetricCount(t, client, rediskey.NewBuilder().BuildStatsDailyKey(
		1,
		string(domainStats.StatisticTypeQuestionnaire),
		"PHQ9",
		time.Now().Format("2006-01-02"),
	), "submission_count")
	if count != 1 {
		t.Fatalf("expected duplicate event to increment only once, got %d", count)
	}
}

func TestHandleStatisticsAssessmentInterpretedSkipsDuplicateEvents(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	handler := handleStatisticsAssessmentInterpreted(&Dependencies{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		RedisCache: client,
	})

	payload := marshalAssessmentInterpretedEvent(t, "evt-interpreted-1", domainAssessment.AssessmentInterpretedData{
		OrgID:         1,
		AssessmentID:  102,
		TesteeID:      202,
		ScaleCode:     "GAD7",
		ScaleVersion:  "v1",
		RiskLevel:     "low",
		InterpretedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
	})

	if err := handler(context.Background(), domainAssessment.EventTypeInterpreted, payload); err != nil {
		t.Fatalf("first handler call failed: %v", err)
	}
	if err := handler(context.Background(), domainAssessment.EventTypeInterpreted, payload); err != nil {
		t.Fatalf("second handler call failed: %v", err)
	}

	count := dailyMetricCount(t, client, rediskey.NewBuilder().BuildStatsDailyKey(
		1,
		string(domainStats.StatisticTypeQuestionnaire),
		"GAD7",
		time.Now().Format("2006-01-02"),
	), "completion_count")
	if count != 1 {
		t.Fatalf("expected duplicate interpreted event to increment only once, got %d", count)
	}
}

func marshalAssessmentSubmittedEvent(t *testing.T, eventID string, data domainAssessment.AssessmentSubmittedData) []byte {
	t.Helper()
	evt := event.New(domainAssessment.EventTypeSubmitted, domainAssessment.AggregateType, "101", data)
	evt.ID = eventID
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal submitted event failed: %v", err)
	}
	return payload
}

func marshalAssessmentInterpretedEvent(t *testing.T, eventID string, data domainAssessment.AssessmentInterpretedData) []byte {
	t.Helper()
	evt := event.New(domainAssessment.EventTypeInterpreted, domainAssessment.AggregateType, "102", data)
	evt.ID = eventID
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal interpreted event failed: %v", err)
	}
	return payload
}

func dailyMetricCount(t *testing.T, client redis.UniversalClient, key, field string) int64 {
	t.Helper()
	value, err := client.HGet(context.Background(), key, field).Int64()
	if err != nil {
		t.Fatalf("get daily metric count failed: %v", err)
	}
	return value
}

func TestStatisticsHandlerHonorsLegacyEventProcessedKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := statisticsCache.NewStatisticsCache(client)
	handler := handleStatisticsAssessmentSubmitted(&Dependencies{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		RedisCache: client,
	})
	payload := marshalAssessmentSubmittedEvent(t, "evt-legacy", domainAssessment.AssessmentSubmittedData{
		OrgID:             1,
		AssessmentID:      103,
		TesteeID:          203,
		QuestionnaireCode: "PHQ9",
		SubmittedAt:       time.Now(),
	})

	if err := cache.MarkEventProcessed(context.Background(), "evt-legacy", time.Hour); err != nil {
		t.Fatalf("seed legacy event processed key failed: %v", err)
	}
	if err := handler(context.Background(), domainAssessment.EventTypeSubmitted, payload); err != nil {
		t.Fatalf("handler call failed: %v", err)
	}

	key := rediskey.NewBuilder().BuildStatsDailyKey(
		1,
		string(domainStats.StatisticTypeQuestionnaire),
		"PHQ9",
		time.Now().Format("2006-01-02"),
	)
	if client.Exists(context.Background(), key).Val() != 0 {
		t.Fatalf("expected legacy processed key to prevent stats increment")
	}
}
