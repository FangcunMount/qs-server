// qs-retirement-fence is an ordinary-user forced-command probe. A separate
// privileged host installs the immutable binary/policy/key configuration and
// owns its restoration; this process never installs or mutates that state.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

type options struct{ policy, policyHash, key, tokenFile string }

var errInput = errors.New("retirement_fence_host_input_rejected")
var errCredential = errors.New("retirement_fence_host_read_credential_unavailable")

func parseOptions(args []string) (options, error) {
	var o options
	flags := flag.NewFlagSet("qs-retirement-fence", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&o.policy, "policy", "", "")
	flags.StringVar(&o.policyHash, "policy-sha256", "", "")
	flags.StringVar(&o.key, "authenticated-key", "", "")
	flags.StringVar(&o.tokenFile, "github-token-file", "", "")
	if len(args) != 8 {
		return o, errInput
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		if seen[args[i]] || !strings.HasPrefix(args[i], "--") || strings.HasPrefix(args[i+1], "--") {
			return o, errInput
		}
		seen[args[i]] = true
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 || o.policy == "" || o.policyHash == "" || o.key == "" || o.tokenFile == "" {
		return o, errInput
	}
	return o, nil
}
func readTokenFile(path string, uid uint32) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errCredential
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0027 != 0 {
		return nil, errCredential
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 || (st.Uid != 0 && st.Uid != uid) {
		return nil, errCredential
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errCredential
	}
	file := os.NewFile(uintptr(fd), "host-read-credential")
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		_ = file.Close()
		return nil, errCredential
	}
	b, err := io.ReadAll(io.LimitReader(file, 8193))
	closeErr := file.Close()
	b = bytes.TrimSuffix(b, []byte("\n"))
	if err != nil || closeErr != nil || len(b) > 8192 || !regexp.MustCompile(`^[\x21-\x7e]+$`).Match(b) {
		return nil, errCredential
	}
	return b, nil
}

type readAuthTransport struct {
	token []byte
	inner http.RoundTripper
}

func (t readAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	if copyReq.URL.Scheme != "https" || copyReq.URL.User != nil || (copyReq.URL.Host != "api.github.com" && copyReq.URL.Host != "token.actions.githubusercontent.com") {
		return nil, errInput
	}
	if copyReq.URL.Host == "api.github.com" {
		copyReq.Header.Set("Authorization", "Bearer "+string(t.token))
	}
	return t.inner.RoundTrip(copyReq)
}
func run(ctx context.Context, args []string, stdin io.Reader, command string, uid uint32) (fence.Receipt, error) {
	opts, err := parseOptions(args)
	if err != nil {
		return fence.Receipt{}, err
	}
	p, err := fence.ReadRootPolicy(opts.policy, opts.policyHash)
	if err != nil {
		return fence.Receipt{}, err
	}
	if err = p.Validate(time.Now()); err != nil {
		return fence.Receipt{}, err
	}
	// Reject obsolete script grammar before reading credentials or signed input.
	if command != p.Command() {
		return fence.Receipt{}, fence.ErrCommand
	}
	if opts.key != p.SSHKeyFingerprint || uid != p.LoginUID {
		return fence.Receipt{}, fence.ErrIdentity
	}
	token, err := readTokenFile(opts.tokenFile, uid)
	if err != nil {
		return fence.Receipt{}, err
	}
	jwt, err := io.ReadAll(io.LimitReader(stdin, 16385))
	if err != nil || len(jwt) > 16384 {
		return fence.Receipt{}, fence.ErrToken
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: readAuthTransport{token: token, inner: transport}, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	permit, err := fence.AuthorizeProbe(ctx, client, p, command, opts.key, string(jwt), uid, time.Now())
	if err != nil {
		return fence.Receipt{}, err
	}
	return permit.Receipt(), nil
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// This executable owns stdin; close it on the host deadline so an SSH client
	// cannot block its read indefinitely. No borrowed resource is closed.
	go func() { <-ctx.Done(); _ = os.Stdin.Close() }()
	receipt, err := run(ctx, os.Args[1:], os.Stdin, os.Getenv("SSH_ORIGINAL_COMMAND"), uint32(os.Getuid()))
	output := struct {
		Category string        `json:"category"`
		Receipt  fence.Receipt `json:"receipt"`
	}{Category: "probe_authorized", Receipt: receipt}
	if err != nil {
		output.Receipt = (*fence.Permit)(nil).Receipt()
		output.Category = err.Error()
	}
	raw, encodeErr := json.Marshal(output)
	if encodeErr != nil {
		os.Exit(2)
	}
	if _, writeErr := os.Stdout.Write(append(raw, '\n')); writeErr != nil {
		os.Exit(2)
	}
	if err != nil {
		os.Exit(1)
	}
}
