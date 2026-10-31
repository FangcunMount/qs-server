//go:build integration && m3_host_runtime

package runtimeclosure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/stretchr/testify/require"
)

// Optional encrypted evidence leaves the private log on the fixture host. The
// recipient private key is never supplied to the workflow or stored in Git.
func encryptM5StartupDiagnostic(role string, plain []byte) error {
	cert, dir := os.Getenv("QS_M5_DIAGNOSTIC_PUBLIC_CERT"), os.Getenv("QS_M5_DIAGNOSTIC_DIRECTORY")
	if cert == "" {
		return nil
	}
	if (role != "api" && role != "collection" && role != "worker") || len(cert) == 0 || len(cert) > 32768 || len(plain) > 1<<20 || !filepath.IsAbs(dir) {
		return errors.New("bounded owned diagnostic input required")
	}
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("private owned diagnostic directory required")
	}
	temporary, err := os.MkdirTemp("", "m5-public-recipient-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	recipient := filepath.Join(temporary, "recipient.pem")
	if err := os.WriteFile(recipient, []byte(cert), 0600); err != nil {
		return err
	}
	path := filepath.Join(dir, role+".p7m")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openssl", "cms", "-encrypt", "-binary", "-aes256", "-outform", "DER", recipient)
	cmd.Stdin = bytes.NewReader(plain)
	cmd.Stdout = f
	runErr := cmd.Run()
	closeErr := f.Close()
	if runErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return errors.New("diagnostic encryption unavailable")
	}
	cipher, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	plainHash, cipherHash, certHash := sha256.Sum256(plain), sha256.Sum256(cipher), sha256.Sum256([]byte(cert))
	raw, err := json.Marshal(map[string]any{"fixture_only": true, "role": role, "source_sha": version.Get().GitCommit, "plaintext_sha256": fmt.Sprintf("%x", plainHash), "ciphertext_sha256": fmt.Sprintf("%x", cipherHash), "recipient_pem_sha256": fmt.Sprintf("%x", certHash), "algorithm": "openssl-cms-aes256"})
	if err != nil {
		return err
	}
	manifest, err := os.OpenFile(filepath.Join(dir, role+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := manifest.Write(append(raw, '\n'))
	return errors.Join(writeErr, manifest.Close())
}

func TestM5StartupDiagnosticEncryption(t *testing.T) {
	dir := t.TempDir()
	key, cert := filepath.Join(dir, "recipient.key"), filepath.Join(dir, "recipient.pem")
	cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", cert, "-days", "1", "-subj", "/CN=owned-m5-diagnostic")
	require.NoError(t, cmd.Run())
	pem, err := os.ReadFile(cert)
	require.NoError(t, err)
	out := filepath.Join(dir, "cipher-only")
	require.NoError(t, os.Mkdir(out, 0700))
	t.Setenv("QS_M5_DIAGNOSTIC_PUBLIC_CERT", string(pem))
	t.Setenv("QS_M5_DIAGNOSTIC_DIRECTORY", out)
	plain := []byte("owned private fixture trace never uploaded in plaintext")
	require.NoError(t, encryptM5StartupDiagnostic("api", plain))
	cipher, err := os.ReadFile(filepath.Join(out, "api.p7m"))
	require.NoError(t, err)
	require.False(t, bytes.Contains(cipher, plain))
	decrypted, err := exec.Command("openssl", "cms", "-decrypt", "-binary", "-inform", "DER", "-in", filepath.Join(out, "api.p7m"), "-recip", cert, "-inkey", key).Output()
	require.NoError(t, err)
	require.Equal(t, plain, decrypted)
	names, err := os.ReadDir(out)
	require.NoError(t, err)
	require.Len(t, names, 2)
	require.Error(t, encryptM5StartupDiagnostic("api", plain), "must not overwrite evidence")
	require.Error(t, encryptM5StartupDiagnostic("../foreign", plain))
	require.NoError(t, os.Chmod(out, 0755))
	require.Error(t, encryptM5StartupDiagnostic("worker", plain))
}
