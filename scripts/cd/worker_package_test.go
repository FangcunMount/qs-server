package cd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	worker "github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/spf13/viper"
)

func workerPackage(t *testing.T, dir string, extra ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("bash", "scripts/cd/prepare-package.sh")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "SERVICE=worker",
		"DEPLOY_PACKAGE_DIR=" + dir, "DEPLOY_PACKAGE=" + dir + ".tar.gz",
		"MESSAGING_PROVIDER=rocketmq", "ROCKETMQ_ENDPOINT=broker.invalid:8081",
		"ROCKETMQ_PUBLISH_ENABLED=false", "ROCKETMQ_WORKER_BINDING_REVISION=m4-worker-fixture",
		"MYSQL_TLS_MODE=VERIFY_IDENTITY", "MYSQL_HOST=mysql.invalid", "MYSQL_USERNAME=synthetic",
		"MYSQL_PASSWORD=synthetic", "MYSQL_DATABASE=qs", "MONGODB_HOST=mongo.invalid",
		"MONGODB_USERNAME=synthetic", "MONGODB_PASSWORD=synthetic", "MONGODB_DBNAME=qs",
		"REDIS_HOST=redis.invalid", "REDIS_PORT=6379",
	}, extra...)
	return cmd.CombinedOutput()
}

func TestWorkerPackageIsFinalConsumerOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "package")
	out, err := workerPackage(t, dir)
	if err != nil {
		t.Fatalf("package: %v %s", err, out)
	}
	reader := viper.New()
	reader.SetConfigFile(filepath.Join(dir, "configs/worker.prod.yaml"))
	if err := reader.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	reader.SetEnvPrefix("QS_WORKER")
	reader.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	reader.AutomaticEnv()
	body, err := os.ReadFile(filepath.Join(dir, "configs/env/config.prod.env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "QS_WORKER_") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				t.Setenv(parts[0], parts[1])
			}
		}
	}
	opts := worker.NewOptions()
	if err := app.DecodeOptions(reader, opts); err != nil {
		t.Fatal(err)
	}
	if opts.Messaging.Provider != "rocketmq" || opts.Messaging.RocketMQ.PublishingEnabled() || len(opts.Messaging.RocketMQ.Transactions) != 0 {
		t.Fatal("worker gained publisher or transaction-owner responsibility")
	}
	if opts.Messaging.RocketMQ.Endpoint != "broker.invalid:8081" || opts.Messaging.RocketMQ.CredentialsFile != "/etc/qs-worker/rocketmq/credentials.json" {
		t.Fatal("effective worker files or endpoint differ")
	}
	if opts.Worker.Concurrency != 48 || opts.Worker.ServiceName != "qs-worker" {
		t.Fatal("worker budget or logical subscription changed")
	}
	if opts.Worker.EventConfigPath != "/app/configs/events.yaml" {
		t.Fatal("legacy event catalog selected")
	}
	cfg, err := eventcatalog.Load(filepath.Join(dir, "configs/events.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "1.0" {
		t.Fatal("current business catalog changed")
	}
	// The current transaction subscriber still uses this catalog's historical
	// reliable-delivery class; it does not grant a worker an Outbox/tx sender.
	reliable := 0
	for _, ev := range cfg.Events {
		if string(ev.Delivery) == "durable_outbox" {
			reliable++
		}
	}
	if reliable != 9 {
		t.Fatal("current reliable business event set changed")
	}
	if strings.Contains(string(body), "NSQ_") {
		t.Fatal("retired worker endpoint injected")
	}
	if !strings.Contains(string(body), "ROCKETMQ_WORKER_BINDING_REVISION=m4-worker-fixture\n") {
		t.Fatal("missing immutable files revision")
	}
}

func TestWorkerPackageRejectsPublisherAndNSQBeforeSideEffects(t *testing.T) {
	for _, input := range []string{"MESSAGING_PROVIDER=nsq", "ROCKETMQ_PUBLISH_ENABLED=true", "ROCKETMQ_WORKER_BINDING_REVISION=", "ROCKETMQ_WORKER_BINDING_REVISION=../bad"} {
		t.Run(input, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "keep")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(dir, "sentinel")
			if err := os.WriteFile(sentinel, []byte("retain"), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := workerPackage(t, dir, input)
			if err == nil {
				t.Fatal("invalid worker production selection accepted")
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Fatalf("invalid worker selection removed existing package: %v %s", err, out)
			}
		})
	}
}
