package aimessagingbinding

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/FangcunMount/reliable-messaging/transport"
)

// FinalTransportFiles validates the actual local credential, CA and v2 route
// bytes. Load/TransactionConfig do not start a broker client or database pool.
func FinalTransportFiles(config []byte) (map[string]string, error) {
	o, err := finalOptions(config)
	if err != nil {
		return nil, ErrBinding
	}
	root := o.MessagingOptions.RocketMQ
	files, err := (Binding{Provider: "rocketmq", RocketMQ: root}).TransportFiles()
	if err != nil {
		return nil, ErrBinding
	}
	mongoHash, err := transportFileHash(root.Transactions["qs.mongo"].CredentialsFile, true)
	if err != nil {
		return nil, ErrBinding
	}
	files["mongo-credentials.json"] = mongoHash
	loaded, err := root.Load(nil, 4, 5*time.Second)
	if err != nil {
		return nil, ErrBinding
	}
	for owner, topics := range map[string][]string{"qs.mysql": {"qs.evaluation.lifecycle", "qs.plan.task", "qs.ai.commands.v1", "qs.ai.acks.v1"}, "qs.mongo": {"qs.evaluation.lifecycle"}} {
		if _, err := root.TransactionConfig(owner, topics); err != nil {
			return nil, ErrBinding
		}
	}
	wanted := map[string]string{"qs.ai.commands.v1": "qs.mysql", "qs.ai.acks.v1": "qs.mysql", "qs.ai.events.v1": "ai.mysql", "iam.authz.version.v2": "iam.mysql"}
	seen := map[string]bool{}
	for _, route := range loaded.Routes {
		owner, ok := wanted[route.Topic]
		if !ok {
			continue
		}
		if route.MessageType != transport.TransactionMessageType || route.Owner != owner || route.RouteRevision != "m3-1" {
			return nil, ErrBinding
		}
		seen[route.Topic] = true
	}
	for topic := range wanted {
		if !seen[topic] {
			return nil, ErrBinding
		}
	}
	return files, nil
}

func transportFileHash(path string, private bool) (string, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > 1<<20 || before.Mode().Perm()&0022 != 0 || (private && before.Mode().Perm()&0007 != 0) {
		return "", ErrBinding
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) < 1 || len(body) > 1<<20 {
		return "", ErrBinding
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return "", ErrBinding
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
