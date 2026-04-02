package iamauth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
)

const (
	// DefaultVersionTopic 与 IAM 授权版本通知主题保持一致。
	DefaultVersionTopic = "iam.authz.version"
)

var channelSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// VersionChangeMessage 对齐 IAM 版本通知载荷。
type VersionChangeMessage struct {
	TenantID string `json:"tenant_id"`
	Version  int64  `json:"version"`
}

// DefaultVersionSyncChannel 为单实例订阅生成唯一 channel，避免多副本间负载均衡掉版本通知。
func DefaultVersionSyncChannel(serviceName string) string {
	if serviceName == "" {
		serviceName = "qs-authz-sync"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	channel := fmt.Sprintf("%s-%s-%d", serviceName, host, os.Getpid())
	channel = channelSanitizer.ReplaceAllString(channel, "-")
	return strings.ToLower(channel)
}

// SubscribeVersionChanges 订阅 IAM authz_version 通知，并将版本水位推进到本地 SnapshotLoader。
func SubscribeVersionChanges(
	ctx context.Context,
	subscriber messaging.Subscriber,
	topic string,
	channel string,
	loader *SnapshotLoader,
) error {
	if subscriber == nil || loader == nil {
		return nil
	}
	if topic == "" {
		topic = DefaultVersionTopic
	}
	if channel == "" {
		channel = DefaultVersionSyncChannel("qs-authz-sync")
	}

	handler := func(msgCtx context.Context, msg *messaging.Message) error {
		var change VersionChangeMessage
		if err := json.Unmarshal(msg.Payload, &change); err != nil {
			logger.L(msgCtx).Warnw("failed to decode IAM authz version message",
				"topic", topic,
				"error", err.Error(),
			)
			return nil
		}
		if change.TenantID == "" || change.Version <= 0 {
			logger.L(msgCtx).Warnw("ignored invalid IAM authz version message",
				"topic", topic,
				"tenant_id", change.TenantID,
				"version", change.Version,
			)
			return nil
		}
		loader.ObserveTenantAuthzVersion(change.TenantID, change.Version)
		logger.L(msgCtx).Debugw("applied IAM authz version watermark",
			"topic", topic,
			"tenant_id", change.TenantID,
			"version", change.Version,
		)
		return nil
	}

	if err := subscriber.Subscribe(topic, channel, handler); err != nil {
		return err
	}
	logger.L(ctx).Infow("subscribed IAM authz version sync",
		"topic", topic,
		"channel", channel,
	)
	return nil
}
