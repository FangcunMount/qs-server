package options

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestMQOptionsPreserveDottedEndpointsAndTrustThroughHostDecode(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(strings.NewReader(`ai_workflow:
  messaging:
    enabled: true
    nsqd:
      '127.0.0.1:4150': 'http://127.0.0.1:4151'
      'nsqd.fixture.test:4150': 'http://nsqd.fixture.test:4151'
    signing_key_file: sign
    ai_recipient_key_file: recipient
    decrypt_key_files:
      qs.encrypt.v1: old-private
      qs.encrypt.v2: new-private
    ai_signer_files:
      ai.sign.v1: old-public
      ai.sign.v2: new-public
`))
	if err != nil {
		t.Fatal(err)
	}
	o := NewOptions()
	o.SecureServing.BindPort = 0
	if err = v.Unmarshal(o); err != nil {
		t.Fatal("normal host configuration cannot decode immutable key/endpoint identity", err)
	}
	// The host calls Complete before Validate and assembly.
	if err = o.Complete(); err != nil {
		t.Fatal(err)
	}
	m := o.AIWorkflow.Messaging
	if m.Validate() != nil || len(m.NSQD) != 2 || m.NSQD["127.0.0.1:4150"] != "http://127.0.0.1:4151" || m.NSQD["nsqd.fixture.test:4150"] != "http://nsqd.fixture.test:4151" || len(m.DecryptKeyFiles) != 2 || m.DecryptKeyFiles["qs.encrypt.v1"] != "old-private" || m.DecryptKeyFiles["qs.encrypt.v2"] != "new-private" || len(m.AISignerFiles) != 2 || m.AISignerFiles["ai.sign.v1"] != "old-public" || m.AISignerFiles["ai.sign.v2"] != "new-public" {
		t.Fatal("configuration rebound local endpoint/key identities")
	}
}

func TestMQOptionsRejectMalformedLocalMapWithoutCoercingTrust(t *testing.T) {
	for _, raw := range []string{`ai_signer_files: {ai.sign: 42}`, `decrypt_key_files: {qs.encrypt: [a,b]}`} {
		v := viper.New()
		v.SetConfigType("yaml")
		if err := v.ReadConfig(strings.NewReader("ai_workflow:\n  messaging:\n    " + raw + "\n")); err != nil {
			t.Fatal(err)
		}
		o := NewOptions()
		o.SecureServing.BindPort = 0
		if err := v.Unmarshal(o); err != nil {
			t.Fatal(err)
		}
		if err := o.Complete(); err == nil || !strings.Contains(err.Error(), "AI MQ local map") {
			t.Fatal("malformed local map was silently converted to trusted strings")
		}
	}
	// Viper omits empty branches. They must never become a trusted endpoint:
	// disabled empty maps remain compatible, enabled empty maps fail Validate.
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader("ai_workflow:\n  messaging:\n    nsqd: {host: {}}\n")); err != nil {
		t.Fatal(err)
	}
	empty := NewOptions()
	empty.SecureServing.BindPort = 0
	if err := v.Unmarshal(empty); err != nil {
		t.Fatal(err)
	}
	if err := empty.Complete(); err != nil || len(empty.AIWorkflow.Messaging.NSQD) != 0 {
		t.Fatal("empty configuration invented a trusted endpoint", err)
	}
	empty.AIWorkflow.Messaging.Enabled = true
	empty.AIWorkflow.Messaging.SigningKeyFile = "sign"
	empty.AIWorkflow.Messaging.AIRecipientKeyFile = "recipient"
	empty.AIWorkflow.Messaging.DecryptKeyFiles = map[string]string{"qs.encrypt": "private"}
	empty.AIWorkflow.Messaging.AISignerFiles = map[string]string{"ai.sign": "public"}
	if err := empty.AIWorkflow.Messaging.Validate(); err == nil {
		t.Fatal("enabled empty endpoint map allowed startup")
	}
	// Direct JSON / constructed options are not rebased through Viper's tree.
	o := AIWorkflowMessagingOptions{NSQD: map[string]string{"127.0.0.1:4150": "http://127.0.0.1:4151"}, AISignerFiles: map[string]string{"ai.sign": "public"}}
	if err := o.Complete(); err != nil || o.AISignerFiles["ai.sign"] != "public" || len(o.NSQD) != 1 {
		t.Fatal("direct options changed", err)
	}
}
