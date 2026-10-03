package aimessagingbinding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func fixture() Binding {
	return Binding{Revision: "mq-ai-v1", Enabled: true, NSQD: map[string]string{"nsqd:4150": "http://nsqd:4151"}, Signing: "/run/qs-server-jose/qs.sign.v1.json", Decrypt: map[string]string{"qs.encrypt.v1": "/run/qs-server-jose/qs.encrypt.v1.json"}, Signers: map[string]string{"ai.sign.v1": "/run/qs-server-jose/ai.sign.v1.json"}, Recipient: "/run/qs-server-jose/ai.encrypt.v1.json", MaxInFlight: 1}
}

func TestMQBindingRefusesKeyMetadataPermissionsAndAmbiguity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qs.sign.v1.json")
	file := KeyFile{Kid: "qs.sign.v1", Path: path, Role: "qs.sign", Private: true}
	valid := `{"kid":"qs.sign.v1","use":"sig","alg":"ES256","key_ops":["sign"]}`
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	if validateKeyFile(file) != nil {
		t.Fatal("preliminary metadata rejected")
	}
	for _, raw := range []string{
		`{"kid":"other"}`, `{"kid":"qs.sign.v1","use":"enc"}`,
		`{"kid":"qs.sign.v1","alg":"HS256"}`,
		`{"kid":"qs.sign.v1","key_ops":["verify"]}`,
		`{"kid":"qs.sign.v1","key_ops":[]}`, `{"kid":"qs.sign.v1","key_ops":null}`,
		`{"kid":"qs.sign.v1","kid":"qs.sign.v1"}`, valid + `{}`,
		strings.Repeat("x", 16385),
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if validateKeyFile(file) != ErrBinding {
			t.Fatal("invalid key metadata accepted")
		}
	}
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0644, 0620, 0660, 0604} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if validateKeyFile(file) != ErrBinding {
			t.Fatal("unsafe private key mode accepted")
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "key.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	file.Path = link
	if validateKeyFile(file) != ErrBinding {
		t.Fatal("mutable symlink accepted")
	}
	// Cryptographic validity and public/private role checks are additionally
	// performed by the existing LoadMessagingKeys and actual image preflight.
}

func TestMQBindingRejectsAmbiguousOrOutOfScopeInputs(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*Binding)
	}{
		{"revision", func(b *Binding) { b.Revision = "../other" }},
		{"disabled", func(b *Binding) { b.Enabled = false }},
		{"source", func(b *Binding) { b.NSQD = map[string]string{"nsqd:4150": "http://secret@other:4151"} }},
		{"concurrency", func(b *Binding) { b.MaxInFlight = 2 }},
		{"host path", func(b *Binding) { b.Signing = "/etc/key.json" }},
		{"key role", func(b *Binding) { b.Signing = "/run/qs-server-jose/ai.sign.v1.json" }},
		{"trust kid", func(b *Binding) { b.Signers = map[string]string{"untrusted": "/run/qs-server-jose/ai.sign.v1.json"} }},
		{"missing trust", func(b *Binding) { b.Decrypt = map[string]string{} }},
	} {
		t.Run(change.name, func(t *testing.T) {
			b := fixture()
			change.edit(&b)
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = Decode(raw); err != ErrBinding {
				t.Fatalf("expected bounded refusal, got %v", err)
			}
		})
	}
	raw, err := json.Marshal(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"enabled":true,"enabled":false}`, strings.Replace(string(raw), `"nsqd:4150":"http://nsqd:4151"`, `"nsqd:4150":"http://nsqd:4151","nsqd:4150":"http://other:4151"`, 1), string(raw) + `{}`, strings.Repeat("x", 16385), strings.Replace(string(raw), `"enabled":true`, `"enabled":true,"private_key":"NEVER_DISCLOSE"`, 1)} {
		if _, err = Decode([]byte(input)); err != ErrBinding {
			t.Fatal("ambiguous input accepted")
		}
	}
	b, err := Decode(raw)
	if err != nil || b.SHA256() != fixture().SHA256() {
		t.Fatal("reviewed identity not stable")
	}
}

func TestMQBindingRenderKeepsOriginalConfigAndDoesNotReadEnvironment(t *testing.T) {
	t.Setenv("QS_APISERVER_MYSQL_PASSWORD", "NEVER_COPY_ENV_SECRET")
	base := []byte("ai_workflow:\n  enabled: true\n  management:\n    address: qs-ai-grpc:50061\nmysql:\n  password: ''\ncache:\n  ttl: 30\n")
	rendered, err := Render(base, fixture())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rendered), "NEVER_COPY_ENV_SECRET") {
		t.Fatal("environment credentials copied")
	}
	v := viper.New()
	v.SetConfigType("json")
	if err = v.ReadConfig(strings.NewReader(string(rendered))); err != nil {
		t.Fatal(err)
	}
	if !v.GetBool("ai_workflow.enabled") || v.GetString("ai_workflow.management.address") != "qs-ai-grpc:50061" || v.GetInt("cache.ttl") != 30 || v.GetString("mysql.password") != "" {
		t.Fatal("non-MQ config changed")
	}
	b := fixture()
	opts := b.Options()
	if err = v.UnmarshalKey("ai_workflow.messaging", &opts); err != nil {
		t.Fatal(err)
	}
	if err = opts.Complete(); err != nil {
		t.Fatal(err)
	}
	if opts.Validate() != nil || opts.NSQD["nsqd:4150"] != "http://nsqd:4151" || opts.AISignerFiles["ai.sign.v1"] != "/run/qs-server-jose/ai.sign.v1.json" {
		t.Fatal("literal local trust/source maps drifted")
	}
	if _, err = Render([]byte("other: true"), b); err != ErrBinding {
		t.Fatal("missing host workflow accepted")
	}
}

func TestMQBindingKeyPathsKeepHistoricalRoleIDsWithoutResealing(t *testing.T) {
	b := fixture()
	b.Decrypt["qs.encrypt"] = "/run/qs-server-jose/qs.encrypt.json"
	b.Signers["ai.sign"] = "/run/qs-server-jose/ai.sign.json"
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(raw); err != nil {
		t.Fatal("historical local key retention rejected")
	}
}
