// Explicit offline release preflight; never a service, publisher or business writer.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"

	binding "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/aimessagingbinding"
)

var sourceSHA = "development"

func main() {
	version := flag.Bool("source-sha", false, "print embedded source identity without reading configuration")
	input := flag.String("binding", "", "reviewed local binding JSON; no key contents")
	base := flag.String("base-config", "", "optional source YAML; no environment credential merge")
	out := flag.String("output-config", "", "new rendered JSON file; never overwritten")
	flag.Parse()
	if *version {
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sourceSHA) {
			fmt.Fprintln(os.Stderr, "MQ release source identity unavailable")
			os.Exit(1)
		}
		fmt.Println(sourceSHA)
		return
	}
	if err := run(*input, *base, *out); err != nil {
		fmt.Fprintln(os.Stderr, "MQ release preflight failed; material withheld")
		os.Exit(1)
	}
}

func bounded(path string, max int64) (raw []byte, resultErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > max {
		return nil, binding.ErrBinding
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}

func run(input, base, out string) (resultErr error) {
	if input == "" || ((base == "") != (out == "")) {
		return binding.ErrBinding
	}
	raw, err := bounded(input, 16384)
	if err != nil {
		return err
	}
	b, err := binding.Decode(raw)
	if err != nil {
		return err
	}
	fingerprints, err := binding.ValidateKeys(b)
	if err != nil {
		return err
	}
	var configHash string
	if base != "" {
		original, err := bounded(base, 1<<20)
		if err != nil {
			return err
		}
		config, err := binding.Render(original, b)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
		if err != nil {
			return err
		}
		n, writeErr := f.Write(config)
		if n != len(config) && writeErr == nil {
			writeErr = io.ErrShortWrite
		}
		if err = errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
		sum := sha256.Sum256(config)
		configHash = hex.EncodeToString(sum[:])
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		SourceSHA    string            `json:"source_sha"`
		Revision     string            `json:"binding_revision"`
		BindingHash  string            `json:"binding_sha256"`
		ConfigHash   string            `json:"rendered_config_sha256,omitempty"`
		Fingerprints map[string]string `json:"public_key_fingerprints"`
		Offline      bool              `json:"offline"`
	}{sourceSHA, b.Revision, b.SHA256(), configHash, fingerprints, true})
}
