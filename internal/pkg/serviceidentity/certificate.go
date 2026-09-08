package serviceidentity

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"
)

// LocalCertificateIdentity reads the configured client certificate for diagnostics.
// It is not proof that a remote server accepted this identity; that requires an RPC.
func LocalCertificateIdentity(caFile, certFile, keyFile string) (string, error) {
	roots, err := os.ReadFile(caFile)
	if err != nil {
		return "", fmt.Errorf("read IAM trust roots: %w", err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(roots) {
		return "", fmt.Errorf("IAM trust roots contain no certificates")
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return "", fmt.Errorf("load IAM client certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", err
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return "", fmt.Errorf("IAM client certificate is outside its validity period")
	}
	name := strings.TrimSpace(leaf.Subject.CommonName)
	if name == "" {
		return "", fmt.Errorf("IAM client certificate requires a service common name")
	}
	return name, nil
}
