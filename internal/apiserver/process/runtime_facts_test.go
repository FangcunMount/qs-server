package process

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime/testsupport"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	"github.com/spf13/viper"
)

func TestAPIRuntimeFactsWireFactoryUsesFinalFlagsAndEnv(t *testing.T) {
	for _, flags := range []bool{false, true} {
		t.Run(map[bool]string{false: "environment", true: "flag_overrides_environment"}[flags], func(t *testing.T) {
			opts := apiserveroptions.NewOptions()
			address, identities := testsupport.NSQPublisherFixture(t)
			sets := opts.Flags()
			fs := sets.FlagSet("messaging")
			v := viper.New()
			v.SetConfigType("yaml")
			if err := v.ReadConfig(strings.NewReader("messaging:\n  enabled: true\n  provider: nsq\n  nsq-addr: file-nsqd.internal:2150\n  nsqd-http-endpoints: [http://file-nsqd.internal:2151]\n")); err != nil {
				t.Fatal(err)
			}
			v.AutomaticEnv()
			v.SetEnvPrefix("QS_APISERVER")
			v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
			t.Setenv("QS_APISERVER_MESSAGING_NSQ_ADDR", address)
			t.Setenv("QS_APISERVER_MESSAGING_NSQD_HTTP_ENDPOINTS", "http://env-nsqd.internal:3151")
			wantTCP, wantHTTP := address, "http://env-nsqd.internal:3151"
			if flags {
				t.Setenv("QS_APISERVER_MESSAGING_NSQ_ADDR", "unused-env.internal:3150")
				if err := fs.Parse([]string{"--messaging.nsq-addr=" + address, "--messaging.nsqd-http-endpoints=https://flag-nsqd.internal:5151"}); err != nil {
					t.Fatal(err)
				}
				wantTCP, wantHTTP = address, "https://flag-nsqd.internal:5151"
			}
			if err := v.BindPFlags(fs); err != nil {
				t.Fatal(err)
			}
			if err := v.Unmarshal(opts); err != nil {
				t.Fatal(err)
			}
			owner, err := runtimefacts.New("apiserver", strings.Repeat("a", 40))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close() }()
			s := &server{config: &apiserverconfig.Config{Options: opts}, runtimeFacts: owner}
			deps := s.buildMQPublisherDeps()
			if deps.newWirePublisher == nil {
				t.Fatal("resolved factory missing")
			}
			publisher, err := deps.newWirePublisher()
			if err != nil {
				t.Fatal(err)
			}
			facts := owner.Snapshot().Transports
			actual := testsupport.ReceiveNSQIdentify(t, identities)
			if len(facts) != 1 || actual.ClientID != facts[0].ClientID || actual.Hostname != facts[0].Hostname || !reflect.DeepEqual(facts[0].NSQDTCPAddresses, []string{wantTCP}) || !reflect.DeepEqual(facts[0].NSQDHTTPAddresses, []string{wantHTTP}) {
				t.Fatal("wire factory projection ignored final overrides")
			}
			if err := publisher.Close(); err != nil {
				t.Fatal(err)
			}
			if owner.Snapshot().Transports[0].State != "stopped" {
				t.Fatal("factory close retained started observation")
			}
		})
	}
}

func TestAPIRuntimeFactsOwnerStopsWhenServingFails(t *testing.T) {
	var order []string
	wantErr := errors.New("HTTP exited")
	err := runPreparedServer(preparedServerRunDeps{
		startShutdown:     func() error { order = append(order, "shutdown"); return nil },
		startRuntimeFacts: func() { order = append(order, "facts-start") },
		stopRuntimeFacts:  func() { order = append(order, "facts-stop") },
		transports:        preparedServerTransports{runHTTP: func() error { order = append(order, "http"); return wantErr }},
	})
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(order, []string{"shutdown", "facts-start", "http", "facts-stop"}) {
		t.Fatalf("runtime observation escaped serving lifecycle: %v %v", order, err)
	}
}
