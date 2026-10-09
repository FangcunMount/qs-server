// Package compatibilityretirementfence verifies a single approved SSH invocation.
// It neither installs an SSH policy nor proves that database or external writers
// are fenced. The host owns its HTTP client, credentials and execution lifecycle.
package compatibilityretirementfence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	ErrPolicy   = errors.New("retirement_fence_policy_rejected")
	ErrCommand  = errors.New("retirement_fence_command_rejected")
	ErrIdentity = errors.New("retirement_fence_identity_rejected")
	ErrToken    = errors.New("retirement_fence_oidc_rejected")
	ErrRemote   = errors.New("retirement_fence_remote_read_failed")
	ErrCoverage = errors.New("retirement_fence_remote_coverage_rejected")
	ErrActive   = errors.New("retirement_fence_other_active_run")
	ErrChanged  = errors.New("retirement_fence_recheck_changed")
	ErrSSH      = errors.New("retirement_fence_ssh_boundary_rejected")
)

const (
	Repository   = "FangcunMount/qs-server"
	WorkflowPath = ".github/workflows/compatibility-retirement.yml"
	Issuer       = "https://token.actions.githubusercontent.com"
	JWKSURL      = Issuer + "/.well-known/jwks"
	APIBase      = "https://api.github.com"
	MaxBodyBytes = 2 * 1024 * 1024
	MaxPages     = 100
)

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var sha64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var decimal = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var attemptNumber = regexp.MustCompile(`^[1-9][0-9]{0,3}$`)
var operationID = regexp.MustCompile(`^[1-9][0-9]{0,19}-[1-9][0-9]{0,3}$`)
var fingerprint = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

// Policy must be approved and stored outside every deployment-user writable path.
// Subject is independently approved; GitHub's configurable subject is not guessed.
type Policy struct {
	Version           int       `json:"version"`
	RepositoryID      string    `json:"repository_id"`
	OwnerID           string    `json:"owner_id"`
	SourceSHA         string    `json:"source_sha"`
	OperationID       string    `json:"operation_id"`
	RequestSHA256     string    `json:"request_sha256"`
	RunID             string    `json:"run_id"`
	RunAttempt        string    `json:"run_attempt"`
	WorkflowID        int64     `json:"workflow_id"`
	CheckRunID        string    `json:"check_run_id"`
	ActorID           string    `json:"actor_id"`
	RunnerID          int64     `json:"runner_id"`
	Subject           string    `json:"subject"`
	Audience          string    `json:"audience"`
	LoginUID          uint32    `json:"login_uid"`
	SSHKeyFingerprint string    `json:"ssh_key_fingerprint"`
	ChallengeSHA256   string    `json:"challenge_sha256"`
	ExpiresAt         time.Time `json:"expires_at"`
	WorkflowIDs       []int64   `json:"workflow_ids"`
}

func (p Policy) Validate(now time.Time) error {
	if p.Version != 1 || !decimal.MatchString(p.RepositoryID) || !decimal.MatchString(p.OwnerID) || !sha40.MatchString(p.SourceSHA) || !operationID.MatchString(p.OperationID) || !sha64.MatchString(p.RequestSHA256) || !decimal.MatchString(p.RunID) || !attemptNumber.MatchString(p.RunAttempt) || p.WorkflowID <= 0 || !decimal.MatchString(p.CheckRunID) || !decimal.MatchString(p.ActorID) || p.RunnerID <= 0 || p.LoginUID == 0 || !fingerprint.MatchString(p.SSHKeyFingerprint) || !sha64.MatchString(p.ChallengeSHA256) || len(p.Subject) < 1 || len(p.Subject) > 512 || strings.ContainsAny(p.Subject, "\r\n\x00") || p.Audience != p.ExpectedAudience() || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(30*time.Minute)) || len(p.WorkflowIDs) < 21 || len(p.WorkflowIDs) > 1000 {
		return ErrPolicy
	}
	seen := map[int64]bool{}
	for _, id := range p.WorkflowIDs {
		if id <= 0 || seen[id] {
			return ErrPolicy
		}
		seen[id] = true
	}
	if !seen[p.WorkflowID] {
		return ErrPolicy
	}
	for _, id := range historicalWorkflowIDs {
		if !seen[id] {
			return ErrPolicy
		}
	}
	return nil
}

func (p Policy) ExpectedAudience() string {
	return "qs-retirement-fence/v1/" + p.OperationID + "/" + p.SourceSHA + "/" + p.RunID + "-" + p.RunAttempt + "/" + p.RequestSHA256 + "/" + p.ChallengeSHA256
}
func (p Policy) Command() string {
	return "qs-retirement-fence-v1 " + p.SourceSHA + " " + p.OperationID + " " + p.RunID + " " + p.RunAttempt + " " + p.RequestSHA256 + " probe"
}
func (p Policy) digest() string { b, _ := json.Marshal(p); return digest(b) }
func digest(b []byte) string    { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }

// decodeJSON rejects duplicate keys at every level and trailing bytes. For signed
// provider/API JSON, future unrelated fields are permitted but never trusted.
func decodeJSON(b []byte, dst any) error {
	if len(b) == 0 || len(b) > MaxBodyBytes {
		return ErrRemote
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := walkJSON(d); err != nil {
		return ErrRemote
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return ErrRemote
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return ErrRemote
	}
	return nil
}
func walkJSON(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return e
			}
			key, valid := k.(string)
			if !valid || seen[key] {
				return ErrRemote
			}
			seen[key] = true
			if e = walkJSON(d); e != nil {
				return e
			}
		}
		end, e := d.Token()
		if e != nil || end != json.Delim('}') {
			return ErrRemote
		}
	case '[':
		for d.More() {
			if e := walkJSON(d); e != nil {
				return e
			}
		}
		end, e := d.Token()
		if e != nil || end != json.Delim(']') {
			return ErrRemote
		}
	default:
		return ErrRemote
	}
	return nil
}
