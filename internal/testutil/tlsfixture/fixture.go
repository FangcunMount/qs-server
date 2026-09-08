// Package tlsfixture generates ephemeral certificates for real transport contract tests.
package tlsfixture

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type Authority struct {
	Cert   *x509.Certificate
	Key    ed25519.PrivateKey
	CAFile string
}
type Pair struct {
	CertFile, KeyFile string
	Certificate       tls.Certificate
}

func New(t *testing.T) *Authority {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	tmpl := &x509.Certificate{SerialNumber: serial(t), Subject: pkix.Name{CommonName: "test-root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	must(t, err)
	cert, err := x509.ParseCertificate(raw)
	must(t, err)
	path := filepath.Join(t.TempDir(), "ca.pem")
	must(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), 0600))
	return &Authority{Cert: cert, Key: key, CAFile: path}
}
func (a *Authority) Issue(t *testing.T, name string, expired bool) Pair {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	end := time.Now().Add(time.Hour)
	if expired {
		end = time.Now().Add(-time.Minute)
	}
	tmpl := &x509.Certificate{SerialNumber: serial(t), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: end, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	raw, err := x509.CreateCertificate(rand.Reader, tmpl, a.Cert, pub, a.Key)
	must(t, err)
	pk, err := x509.MarshalPKCS8PrivateKey(key)
	must(t, err)
	dir := t.TempDir()
	p := Pair{CertFile: filepath.Join(dir, "cert.pem"), KeyFile: filepath.Join(dir, "key.pem")}
	must(t, os.WriteFile(p.CertFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), 0600))
	must(t, os.WriteFile(p.KeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}), 0600))
	p.Certificate, err = tls.LoadX509KeyPair(p.CertFile, p.KeyFile)
	must(t, err)
	return p
}
func (a *Authority) Client(pair *Pair) *tls.Config {
	roots := x509.NewCertPool()
	roots.AddCert(a.Cert)
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "server.test"}
	if pair != nil {
		cfg.Certificates = []tls.Certificate{pair.Certificate}
	}
	return cfg
}
func serial(t *testing.T) *big.Int {
	n, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	must(t, e)
	return n
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
