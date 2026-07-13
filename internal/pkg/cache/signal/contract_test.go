package cachesignal

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCacheSignalWireContract(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, time.July, 13, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		signal interface {
			SignalName() string
			SignalKey() string
		}
		wantName string
		wantKey  string
		wantJSON string
	}{
		{
			name: "questionnaire", signal: QuestionnaireCacheChangedSignal{
				Code: "q-1", Version: "v2", Action: "published", OccurredAt: occurredAt,
			},
			wantName: SignalNameQuestionnaireCacheChanged, wantKey: "q-1",
			wantJSON: `{"code":"q-1","version":"v2","action":"published","occurred_at":"2026-07-13T10:30:00Z"}`,
		},
		{
			name: "scale", signal: ScaleCacheChangedSignal{
				Code: "scale-1", Action: "archived", OccurredAt: occurredAt,
			},
			wantName: SignalNameScaleCacheChanged, wantKey: "scale-1",
			wantJSON: `{"code":"scale-1","action":"archived","occurred_at":"2026-07-13T10:30:00Z"}`,
		},
		{
			name: "typology", signal: TypologyModelCacheChangedSignal{
				Code: "mbti", Action: "published", OccurredAt: occurredAt,
			},
			wantName: SignalNameTypologyModelCacheChanged, wantKey: "mbti",
			wantJSON: `{"code":"mbti","action":"published","occurred_at":"2026-07-13T10:30:00Z"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.signal.SignalName(); got != tc.wantName {
				t.Fatalf("SignalName() = %q, want %q", got, tc.wantName)
			}
			if got := tc.signal.SignalKey(); got != tc.wantKey {
				t.Fatalf("SignalKey() = %q, want %q", got, tc.wantKey)
			}
			payload, err := json.Marshal(tc.signal)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if got := string(payload); got != tc.wantJSON {
				t.Fatalf("wire JSON = %s, want %s", got, tc.wantJSON)
			}
		})
	}
}
