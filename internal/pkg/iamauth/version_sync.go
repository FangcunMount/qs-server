package iamauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

const (
	// DefaultVersionTopic 与 IAM 授权版本通知主题保持一致。
	DefaultVersionTopic = "iam.authz.version.v2"
)

var channelSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// VersionChangeMessage 对齐 IAM 版本通知载荷。
type VersionChangeMessage struct {
	Version int64 `json:"version"`
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

// EphemeralVersionSyncChannel keeps a per-process NSQ subscription within the
// protocol's 64-byte channel limit. Missed notifications are safe only when
// the independent committed-version guard is enabled.
func EphemeralVersionSyncChannel(serviceName string) string {
	const suffix = "#ephemeral"
	channel := DefaultVersionSyncChannel(serviceName)
	if len(channel)+len(suffix) > 64 {
		digest := sha256.Sum256([]byte(channel))
		channel = channel[:64-len(suffix)-13] + "-" + hex.EncodeToString(digest[:6])
	}
	return channel + suffix
}

type SDKVersionSubscriber interface {
	Subscribe(string, string, rmtransport.Handler) error
}

// SubscribeVersionChangesSDK consumes IAM version notifications as native SDK
// deliveries. The committed-version guard remains the independent freshness
// authority when NSQ is disconnected; malformed notifications are ACKed as in
// the prior path and cannot advance the watermark.
func SubscribeVersionChangesSDK(
	ctx context.Context,
	subscriber SDKVersionSubscriber,
	topic, channel string,
	loader *SnapshotLoader,
) error {
	if subscriber == nil || loader == nil {
		return nil
	}
	topic, channel = versionSyncRoute(topic, channel)
	if err := subscriber.Subscribe(topic, channel, func(msgCtx context.Context, delivery rmtransport.Delivery) error {
		if delivery == nil {
			return fmt.Errorf("IAM authz version delivery is nil")
		}
		applyVersionChange(msgCtx, topic, loader, delivery.Message().Payload)
		return delivery.Ack()
	}); err != nil {
		return err
	}
	logger.L(ctx).Infow("subscribed IAM authz version sync",
		"topic", topic,
		"channel", channel,
	)
	return nil
}

func versionSyncRoute(topic, channel string) (string, string) {
	if topic == "" {
		topic = DefaultVersionTopic
	}
	if channel == "" {
		channel = DefaultVersionSyncChannel("qs-authz-sync")
	}
	return topic, channel
}

func applyVersionChange(ctx context.Context, topic string, loader *SnapshotLoader, payload []byte) {
	var change VersionChangeMessage
	if err := json.Unmarshal(payload, &change); err != nil {
		logger.L(ctx).Warnw("failed to decode IAM authz version message", "topic", topic, "error", err.Error())
		return
	}
	if change.Version <= 0 {
		logger.L(ctx).Warnw("ignored invalid IAM authz version message", "topic", topic, "version", change.Version)
		return
	}
	loader.ObserveAuthzVersion(change.Version)
	logger.L(ctx).Debugw("applied IAM authz version watermark", "topic", topic, "version", change.Version)
}
