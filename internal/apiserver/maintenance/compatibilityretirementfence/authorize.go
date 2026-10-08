package compatibilityretirementfence

import (
	"context"
	"time"
)

// Permit is opaque and only records authorization of one SSH probe. It is not
// accepted as a whole-system writer fence, a retirement verdict, or DROP authority.
type Permit struct {
	policyHash, sourceSHA, operation, run, request, tokenHash, snapshotHash string
	workflows                                                               int
}

type Receipt struct {
	Protocol                     string   `json:"protocol"`
	PolicySHA256                 string   `json:"policy_sha256"`
	SourceSHA                    string   `json:"source_sha"`
	OperationID                  string   `json:"operation_id"`
	RunID                        string   `json:"run_id"`
	RequestSHA256                string   `json:"request_sha256"`
	TokenSHA256                  string   `json:"token_sha256"`
	GitHubSnapshotSHA256         string   `json:"github_snapshot_sha256"`
	WorkflowCount                int      `json:"workflow_count"`
	SSHProbeInvocationVerified   bool     `json:"ssh_probe_invocation_verified"`
	ProductionInstalled          bool     `json:"production_installed"`
	WholeSystemWriterFenceProven bool     `json:"whole_system_writer_fence_proven"`
	MutationBackendEnabled       bool     `json:"mutation_backend_enabled"`
	DropReady                    bool     `json:"drop_ready"`
	ReplayProtectionUnproven     bool     `json:"replay_protection_unproven"`
	Gaps                         []string `json:"gaps"`
}

func (p *Permit) Receipt() Receipt {
	r := Receipt{Protocol: "approved-ssh-probe/v1", ReplayProtectionUnproven: true, Gaps: []string{"root_protected_ssh_installation_and_effective_auth_paths_not_bound", "authenticated_management_and_exact_rollback_not_bound", "existing_sessions_and_local_runner_execution_not_fenced", "login_shell_startup_files_and_loader_environment_not_fenced", "direct_db_credentials_and_global_sessions_not_fenced", "business_services_schedulers_cron_and_external_writers_not_fenced", "environment_repository_organization_secret_and_ref_policies_not_bound", "atomic_platform_queue_control_not_proven", "single_use_challenge_consumption_and_privileged_executor_not_integrated"}}
	if p == nil {
		return r
	}
	r.PolicySHA256 = p.policyHash
	r.SourceSHA = p.sourceSHA
	r.OperationID = p.operation
	r.RunID = p.run
	r.RequestSHA256 = p.request
	r.TokenSHA256 = p.tokenHash
	r.GitHubSnapshotSHA256 = p.snapshotHash
	r.WorkflowCount = p.workflows
	r.SSHProbeInvocationVerified = true
	return r
}

// AuthorizeProbe verifies fixed SSH grammar, the independently bound key/UID,
// GitHub's cryptographic OIDC origin and two independently complete live snapshots.
// No shell command, client payload, database operation or subprocess is executed.
// The authenticatedKey argument must come from a root-protected per-key forced
// command, never from SSH client environment; installation is a separate gate.
func AuthorizeProbe(ctx context.Context, client HTTPDoer, p Policy, originalCommand, authenticatedKey, token string, loginUID uint32, now time.Time) (*Permit, error) {
	if ctx == nil {
		return nil, ErrPolicy
	}
	started := time.Now()
	if err := p.Validate(now); err != nil {
		return nil, err
	}
	if originalCommand != p.Command() {
		return nil, ErrCommand
	}
	if authenticatedKey != p.SSHKeyFingerprint || loginUID != p.LoginUID {
		return nil, ErrIdentity
	}
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var keys jwks
	raw, err := getJSON(bounded, client, JWKSURL, &keys)
	if err != nil {
		return nil, err
	}
	if _, err = verifyOIDC(token, raw, p, now); err != nil {
		return nil, err
	}
	before, err := readSnapshot(bounded, client, p)
	if err != nil {
		return nil, err
	}
	after, err := readSnapshot(bounded, client, p)
	if err != nil {
		return nil, err
	}
	if before.Digest != after.Digest {
		return nil, ErrChanged
	}
	// Network reads may consume the remaining validity interval. Recheck the
	// signed token and root approval against elapsed monotonic time.
	finished := now.Add(time.Since(started))
	if err = p.Validate(finished); err != nil {
		return nil, err
	}
	if _, err = verifyOIDC(token, raw, p, finished); err != nil {
		return nil, err
	}
	return &Permit{policyHash: p.digest(), sourceSHA: p.SourceSHA, operation: p.OperationID, run: p.RunID + "-" + p.RunAttempt, request: p.RequestSHA256, tokenHash: digest([]byte(token)), snapshotHash: after.Digest, workflows: after.WorkflowCount}, nil
}
