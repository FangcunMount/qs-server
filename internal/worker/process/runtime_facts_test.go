package process

import (
	"context"
	"reflect"
	"strings"
	"testing"

	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime/testsupport"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	messagingintegration "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/spf13/viper"
)

func TestWorkerRuntimeFactsUseFinalFlagsAndEnv(t *testing.T) {
	for _, flags := range []bool{false, true} {
		t.Run(map[bool]string{false: "environment", true: "flag_overrides_environment"}[flags], func(t *testing.T) {
			opts := options.NewOptions()
			address, identities := testsupport.NSQPublisherFixture(t)
			sets := opts.Flags()
			fs := sets.FlagSet("messaging")
			v := viper.New()
			v.SetConfigType("yaml")
			if err := v.ReadConfig(strings.NewReader("messaging:\n  provider: nsq\n  nsq-addr: file-nsqd.internal:2150\n  nsq-lookupd-addr: file-lookupd.internal:2161\n  nsqd-http-endpoints: [http://file-nsqd.internal:2151]\n")); err != nil {
				t.Fatal(err)
			}
			v.AutomaticEnv()
			v.SetEnvPrefix("QS_WORKER")
			v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
			t.Setenv("QS_WORKER_MESSAGING_NSQ_ADDR", address)
			t.Setenv("QS_WORKER_MESSAGING_NSQ_LOOKUPD_ADDR", "env-lookupd.internal:3161")
			t.Setenv("QS_WORKER_MESSAGING_NSQD_HTTP_ENDPOINTS", "http://env-nsqd.internal:3151")
			wantTCP, wantLookup, wantHTTP := address, "env-lookupd.internal:3161", "http://env-nsqd.internal:3151"
			if flags {
				t.Setenv("QS_WORKER_MESSAGING_NSQ_ADDR", "unused-env.internal:3150")
				if err := fs.Parse([]string{"--messaging.nsq-addr=" + address, "--messaging.nsq-lookupd-addr=flag-lookupd.internal:5161", "--messaging.nsqd-http-endpoints=https://flag-nsqd.internal:5151"}); err != nil {
					t.Fatal(err)
				}
				wantTCP, wantLookup, wantHTTP = address, "flag-lookupd.internal:5161", "https://flag-nsqd.internal:5151"
			}
			if err := v.BindPFlags(fs); err != nil {
				t.Fatal(err)
			}
			if err := v.Unmarshal(opts); err != nil {
				t.Fatal(err)
			}
			cfg, err := workerconfig.CreateConfigFromOptions(opts)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := runtimefacts.New("worker", strings.Repeat("a", 40))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close() }()
			s := &server{config: cfg, runtimeFacts: owner}
			subscriber, err := eventtransport.NewSDKDeliverySubscriberWithFacts(s.loadedSubscriberConfig(), 1, 8, func(context.Context, legacy.FailedHandoff) error { return nil }, s.runtimeFacts, "worker-events")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = subscriber.Close() }()
			publisher, err := messagingintegration.CreateSDKWirePublisherWithFacts(cfg.Messaging, s.runtimeFacts)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = publisher.Close() }()
			actual := testsupport.ReceiveNSQIdentify(t, identities)
			facts := owner.Snapshot().Transports
			if len(facts) != 2 || actual.ClientID != facts[1].ClientID || actual.Hostname != facts[1].Hostname || !reflect.DeepEqual(facts[0].LookupdAddresses, []string{wantLookup}) || !reflect.DeepEqual(facts[0].NSQDHTTPAddresses, []string{wantHTTP}) || !reflect.DeepEqual(facts[1].NSQDTCPAddresses, []string{wantTCP}) || !reflect.DeepEqual(facts[1].NSQDHTTPAddresses, []string{wantHTTP}) {
				t.Fatal("observer captured file/default inputs instead of final host objects")
			}
		})
	}
}
