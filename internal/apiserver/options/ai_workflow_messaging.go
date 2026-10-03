package options

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Maps are local configuration: no discovery, remote keys, or reference URLs.
type AIWorkflowMessagingOptions struct {
	Enabled         bool              `json:"enabled" mapstructure:"enabled"`
	NSQD            map[string]string `json:"nsqd" mapstructure:"-"`
	SigningKeyFile  string            `json:"signing_key_file" mapstructure:"signing_key_file"`
	DecryptKeyFiles map[string]string `json:"decrypt_key_files" mapstructure:"-"`
	AISignerFiles   map[string]string `json:"ai_signer_files" mapstructure:"-"`
	// Viper expands dots inside map keys. Decode these three local maps as a
	// tree, then restore literal endpoint/kid names during the host Complete.
	// JSON maintenance/probe inputs keep their original map representation.
	NSQDConfig         map[string]any `json:"-" mapstructure:"nsqd"`
	DecryptConfig      map[string]any `json:"-" mapstructure:"decrypt_key_files"`
	SignerConfig       map[string]any `json:"-" mapstructure:"ai_signer_files"`
	AIRecipientKeyFile string         `json:"ai_recipient_key_file" mapstructure:"ai_recipient_key_file"`
}

func (o AIWorkflowMessagingOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	if len(o.NSQD) == 0 || strings.TrimSpace(o.SigningKeyFile) == "" || strings.TrimSpace(o.AIRecipientKeyFile) == "" || len(o.DecryptKeyFiles) == 0 || len(o.AISignerFiles) == 0 {
		return errors.New("AI MQ requires explicit NSQD and local signing/encryption trust files")
	}
	for tcp, http := range o.NSQD {
		host, port, err := net.SplitHostPort(tcp)
		n, pErr := strconv.Atoi(port)
		u, uErr := url.Parse(http)
		if err != nil || host == "" || pErr != nil || n < 1 || n > 65535 || uErr != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return errors.New("AI MQ requires fixed NSQD TCP/HTTP endpoints")
		}
	}
	for _, m := range []map[string]string{o.DecryptKeyFiles, o.AISignerFiles} {
		for kid, path := range m {
			if strings.TrimSpace(kid) == "" || strings.TrimSpace(path) == "" {
				return errors.New("AI MQ requires nonempty local kid trust mappings")
			}
		}
	}
	return nil
}

// Complete is pure configuration decoding: no key read, pool, client or task.
func (o *AIWorkflowMessagingOptions) Complete() error {
	for _, pair := range []struct {
		raw    map[string]any
		target *map[string]string
	}{
		{o.NSQDConfig, &o.NSQD}, {o.DecryptConfig, &o.DecryptKeyFiles}, {o.SignerConfig, &o.AISignerFiles},
	} {
		if pair.raw == nil {
			continue
		}
		if len(pair.raw) == 0 {
			*pair.target = map[string]string{}
			continue
		}
		restored := make(map[string]string)
		var walk func(string, map[string]any) error
		walk = func(prefix string, tree map[string]any) error {
			if len(tree) == 0 {
				return errors.New("AI MQ local map has an empty branch")
			}
			for key, value := range tree {
				if key == "" {
					return errors.New("AI MQ local map has an empty identity")
				}
				name := key
				if prefix != "" {
					name = prefix + "." + key
				}
				switch v := value.(type) {
				case string:
					if _, exists := restored[name]; exists {
						return errors.New("AI MQ local map identity is ambiguous")
					}
					restored[name] = v
				case map[string]any:
					if err := walk(name, v); err != nil {
						return err
					}
				default:
					return errors.New("AI MQ local map requires string values")
				}
			}
			return nil
		}
		if err := walk("", pair.raw); err != nil {
			return err
		}
		*pair.target = restored
	}
	return nil
}
