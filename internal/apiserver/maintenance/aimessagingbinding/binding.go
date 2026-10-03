// Package aimessagingbinding validates a reviewed deployment binding offline.
// It owns no business transaction, broker client or execution lifecycle.
package aimessagingbinding

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	client "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	options "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/spf13/viper"
)

var ErrBinding = errors.New("invalid reviewed MQ binding; contents withheld")

type Binding struct {
	Revision    string            `json:"binding_revision"`
	Enabled     bool              `json:"enabled"`
	NSQD        map[string]string `json:"nsqd"`
	Signing     string            `json:"signing_key_file"`
	Decrypt     map[string]string `json:"decrypt_key_files"`
	Signers     map[string]string `json:"ai_signer_files"`
	Recipient   string            `json:"ai_recipient_key_file"`
	MaxInFlight int               `json:"max_in_flight"`
}

func (b Binding) Options() options.AIWorkflowMessagingOptions {
	return options.AIWorkflowMessagingOptions{Enabled: b.Enabled, NSQD: b.NSQD, SigningKeyFile: b.Signing, DecryptKeyFiles: b.Decrypt, AISignerFiles: b.Signers, AIRecipientKeyFile: b.Recipient}
}

func (b Binding) SHA256() string {
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Decode rejects duplicate object keys, unknown fields, excess bytes and trailing documents.
func Decode(raw []byte) (Binding, error) {
	var b Binding
	if len(raw) == 0 || len(raw) > 16384 {
		return b, ErrBinding
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := unique(d, 0, false); err != nil {
		return b, ErrBinding
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return b, ErrBinding
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&b) != nil || !b.Enabled || b.MaxInFlight != 1 || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`).MatchString(b.Revision) || len(b.NSQD) > 8 || len(b.Decrypt) > 8 || len(b.Signers) > 8 {
		return b, ErrBinding
	}
	if b.Options().Validate() != nil {
		return b, ErrBinding
	}
	if _, err := b.Files(); err != nil {
		return b, ErrBinding
	}
	return b, nil
}

func unique(d *json.Decoder, depth int, arrays bool) error {
	if depth > 16 {
		return ErrBinding
	}
	t, err := d.Token()
	if err != nil {
		return ErrBinding
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '[' && arrays {
		for d.More() {
			if unique(d, depth+1, arrays) != nil {
				return ErrBinding
			}
		}
		t, err = d.Token()
		if err != nil || t != json.Delim(']') {
			return ErrBinding
		}
		return nil
	}
	if delim != '{' {
		return ErrBinding
	}
	seen := map[string]bool{}
	for d.More() {
		k, err := d.Token()
		if err != nil {
			return ErrBinding
		}
		name, ok := k.(string)
		if !ok || seen[name] {
			return ErrBinding
		}
		seen[name] = true
		if err := unique(d, depth+1, arrays); err != nil {
			return err
		}
	}
	t, err = d.Token()
	if err != nil || t != json.Delim('}') {
		return ErrBinding
	}
	return nil
}

type KeyFile struct {
	Kid, Path, Role string
	Private         bool
}

func (b Binding) Files() ([]KeyFile, error) {
	var files []KeyFile
	add := func(path, role, expected string, private bool) error {
		name := filepath.Base(path)
		kid := strings.TrimSuffix(name, ".json")
		pattern := `^` + regexp.QuoteMeta(role) + `(?:\.[a-z0-9][a-z0-9._-]{0,63})?$`
		if filepath.Dir(path) != "/run/qs-server-jose" || !strings.HasSuffix(name, ".json") || !regexp.MustCompile(pattern).MatchString(kid) || (expected != "" && expected != kid) {
			return ErrBinding
		}
		files = append(files, KeyFile{kid, path, role, private})
		return nil
	}
	if add(b.Signing, "qs.sign", "", true) != nil || add(b.Recipient, "ai.encrypt", "", false) != nil {
		return nil, ErrBinding
	}
	for kid, path := range b.Decrypt {
		if add(path, "qs.encrypt", kid, true) != nil {
			return nil, ErrBinding
		}
	}
	for kid, path := range b.Signers {
		if add(path, "ai.sign", kid, false) != nil {
			return nil, ErrBinding
		}
	}
	return files, nil
}

func ValidateKeys(b Binding) (map[string]string, error) {
	files, err := b.Files()
	if err != nil {
		return nil, ErrBinding
	}
	for _, file := range files {
		if validateKeyFile(file) != nil {
			return nil, ErrBinding
		}
	}
	keys, err := client.LoadMessagingKeys(b.Options())
	if err != nil {
		return nil, ErrBinding
	}
	fingerprints := map[string]string{}
	publics := map[string]any{keys.Signing.KeyID: keys.Signing.Public(), keys.Recipient.KeyID: keys.Recipient.Public()}
	for kid, key := range keys.Ring.Decrypt {
		publics[kid] = key.Public()
	}
	for kid, key := range keys.Ring.Signers {
		publics[kid] = key.Key.Public()
	}
	for kid, key := range publics {
		data, err := json.Marshal(key)
		if err != nil {
			return nil, ErrBinding
		}
		sum := sha256.Sum256(data)
		fingerprints[kid] = hex.EncodeToString(sum[:])
	}
	return fingerprints, nil
}

// Render preserves the source's semantic configuration; only the reviewed MQ subtree changes.
// Environment credentials are not read, merged, printed or written.
func Render(base []byte, b Binding) ([]byte, error) {
	if len(base) == 0 || len(base) > 1<<20 {
		return nil, ErrBinding
	}
	v := viper.New()
	v.SetConfigType("yaml")
	if v.ReadConfig(bytes.NewReader(base)) != nil {
		return nil, ErrBinding
	}
	settings := v.AllSettings()
	workflow, ok := settings["ai_workflow"].(map[string]any)
	if !ok {
		return nil, ErrBinding
	}
	workflow["messaging"] = b.Options()
	result, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, ErrBinding
	}
	return append(result, '\n'), nil
}

func validateKeyFile(file KeyFile) error {
	info, err := os.Lstat(file.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 16384 || info.Mode().Perm()&0022 != 0 || (file.Private && info.Mode().Perm()&0007 != 0) {
		return ErrBinding
	}
	data, err := os.ReadFile(file.Path)
	if err != nil || len(data) == 0 || len(data) > 16384 {
		return ErrBinding
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if unique(d, 0, true) != nil {
		return ErrBinding
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return ErrBinding
	}
	var attrs struct {
		Kid string          `json:"kid"`
		Use string          `json:"use"`
		Alg string          `json:"alg"`
		Ops json.RawMessage `json:"key_ops"`
	}
	if json.Unmarshal(data, &attrs) != nil || attrs.Kid != file.Kid {
		return ErrBinding
	}
	purpose, algorithm := "enc", "ECDH-ES+A256KW"
	allowed := map[string]bool{"deriveKey": true, "deriveBits": true}
	if strings.HasSuffix(file.Role, "sign") {
		purpose, algorithm = "sig", "ES256"
		if file.Private {
			allowed = map[string]bool{"sign": true}
		} else {
			allowed = map[string]bool{"verify": true}
		}
	} else if file.Private {
		allowed["decrypt"], allowed["unwrapKey"] = true, true
	} else {
		allowed["encrypt"], allowed["wrapKey"] = true, true
	}
	if (attrs.Use != "" && attrs.Use != purpose) || (attrs.Alg != "" && attrs.Alg != algorithm) {
		return ErrBinding
	}
	var ops []string
	if len(attrs.Ops) != 0 {
		if json.Unmarshal(attrs.Ops, &ops) != nil || len(ops) == 0 {
			return ErrBinding
		}
	}
	for _, op := range ops {
		if !allowed[op] {
			return ErrBinding
		}
	}
	return nil
}
