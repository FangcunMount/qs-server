package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// PreflightMessagingTopology only reads preconfigured NSQD. Nothing is created.
func PreflightMessagingTopology(ctx context.Context, client *http.Client, sources map[string]string) error {
	if client == nil || len(sources) == 0 {
		return errors.New("AI MQ topology unavailable")
	}
	topics := map[string]string{app.CommandsTopic: "qs-ai.commands.v1", app.EventsTopic: "qs-server.ai-events.v1", app.AcksTopic: "qs-ai.acks.v1"}
	required := map[string]string{}
	for topic, channel := range topics {
		required[topic] = channel
		required[legacy.FailedHandoffTopic(topic, channel)] = legacy.FailedHandoffChannel
	}
	for _, base := range sources {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/stats?format=json", nil)
		if err != nil {
			cancel()
			return errors.New("AI MQ topology unavailable")
		}
		response, err := client.Do(req)
		if err != nil {
			cancel()
			return errors.New("AI MQ topology unavailable")
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1048577))
		_ = response.Body.Close()
		cancel()
		if readErr != nil || response.StatusCode != http.StatusOK || len(raw) > 1048576 {
			return errors.New("AI MQ topology unavailable")
		}
		var stats struct {
			Topics []struct {
				Name     string `json:"topic_name"`
				Channels []struct {
					Name string `json:"channel_name"`
				} `json:"channels"`
			} `json:"topics"`
		}
		if json.Unmarshal(raw, &stats) != nil {
			return errors.New("AI MQ topology unavailable")
		}
		found := map[string]bool{}
		for _, t := range stats.Topics {
			for _, c := range t.Channels {
				if required[t.Name] == c.Name {
					found[t.Name] = true
				}
			}
		}
		for topic := range required {
			if !found[topic] {
				return errors.New("AI MQ topic/channel must be provisioned before startup")
			}
		}
	}
	return nil
}
