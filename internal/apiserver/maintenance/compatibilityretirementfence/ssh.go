package compatibilityretirementfence

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh"
)

var protectedPath = regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`)

// ReadRootPolicy requires a root-owned, non-symlink file and root-owned unwritable
// ancestors. Deployment-user authorized_keys or uploaded tools cannot establish
// this trust anchor. The caller must independently approve the original raw hash.
func ReadRootPolicy(path, approvedSHA256 string) (Policy, error) {
	var p Policy
	if !sha64.MatchString(approvedSHA256) {
		return p, ErrSSH
	}
	b, err := readProtected(path, 0, "/")
	if err != nil || digest(b) != approvedSHA256 {
		return p, ErrSSH
	}
	if decodeJSON(b, &p) != nil {
		return p, ErrPolicy
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&p) != nil {
		return p, ErrPolicy
	}
	return p, nil
}

func readProtected(path string, owner uint32, anchor string) ([]byte, error) {
	if !protectedPath.MatchString(path) || filepath.Clean(path) != path || !filepath.IsAbs(path) || !filepath.IsAbs(anchor) || filepath.Clean(anchor) != anchor {
		return nil, ErrSSH
	}
	if anchor != "/" && !strings.HasPrefix(path, anchor+string(os.PathSeparator)) {
		return nil, ErrSSH
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return nil, ErrSSH
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != owner || st.Nlink != 1 {
		return nil, ErrSSH
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		pi, e := os.Lstat(parent)
		if e != nil || !pi.IsDir() || pi.Mode().Perm()&0022 != 0 {
			return nil, ErrSSH
		}
		ps, valid := pi.Sys().(*syscall.Stat_t)
		if !valid || ps.Uid != owner {
			return nil, ErrSSH
		}
		if parent == anchor {
			break
		}
		if parent == "/" {
			return nil, ErrSSH
		}
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrSSH
	}
	file := os.NewFile(uintptr(fd), "protected-policy")
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		_ = file.Close()
		return nil, ErrSSH
	}
	b, err := io.ReadAll(io.LimitReader(file, MaxBodyBytes+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || len(b) > MaxBodyBytes {
		return nil, ErrSSH
	}
	return b, nil
}

// SSHPreparation retains exact original bytes in memory for host-managed CAS
// installation/rollback; neither method writes the server's configuration.
// The root host must install an exclusive root-protected AuthorizedKeysFile and
// independently check sshd -T for every account/client context before activation.
type SSHPreparation struct {
	original, restricted         []byte
	originalHash, restrictedHash string
	keys                         []string
}
type SSHPreparationReceipt struct {
	OriginalSHA256            string `json:"original_sha256"`
	RestrictedSHA256          string `json:"restricted_sha256"`
	KeyCount                  int    `json:"key_count"`
	ProductionInstalled       bool   `json:"production_installed"`
	RootEffectiveSSHDVerified bool   `json:"root_effective_sshd_verified"`
	DropReady                 bool   `json:"drop_ready"`
}

func (p *SSHPreparation) Receipt() SSHPreparationReceipt {
	if p == nil {
		return SSHPreparationReceipt{}
	}
	return SSHPreparationReceipt{OriginalSHA256: p.originalHash, RestrictedSHA256: p.restrictedHash, KeyCount: len(p.keys)}
}
func (p *SSHPreparation) RestrictedBytes() []byte {
	if p == nil {
		return nil
	}
	return bytes.Clone(p.restricted)
}
func (p *SSHPreparation) RestoreOriginal(actualRestricted []byte) ([]byte, error) {
	if p == nil || !bytes.Equal(actualRestricted, p.restricted) {
		return nil, ErrChanged
	}
	return bytes.Clone(p.original), nil
}

// PrepareSSH restricts every raw key; it never rotates keys or adds permissions.
// Existing origin/expiry/verification restrictions are retained. Certificates,
// environments or unsupported key options block instead of being silently lost.
func PrepareSSH(original []byte, approvedOriginalSHA256, gatePath, policyPath, policySHA256, tokenPath string, approvedFingerprints []string) (*SSHPreparation, error) {
	if !sha64.MatchString(policySHA256) || !protectedPath.MatchString(tokenPath) || filepath.Clean(tokenPath) != tokenPath || digest(original) != approvedOriginalSHA256 || len(original) == 0 || len(original) > MaxBodyBytes || !protectedPath.MatchString(gatePath) || filepath.Clean(gatePath) != gatePath || !protectedPath.MatchString(policyPath) || filepath.Clean(policyPath) != policyPath {
		return nil, ErrSSH
	}
	wanted := map[string]bool{}
	for _, f := range approvedFingerprints {
		if !fingerprint.MatchString(f) || wanted[f] {
			return nil, ErrSSH
		}
		wanted[f] = true
	}
	if len(wanted) == 0 {
		return nil, ErrSSH
	}
	seen := map[string]bool{}
	var lines []string
	var keys []string
	for _, line := range bytes.Split(original, []byte("\n")) {
		trim := bytes.TrimSpace(line)
		if len(trim) == 0 || trim[0] == '#' {
			continue
		}
		key, _, opts, rest, err := ssh.ParseAuthorizedKey(trim)
		if err != nil || len(bytes.TrimSpace(rest)) != 0 {
			return nil, ErrSSH
		}
		if _, cert := key.(*ssh.Certificate); cert {
			return nil, ErrSSH
		}
		f := ssh.FingerprintSHA256(key)
		if !wanted[f] || seen[f] {
			return nil, ErrSSH
		}
		seen[f] = true
		keys = append(keys, f)
		options := []string{"restrict"}
		for _, option := range opts {
			switch {
			case option == "restrict" || strings.HasPrefix(option, "command=") || option == "no-port-forwarding" || option == "no-agent-forwarding" || option == "no-X11-forwarding" || option == "no-pty" || option == "no-user-rc":
			case strings.HasPrefix(option, "from=") || strings.HasPrefix(option, "expiry-time=") || option == "verify-required" || option == "no-touch-required":
				options = append(options, option)
			default:
				return nil, ErrSSH
			}
		}
		command := gatePath + " --policy " + policyPath + " --policy-sha256 " + policySHA256 + " --authenticated-key " + f + " --github-token-file " + tokenPath
		options = append(options, `command="`+command+`"`)
		lines = append(lines, strings.Join(options, ",")+" "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
	}
	if len(seen) != len(wanted) {
		return nil, ErrSSH
	}
	restricted := []byte(strings.Join(lines, "\n") + "\n")
	return &SSHPreparation{original: bytes.Clone(original), restricted: restricted, originalHash: digest(original), restrictedHash: digest(restricted), keys: keys}, nil
}

// VerifyEffectiveSSHD checks actual sshd -T output, not a proposed config. The
// host must collect it for each user/host/address Match context; this is not an
// installation or a complete authentication-path proof.
func VerifyEffectiveSSHD(raw []byte, rootAuthorizedKeysPath string) error {
	if len(raw) == 0 || len(raw) > MaxBodyBytes || !protectedPath.MatchString(rootAuthorizedKeysPath) {
		return ErrSSH
	}
	settings := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		pair := strings.SplitN(line, " ", 2)
		if len(pair) != 2 {
			continue
		}
		if _, exists := settings[pair[0]]; exists {
			return ErrSSH
		}
		settings[pair[0]] = strings.TrimSpace(pair[1])
	}
	required := map[string]string{"authenticationmethods": "publickey", "authorizedkeysfile": rootAuthorizedKeysPath, "authorizedkeyscommand": "none", "trustedusercakeys": "none", "passwordauthentication": "no", "kbdinteractiveauthentication": "no", "hostbasedauthentication": "no", "gssapiauthentication": "no", "permituserenvironment": "no", "permituserrc": "no", "disableforwarding": "yes", "permittty": "no", "forcecommand": "none", "pubkeyauthentication": "yes", "strictmodes": "yes"}
	for key, value := range required {
		if settings[key] != value {
			return ErrSSH
		}
	}
	for _, pattern := range strings.Fields(settings["acceptenv"]) {
		// Only locale variables may be accepted; no forced-command arguments or
		// loader/interpreter environment may be supplied by a historical SSH client.
		if pattern != "LANG" && pattern != "LC_*" {
			return ErrSSH
		}
	}
	return nil
}
