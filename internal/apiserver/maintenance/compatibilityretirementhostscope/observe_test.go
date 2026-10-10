package compatibilityretirementhostscope

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These fixtures exercise byte/EOF/recheck semantics only. They never bypass
// Observe's real Linux/root check or establish a host authentication/fence proof.
func fixtureObserver(t *testing.T) *observer {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	o := &observer{root: root, uid: uint32(os.Geteuid()), ctx: context.Background(), files: map[string]witness{}, directories: map[string][]string{}, binaries: map[string]string{}, commandHashes: map[string]string{}, absent: map[string]bool{}, links: map[string]string{}, argvHashes: map[int]string{}}
	for _, n := range []string{"ssh_configuration_sources", "nss_accounts_and_key_sources", "existing_sessions_and_processes", "local_activation_sources"} {
		o.out.Scopes = append(o.out.Scopes, Scope{Name: n, Unknown: []string{}})
	}
	o.command = func(string, ...string) ([]byte, error) { return nil, ErrIncomplete }
	return o
}
func put(t *testing.T, o *observer, path, body string) {
	t.Helper()
	p, _ := o.physical(path)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func binding() Binding {
	return Binding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", RunID: "124-1", HostRole: "server_a", RequestSHA256: strings.Repeat("b", 64), ApprovalSHA256: strings.Repeat("c", 64)}
}
func TestRootScopeActualAuthorityAndBoundedBinding(t *testing.T) {
	if b := binding(); !b.Valid() {
		t.Fatal("valid fixture binding")
	}
	for _, v := range []Binding{{}, {SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", RunID: "124-1", HostRole: "server_b", RequestSHA256: strings.Repeat("b", 64), ApprovalSHA256: strings.Repeat("c", 64)}} {
		if v.Valid() {
			t.Fatal("invalid binding accepted")
		}
	}
	// This test calls the public producer only with a canceled context and an
	// invalid binding. It never reads host files even on a Linux root runner.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Observe(ctx, Binding{}); e == nil {
		t.Fatal("unbound DTO obtained producer")
	}
}
func TestRootScopeExactFileEOFSymlinkForbiddenAndChanged(t *testing.T) {
	o := fixtureObserver(t)
	put(t, o, "/etc/ssh/sshd_config", "Port 22\n")
	raw, e := o.read("/etc/ssh/sshd_config", "sshd_configuration", fileLimit)
	if e != nil || string(raw) != "Port 22\n" {
		t.Fatalf("finite actual bytes: %v", e)
	}
	put(t, o, "/etc/ssh/sshd_config", "Port 23\n")
	if _, e = o.read("/etc/ssh/sshd_config", "sshd_configuration", fileLimit); e == nil {
		t.Fatal("different baseline accepted")
	}
	put(t, o, "/etc/shadow", "forbidden-body")
	if _, e = o.read("/etc/shadow", "anything", fileLimit); !errors.Is(e, ErrBinding) {
		t.Fatal("shadow read")
	}
	target, _ := o.physical("/etc/ssh/key")
	if e := os.Symlink("sshd_config", target); e != nil {
		t.Fatal(e)
	}
	if _, e = o.read("/etc/ssh/key", "ssh_public_authorization", fileLimit); e == nil {
		t.Fatal("followed arbitrary source symlink")
	}
}
func TestRootScopeIncludeMatchAndDynamicProviderBodyFree(t *testing.T) {
	o := fixtureObserver(t)
	secret := "fixture-credential-body-should-never-be-output"
	put(t, o, "/etc/ssh/sshd_config", "Include sshd_config.d/*.conf\nMatch User alice Address 192.0.2.*\nAuthorizedKeysCommand /usr/bin/provider --token="+secret+"\nTrustedUserCAKeys /etc/ssh/public_ca\n")
	put(t, o, "/etc/ssh/sshd_config.d/one.conf", "PasswordAuthentication no\n")
	put(t, o, "/etc/ssh/public_ca", "ssh-ed25519 public-fixture-blob\n")
	visited := map[string]bool{}
	o.readSSHConfig("/etc/ssh/sshd_config", visited, 0)
	if len(visited) != 2 || len(o.out.Entries) != 3 {
		t.Fatalf("actual includes/typed domains: %d/%d", len(visited), len(o.out.Entries))
	}
	if !contains(o.scope("ssh_configuration_sources").Unknown, "all_match_authentication_domains_not_exhaustively_proven") || !contains(o.scope("nss_accounts_and_key_sources").Unknown, "dynamic_key_or_principal_provider_not_exhaustively_proven") {
		t.Fatal("dynamic/Match incorrectly closed")
	}
	raw, e := json.Marshal(o.out)
	if e != nil || strings.Contains(string(raw), secret) || strings.Contains(string(raw), "alice") || strings.Contains(string(raw), "/usr/bin/provider") {
		t.Fatal("raw source/subject leaked")
	}
}
func TestRootScopeNSSRealSameCountMismatchRemainsUnknown(t *testing.T) {
	o := fixtureObserver(t)
	local := "alice:x:1001:1001::/home/alice:/bin/bash\n"
	put(t, o, "/etc/passwd", local)
	put(t, o, "/etc/group", "alice:x:1001:\n")
	put(t, o, "/etc/nsswitch.conf", "passwd: files sss\ngroup: files sss\n")
	o.command = func(kind string, args ...string) ([]byte, error) {
		if kind == "nss_passwd" {
			return []byte(strings.ReplaceAll(local, "alice", "different")), nil
		}
		if kind == "nss_group" {
			return []byte("alice:x:1001:\n"), nil
		}
		return nil, ErrIncomplete
	}
	o.observeAccounts()
	g := o.scope("nss_accounts_and_key_sources").Unknown
	if !contains(g, "external_or_conditional_nss_backend_not_exhaustively_proven") || !contains(g, "native_nss_differs_from_local_accounts") {
		t.Fatal("same-count external NSS falsely complete")
	}
}

func TestRootScopeFiniteAccountReadFailuresNeverComplete(t *testing.T) {
	for _, failure := range []string{"nss-command", "nss-schema", "groups", "startup", "public-key", "pam", "home"} {
		t.Run(failure, func(t *testing.T) {
			o := fixtureObserver(t)
			local := "alice:x:1001:1001::/home/alice:/bin/bash\n"
			if failure == "home" {
				local = strings.ReplaceAll(local, "/home/alice", "relative-home")
			}
			put(t, o, "/etc/passwd", local)
			put(t, o, "/etc/group", "alice:x:1001:\n")
			put(t, o, "/etc/nsswitch.conf", "passwd: files\ngroup: files\n")
			o.command = func(kind string, args ...string) ([]byte, error) {
				if kind == "nss_passwd" {
					if failure == "nss-schema" {
						return []byte("malformed-account\n"), nil
					}
					return []byte(local), nil
				}
				if kind == "nss_group" && failure != "nss-command" {
					return []byte("alice:x:1001:\n"), nil
				}
				return nil, ErrIncomplete
			}
			paths := map[string]string{"groups": "/etc/group", "startup": "/home/alice/.profile", "public-key": "/home/alice/.ssh/authorized_keys", "pam": "/etc/pam.d/sshd"}
			if path := paths[failure]; path != "" {
				p, _ := o.physical(path)
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if err := os.Symlink("unread-source", p); err != nil {
					t.Fatal(err)
				}
			}
			o.observeAccounts()
			if o.scope("nss_accounts_and_key_sources").EnumerationComplete {
				t.Fatal("failed selected file/native schema read claimed finite EOF")
			}
		})
	}
}

func TestRootScopeFiniteAccountEOFDoesNotCloseSemanticGaps(t *testing.T) {
	o := fixtureObserver(t)
	local := "alice:x:1001:1001::/home/alice:/bin/bash\n"
	put(t, o, "/etc/passwd", local)
	put(t, o, "/etc/group", "alice:x:1001:\n")
	put(t, o, "/etc/nsswitch.conf", "passwd: files sss\ngroup: files\n")
	put(t, o, "/home/alice/.profile", "run-indirect-command\n")
	o.command = func(kind string, args ...string) ([]byte, error) {
		if kind == "nss_passwd" {
			return []byte(local), nil
		}
		if kind == "nss_group" {
			return []byte("alice:x:1001:\n"), nil
		}
		return nil, ErrIncomplete
	}
	o.observeAccounts()
	s := o.scope("nss_accounts_and_key_sources")
	if !s.EnumerationComplete || !contains(s.Unknown, "external_or_conditional_nss_backend_not_exhaustively_proven") || !contains(s.Unknown, "account_startup_indirect_execution_not_proven") || !contains(s.Unknown, "pam_and_dynamic_authentication_modules_not_exhaustively_proven") {
		t.Fatal("finite EOF erased unknown authentication/indirect execution scope")
	}
}
func TestRootScopeActivationSocketSymlinkAndFiniteDirectoryRecheck(t *testing.T) {
	o := fixtureObserver(t)
	put(t, o, "/etc/systemd/system/qs.socket", "[Socket]\nListenStream=1234\n")
	put(t, o, "/etc/systemd/system/qs.service", "[Service]\nExecStart=/bin/task --credential=private-fixture-token\n")
	p, _ := o.physical("/etc/systemd/system/multi-user.target")
	if e := os.Symlink("qs.service", p); e != nil {
		t.Fatal(e)
	}
	o.command = func(kind string, args ...string) ([]byte, error) {
		switch kind {
		case "units", "unit_files":
			return []byte("qs.service enabled\nqs.socket enabled\n"), nil
		case "unit":
			if len(args) != 2 || args[0] != "qs.service" || args[1] != "qs.socket" {
				t.Fatal("batch fixed listing binding")
			}
			return []byte("Id=qs.service\nMainPID=0\n\nId=qs.socket\nMainPID=0\n"), nil
		}
		return nil, ErrIncomplete
	}
	o.observeActivation()
	if !unitPattern.MatchString("qs.socket") || unitPattern.MatchString("--all") || len(o.links) != 1 {
		t.Fatal("actual socket/symlink source not observed")
	}
	raw, _ := json.Marshal(o.out)
	if strings.Contains(string(raw), "private-fixture-token") {
		t.Fatal("raw activation command leaked")
	}
	before := o.directories["/etc/systemd/system"]
	put(t, o, "/etc/systemd/system/new.service", "[Service]\n")
	if _, e := o.list("/etc/systemd/system"); e == nil || len(before) != 3 {
		t.Fatal("new launch source hidden by old EOF")
	}
}
func TestRootScopeCatalogDoesNotMintFenceAndExactBytes(t *testing.T) {
	o := fixtureObserver(t)
	put(t, o, "/etc/machine-id", "fixture-machine\n")
	put(t, o, "/proc/sys/kernel/random/boot_id", "fixture-boot\n")
	put(t, o, "/etc/passwd", "root:x:0:0::/root:/bin/bash\n")
	put(t, o, "/etc/group", "root:x:0:\n")
	put(t, o, "/etc/nsswitch.conf", "passwd: files\ngroup: files\n")
	put(t, o, "/etc/ssh/sshd_config", "Port 22\n")
	result, e := o.observe(binding())
	if e == nil || result.ObservationComplete || result.WriterScopeComplete {
		t.Fatal("missing actual namespaces/proc marked complete")
	}
	if !contains(result.Unknown, "external_database_and_qs_ai_writers_not_observed") {
		t.Fatal("external scope silently inferred")
	}
	for _, v := range result.Capabilities {
		if v {
			t.Fatal("observation minted authority")
		}
	}
	c := result.CatalogSHA256()
	result.ObservedAt = "different"
	result.ElapsedMillis++
	if c != result.CatalogSHA256() {
		t.Fatal("wall-clock changed content catalog")
	}
	result.Files[0].ContentSHA256 = strings.Repeat("f", 64)
	if c == result.CatalogSHA256() {
		t.Fatal("catalog ignored actual source bytes")
	}
}

func TestRootScopeContainerCandidatesExactAndBodyFree(t *testing.T) {
	o := fixtureObserver(t)
	id := strings.Repeat("a", 64)
	image := "sha256:" + strings.Repeat("b", 64)
	secret := "fixture-container-argument-credential"
	o.scope("local_activation_sources").EnumerationComplete = true
	o.command = func(kind string, args ...string) ([]byte, error) {
		switch kind {
		case "docker_list":
			return []byte(id + "\n"), nil
		case "docker_inspect":
			if len(args) != 1 || args[0] != id {
				t.Fatal("roster not bound")
			}
			return []byte(`{"id":"` + id + `","image":"` + image + `","pid":31,"labels":{"private":"` + secret + `"},"entrypoint":["/qs-api"],"command":["--token=` + secret + `"],"restart":{"Name":"unless-stopped"},"mounts":[]}` + "\n"), nil
		}
		return nil, ErrIncomplete
	}
	o.observeContainers()
	if len(o.out.Containers) != 1 || o.out.Containers[0].ID != id || o.out.Containers[0].PID != 31 || o.out.Containers[0].CommandSHA256 == "" {
		t.Fatal("exact CID/PID/image candidate missing")
	}
	raw, _ := json.Marshal(o.out)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "--token") {
		t.Fatal("Docker argument/label body leaked")
	}
	if !contains(o.scope("local_activation_sources").Unknown, "docker_socket_and_other_container_writer_admission_not_fenced") {
		t.Fatal("Docker catalog claimed admission")
	}
	bad := fixtureObserver(t)
	bad.scope("local_activation_sources").EnumerationComplete = true
	bad.command = func(kind string, args ...string) ([]byte, error) {
		if kind == "docker_list" {
			return []byte(id + "\n"), nil
		}
		return []byte("{}\n"), nil
	}
	bad.observeContainers()
	if bad.scope("local_activation_sources").EnumerationComplete {
		t.Fatal("partial selected response claimed EOF")
	}
}
