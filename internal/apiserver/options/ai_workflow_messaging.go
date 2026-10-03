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
	Enabled            bool              `json:"enabled" mapstructure:"enabled"`
	NSQD               map[string]string `json:"nsqd" mapstructure:"nsqd"`
	SigningKeyFile     string            `json:"signing_key_file" mapstructure:"signing_key_file"`
	DecryptKeyFiles    map[string]string `json:"decrypt_key_files" mapstructure:"decrypt_key_files"`
	AISignerFiles      map[string]string `json:"ai_signer_files" mapstructure:"ai_signer_files"`
	AIRecipientKeyFile string            `json:"ai_recipient_key_file" mapstructure:"ai_recipient_key_file"`
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
