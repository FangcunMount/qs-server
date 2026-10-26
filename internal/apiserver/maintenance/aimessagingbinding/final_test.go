package aimessagingbinding

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	options "github.com/FangcunMount/qs-server/internal/apiserver/options"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/spf13/viper"
)

func finalFixture(t *testing.T, extra ...string) (map[string]any, Binding) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "configs/apiserver.prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.yaml")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", filepath.Join(root, "scripts/cd/render-api-messaging.py"), "render", path)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "MESSAGING_PROVIDER=rocketmq", "ROCKETMQ_ENDPOINT=broker.invalid:8081", "ROCKETMQ_INSTANCE_ID=qs-apiserver-1", "ROCKETMQ_MYSQL_STORE_ID=Original-$主", "ROCKETMQ_MYSQL_WRITER_EPOCH=Epoch-$$-SQL", "ROCKETMQ_MONGO_STORE_ID=Original-Mongo", "ROCKETMQ_MONGO_WRITER_EPOCH=Epoch-Mongo"}
	cmd.Env = append(cmd.Env, extra...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render: %v %s", err, out)
	}
	reader := viper.New()
	reader.SetConfigFile(path)
	if err := reader.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	settings := reader.AllSettings()
	settings["messaging"] = reader.Get("messaging")
	b := fixture()
	b.Provider = "rocketmq"
	b.NSQD = nil
	publishing := reader.GetBool("messaging.rocketmq.publish_enabled")
	b.PublishEnabled = &publishing
	b.RocketMQ = &generic.RocketMQOptions{Endpoint: "broker.invalid:8081", InstanceID: "qs-apiserver-1", CredentialsFile: "/app/configs/rocketmq/credentials.json", CAFile: "/app/configs/rocketmq/ca.crt", RoutesFile: "/app/configs/rocketmq/routes.json", Transactions: map[string]generic.TransactionOptions{
		"qs.mysql": {StoreID: "Original-$主", WriterEpoch: "Epoch-$$-SQL", RouteRevision: "m3-1", CredentialsFile: "/app/configs/rocketmq/credentials.json", Limits: &generic.TransactionLimits{Sends: 4, Branches: 16, CheckWorkers: 2, CheckQueue: 16, ReconcileBatch: 50, CheckTimeout: 2 * time.Second, RPCTimeout: 5 * time.Second, Lease: time.Minute, ReconcileInterval: 5 * time.Second}},
	}}
	return settings, b
}

func tree(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	for _, key := range keys {
		var ok bool
		m, ok = m[key].(map[string]any)
		if !ok {
			t.Fatalf("missing test subtree %s", key)
		}
	}
	return m
}

func TestFinalProductionRenderUsesActualPackagerAndHostDecoder(t *testing.T) {
	for _, admission := range []string{"false", "true"} {
		t.Run(admission, func(t *testing.T) {
			base, b := finalFixture(t, "ROCKETMQ_PUBLISH_ENABLED="+admission)
			raw, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("QS_APISERVER_MESSAGING_ROCKETMQ_ENDPOINT", "NEVER_COPY_ENV")
			result, err := RenderFinal(raw, b)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(result, []byte("NEVER_COPY_ENV")) {
				t.Fatal("ambient settings copied")
			}
			v := viper.New()
			v.SetConfigType("json")
			if err := v.ReadConfig(bytes.NewReader(result)); err != nil {
				t.Fatal(err)
			}
			opts := options.NewOptions()
			if err := app.DecodeOptions(v, opts); err != nil {
				t.Fatal(err)
			}
			root := opts.MessagingOptions.RocketMQ
			bridge := opts.AIWorkflow.Messaging.RocketMQ
			if root.Transactions["qs.mysql"].StoreID != "Original-$主" || bridge.Transactions["qs.mysql"].WriterEpoch != "Epoch-$$-SQL" {
				t.Fatal("identity bytes changed")
			}
			if root.Transactions["qs.mongo"].CredentialsFile == root.CredentialsFile {
				t.Fatal("owner principals combined")
			}
			sub := opts.IAMOptions.AuthzSync
			if sub.RocketMQ.Endpoint != root.Endpoint || sub.RocketMQ.PublishingEnabled() || len(sub.RocketMQ.Transactions) != 0 {
				t.Fatal("frozen IAM consumer differs from runtime")
			}
			if opts.MigrationOptions.Enabled || opts.Eventing.StandardOutbox.Mongo || opts.Eventing.StandardOutbox.Assessment {
				t.Fatal("legacy startup responsibilities returned")
			}
		})
	}

}

