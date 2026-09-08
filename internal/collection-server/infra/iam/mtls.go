package iam

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
)

// LocalCertificateIdentity validates local mTLS material; the server authenticates RPCs independently.
func (c *Client) LocalCertificateIdentity() (string, error) {
	if c == nil || c.config == nil || !c.config.GRPCEnabled || c.config.GRPC == nil || c.config.GRPC.TLS == nil || !c.config.GRPC.TLS.Enabled {
		return "", fmt.Errorf("IAM gRPC mTLS is required")
	}
	cfg := c.config.GRPC.TLS
	return serviceidentity.LocalCertificateIdentity(cfg.CAFile, cfg.CertFile, cfg.KeyFile)
}
