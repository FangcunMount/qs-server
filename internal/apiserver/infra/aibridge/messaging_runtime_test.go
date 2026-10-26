package aibridge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	jose "github.com/go-jose/go-jose/v4"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func localMessagingKey(t *testing.T, id string) jose.JSONWebKey {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return jose.JSONWebKey{Key: k, KeyID: id}
}
func writeMessagingKey(t *testing.T, key jose.JSONWebKey) string {
	t.Helper()
	raw, e := json.Marshal(key)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "key.json")
	if e = os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestMQKeyRolesRotationAndLocalTrustMapping(t *testing.T) {
	sign, crypt, old, aiSign, aiCrypt := localMessagingKey(t, "qs-sign"), localMessagingKey(t, "qs-crypt"), localMessagingKey(t, "qs-old"), localMessagingKey(t, "ai-sign"), localMessagingKey(t, "ai-crypt")
	o := opts.AIWorkflowMessagingOptions{SigningKeyFile: writeMessagingKey(t, sign), AIRecipientKeyFile: writeMessagingKey(t, aiCrypt.Public()), DecryptKeyFiles: map[string]string{crypt.KeyID: writeMessagingKey(t, crypt), old.KeyID: writeMessagingKey(t, old)}, AISignerFiles: map[string]string{aiSign.KeyID: writeMessagingKey(t, aiSign.Public())}}
	k, e := LoadMessagingKeys(o)
	if e != nil || len(k.Ring.Decrypt) != 2 {
		t.Fatalf("rotation=%v", e)
	}
	for _, scenario := range []string{"public_signer", "private_recipient", "unknown_kid", "reused_qs_key", "reused_ai_key", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			bad := o
			switch scenario {
			case "public_signer":
				bad.SigningKeyFile = writeMessagingKey(t, sign.Public())
			case "private_recipient":
				bad.AIRecipientKeyFile = writeMessagingKey(t, aiCrypt)
			case "unknown_kid":
				bad.DecryptKeyFiles = map[string]string{"other": writeMessagingKey(t, crypt)}
			case "reused_qs_key":
				bad.DecryptKeyFiles = map[string]string{sign.KeyID: writeMessagingKey(t, sign)}
			case "reused_ai_key":
				bad.AISignerFiles = map[string]string{aiCrypt.KeyID: writeMessagingKey(t, aiCrypt.Public())}
			case "missing":
				bad.SigningKeyFile = "/missing/sensitive-private-key"
			}
			_, e := LoadMessagingKeys(bad)
			if e == nil || strings.Contains(e.Error(), "sensitive") {
				t.Fatal("invalid trust accepted or secret path exposed")
			}
		})
	}
}