func TestFinalProductionRenderRejectsConflictsBeforeReadingFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(map[string]any, *Binding)
	}{
		{"missing-root-owner", func(m map[string]any, b *Binding) {
			delete(tree(t, m, "messaging", "rocketmq", "transactions"), "qs.mongo")
		}},
		{"different-epoch", func(m map[string]any, b *Binding) {
			x := b.RocketMQ.Transactions["qs.mysql"]
			x.WriterEpoch = "Different"
			b.RocketMQ.Transactions["qs.mysql"] = x
		}},
		{"case-changed-identity", func(m map[string]any, b *Binding) {
			x := b.RocketMQ.Transactions["qs.mysql"]
			x.StoreID = "original-$主"
			b.RocketMQ.Transactions["qs.mysql"] = x
		}},
		{"bridge-without-transaction", func(m map[string]any, b *Binding) { b.RocketMQ.Transactions = nil }},
		{"bridge-budget-expansion", func(m map[string]any, b *Binding) { b.RocketMQ.Transactions["qs.mysql"].Limits.CheckWorkers = 4 }},
		{"bridge-implicit-budget", func(m map[string]any, b *Binding) {
			x := b.RocketMQ.Transactions["qs.mysql"]
			x.Limits = nil
			b.RocketMQ.Transactions["qs.mysql"] = x
		}},
		{"root-budget-expansion", func(m map[string]any, b *Binding) {
			tree(t, m, "messaging", "rocketmq", "transactions", "qs.mongo", "limits")["sends"] = 8
		}},
		{"shared-owner-credential", func(m map[string]any, b *Binding) {
			tree(t, m, "messaging", "rocketmq", "transactions", "qs.mongo")["credentials_file"] = "/app/configs/rocketmq/credentials.json"
		}},
		{"different-endpoint", func(m map[string]any, b *Binding) { b.RocketMQ.Endpoint = "other.invalid:8081" }},
		{"open-bridge-closed-root", func(m map[string]any, b *Binding) { open := true; b.PublishEnabled = &open }},
		{"conflicting-inner-admission", func(m map[string]any, b *Binding) { open := true; b.RocketMQ.PublishEnabled = &open }},
		{"wrong-bridge-instance", func(m map[string]any, b *Binding) { b.RocketMQ.InstanceID = "extra-subscriber" }},
		{"wrong-iam-instance", func(m map[string]any, b *Binding) {
			tree(t, m, "iam", "authz-sync", "rocketmq")["instance_id"] = "extra-subscriber"
		}},
		{"iam-owner", func(m map[string]any, b *Binding) {
			tree(t, m, "iam", "authz-sync", "rocketmq")["transactions"] = tree(t, m, "messaging", "rocketmq", "transactions")
		}},
		{"iam-publisher", func(m map[string]any, b *Binding) {
			tree(t, m, "iam", "authz-sync", "rocketmq")["publish_enabled"] = true
		}},
		{"legacy-outbox", func(m map[string]any, b *Binding) { tree(t, m, "eventing", "standard_outbox")["mongo"] = true }},
		{"startup-ddl", func(m map[string]any, b *Binding) { tree(t, m, "migration")["enabled"] = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, b := finalFixture(t)
			test.edit(m, &b)
			raw, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			result, err := RenderFinal(raw, b)
			if err != ErrBinding || result != nil {
				t.Fatalf("conflict accepted: %v", err)
			}
			if strings.Contains(err.Error(), "Original") {
				t.Fatal("identity leaked")
			}
		})
	}
}

func TestTransportFileHashRejectsUnsafeFilesButAllowsReadAtime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("private-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Unix(1, 0), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if hash, err := transportFileHash(path, true); err != nil || len(hash) != 64 {
		t.Fatal("read atime must not invalidate stable bytes")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := transportFileHash(path, true); err != ErrBinding {
		t.Fatal("public credential accepted")
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	if _, err := transportFileHash(path, false); err != ErrBinding {
		t.Fatal("group writable transport accepted")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := transportFileHash(link, false); err != ErrBinding {
		t.Fatal("symlink accepted")
	}
}
