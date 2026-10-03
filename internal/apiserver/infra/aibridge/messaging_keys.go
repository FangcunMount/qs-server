package aibridge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/json"
	"errors"
	"os"
	"reflect"

	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
)

type MessagingKeys struct {
	Ring               protected.Keyring
	Signing, Recipient jose.JSONWebKey
}

// LoadMessagingKeys is an explicit host bootstrap read, never a broker action.
// Failures omit filenames, key material and untrusted decoder text.
func LoadMessagingKeys(o opts.AIWorkflowMessagingOptions) (MessagingKeys, error) {
	result := MessagingKeys{Ring: protected.Keyring{Decrypt: map[string]jose.JSONWebKey{}, Signers: map[string]protected.TrustedSigner{}}}
	read := func(path string, private bool) (jose.JSONWebKey, error) {
		var key jose.JSONWebKey
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &key) != nil || key.KeyID == "" || !key.Valid() {
			return key, errors.New("AI MQ local key unavailable")
		}
		valid := false
		switch k := key.Key.(type) {
		case *ecdsa.PrivateKey:
			valid = private && k.Curve == elliptic.P256()
		case *ecdsa.PublicKey:
			valid = !private && k.Curve == elliptic.P256()
		}
		if !valid {
			return key, errors.New("AI MQ requires role-specific P-256 keys")
		}
		return key, nil
	}
	var err error
	if result.Signing, err = read(o.SigningKeyFile, true); err != nil {
		return result, err
	}
	if result.Recipient, err = read(o.AIRecipientKeyFile, false); err != nil {
		return result, err
	}
	for kid, path := range o.DecryptKeyFiles {
		k, e := read(path, true)
		if e != nil {
			return result, e
		}
		if k.KeyID != kid || reflect.DeepEqual(k.Public().Key, result.Signing.Public().Key) {
			return result, errors.New("AI MQ decryption trust mapping rejected")
		}
		result.Ring.Decrypt[kid] = k
	}
	for kid, path := range o.AISignerFiles {
		k, e := read(path, false)
		if e != nil {
			return result, e
		}
		if k.KeyID != kid || reflect.DeepEqual(k.Key, result.Recipient.Key) {
			return result, errors.New("AI MQ signature trust mapping rejected")
		}
		result.Ring.Signers[kid] = protected.TrustedSigner{Producer: "qs-ai", Key: k}
	}
	return result, nil
}
