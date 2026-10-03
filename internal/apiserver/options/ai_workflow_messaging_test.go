package options

import "testing"

func TestMQOptionsRequireFixedEndpointsAndLocalTrust(t *testing.T) {
	if (AIWorkflowMessagingOptions{}).Validate() != nil {
		t.Fatal("disabled configuration requires secrets")
	}
	o := AIWorkflowMessagingOptions{Enabled: true, NSQD: map[string]string{"localhost:4150": "http://localhost:4151"}, SigningKeyFile: "sign", AIRecipientKeyFile: "encrypt", DecryptKeyFiles: map[string]string{"old": "old.json"}, AISignerFiles: map[string]string{"ai": "ai.json"}}
	if o.Validate() != nil {
		t.Fatal("valid explicit options rejected")
	}
	for _, endpoint := range []string{"https://user:pass@localhost:4151", "https://localhost/path", "file:///keys", "http://localhost?query=1", "http://localhost/#fragment"} {
		bad := o
		bad.NSQD = map[string]string{"localhost:4150": endpoint}
		if bad.Validate() == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, tcp := range []string{"localhost", "localhost:0", "localhost:65536"} {
		bad := o
		bad.NSQD = map[string]string{tcp: "http://localhost:4151"}
		if bad.Validate() == nil {
			t.Fatal("unsafe TCP accepted")
		}
	}
}
