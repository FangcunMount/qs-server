package cd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/spf13/viper"
)

func apiPackage(t *testing.T, dir string, extra ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("bash", "scripts/cd/prepare-package.sh")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "SERVICE=apiserver",
		"DEPLOY_PACKAGE_DIR=" + dir, "DEPLOY_PACKAGE=" + dir + ".tar.gz",
		"MESSAGING_PROVIDER=rocketmq", "ROCKETMQ_ENDPOINT=broker.invalid:8081",
		"ROCKETMQ_INSTANCE_ID=qs-apiserver-1",
		"ROCKETMQ_MYSQL_STORE_ID=Original-MySQL-$主", "ROCKETMQ_MYSQL_WRITER_EPOCH=Writer-$$-SQL",
		"ROCKETMQ_MONGO_STORE_ID=Original-Mongo", "ROCKETMQ_MONGO_WRITER_EPOCH=Original-Mongo-Epoch",
		"MYSQL_TLS_MODE=VERIFY_IDENTITY", "MYSQL_HOST=mysql.invalid", "MYSQL_USERNAME=synthetic",
		"MYSQL_PASSWORD=synthetic", "MYSQL_DATABASE=qs", "MONGODB_HOST=mongo.invalid",
		"MONGODB_USERNAME=synthetic", "MONGODB_PASSWORD=synthetic", "MONGODB_DBNAME=qs",
		"REDIS_HOST=redis.invalid", "REDIS_PORT=6379", "JWT_SECRET=synthetic",
		"OSS_ACCESS_KEY_ID=synthetic", "OSS_ACCESS_KEY_SECRET=synthetic", "DELEGATED_SUBJECT_CURRENT_KEY=synthetic",
	}, extra...)
	return cmd.CombinedOutput()
}

func TestAPIPackagePreservesFinalDualOwnerAndIAMSubscription(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "package")
	out, err := apiPackage(t, dir)
	if err != nil {
		t.Fatalf("package: %v %s", err, out)
	}
	reader := viper.New()
	reader.SetConfigFile(filepath.Join(dir, "configs/apiserver.prod.yaml"))
	if err := reader.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	reader.SetEnvPrefix("QS_APISERVER")
	reader.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	reader.AutomaticEnv()
	envBody, err := os.ReadFile(filepath.Join(dir, "configs/env/config.prod.env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(envBody), "\n") {
		if strings.HasPrefix(line, "QS_APISERVER_") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				t.Setenv(parts[0], parts[1])
			}
		}
	}
	opts := api.NewOptions()
	if err := app.DecodeOptions(reader, opts); err != nil {
		t.Fatal(err)
	}
	mq := opts.MessagingOptions.RocketMQ
	if opts.MessagingOptions.Provider != "rocketmq" || mq == nil || len(mq.Transactions) != 2 || mq.PublishingEnabled() {
		t.Fatal("final dual-owner binding or closed default admission missing")
	}
	for owner, identity := range map[string][2]string{
		"qs.mysql": {"Original-MySQL-$主", "Writer-$$-SQL"},
		"qs.mongo": {"Original-Mongo", "Original-Mongo-Epoch"},
	} {
		binding := mq.Transactions[owner]
		if binding.StoreID != identity[0] || binding.WriterEpoch != identity[1] {
			t.Fatal("original transaction identity bytes changed")
		}
		limits, err := binding.ProtocolLimits(owner)
		if err != nil || limits.Sends != 4 || limits.CheckWorkers != 2 || limits.CheckQueue != 16 || limits.ReconcileBatch != 50 || limits.Branches != 16 || limits.CheckTimeout != 2*time.Second || limits.RPCTimeout != 5*time.Second || limits.Lease != time.Minute || limits.ReconcileInterval != 5*time.Second {
			t.Fatal("original protocol budget changed")
		}
	}
	if mq.Transactions["qs.mysql"].CredentialsFile != mq.CredentialsFile || mq.Transactions["qs.mongo"].CredentialsFile == mq.CredentialsFile {
		t.Fatal("owner credentials must remain separate")
	}
	sync := opts.IAMOptions.AuthzSync
	if sync.Provider != "rocketmq" || sync.EphemeralNSQ || sync.ChannelPrefix != "qs-authz-sync" || sync.RocketMQ.InstanceID != "qs-apiserver-1" || sync.RocketMQ.Endpoint != "broker.invalid:8081" {
		t.Fatal("IAM fanout identity changed")
	}
	if sync.RocketMQ.CredentialsFile != mq.CredentialsFile || len(sync.RocketMQ.Transactions) != 0 {
		t.Fatal("IAM subscription gained an owner or lost its original principal")
	}
	if opts.MigrationOptions.Enabled || opts.Eventing.StandardOutbox.Mongo || opts.Eventing.StandardOutbox.Assessment {
		t.Fatal("production startup gained DDL or publishing Outbox")
	}
	body, err := os.ReadFile(filepath.Join(dir, "configs/env/config.prod.env"))
	if err != nil || strings.Contains(string(body), "NSQ_") {
		t.Fatal("retired NSQ endpoint injected")
	}
}

func TestAPIPackageRejectsIncompleteIdentityBeforeReplacement(t *testing.T) {
	for _, input := range []string{"MESSAGING_PROVIDER=nsq", "MESSAGING_PROVIDER=", "ROCKETMQ_MYSQL_STORE_ID=", "ROCKETMQ_MYSQL_WRITER_EPOCH=", "ROCKETMQ_MONGO_STORE_ID=", "ROCKETMQ_MONGO_WRITER_EPOCH=", "ROCKETMQ_MYSQL_STORE_ID=" + strings.Repeat("主", 43), "ROCKETMQ_INSTANCE_ID=new-fanout", "ROCKETMQ_PUBLISH_ENABLED=yes"} {
		t.Run(input, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "retain")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(dir, "sentinel")
			if err := os.WriteFile(sentinel, []byte("retained"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := apiPackage(t, dir, input); err == nil {
				t.Fatal("invalid final production configuration accepted")
			}
			if body, err := os.ReadFile(sentinel); err != nil || string(body) != "retained" {
				t.Fatal("invalid binding replaced the existing package")
			}
		})
	}
}
