package configcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type collectionComposeContract struct {
	Services map[string]struct {
		ContainerName string   `yaml:"container_name"`
		Ports         []string `yaml:"ports"`
		Expose        []string `yaml:"expose"`
		MemLimit      string   `yaml:"mem_limit"`
		CPUs          string   `yaml:"cpus"`
		Environment   []string `yaml:"environment"`
		Volumes       []string `yaml:"volumes"`
		Logging       struct {
			Driver  string            `yaml:"driver"`
			Options map[string]string `yaml:"options"`
		} `yaml:"logging"`
		Networks map[string]struct {
			Aliases []string `yaml:"aliases"`
		} `yaml:"networks"`
	} `yaml:"services"`
}

func TestCollectionComposeSupportsTwoReplicas(t *testing.T) {
	t.Parallel()

	composePath := filepath.Join(repoRoot(t), "build", "docker", "docker-compose.prod.yml")
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read production compose: %v", err)
	}

	var contract collectionComposeContract
	if err := yaml.Unmarshal(content, &contract); err != nil {
		t.Fatalf("parse production compose: %v", err)
	}
	service, ok := contract.Services["server"]
	if !ok {
		t.Fatal("production compose must declare collection service key server")
	}
	if _, legacy := contract.Services["qs-collection-server"]; legacy {
		t.Fatal("production compose must not retain duplicated collection service key qs-collection-server")
	}
	if service.ContainerName != "" {
		t.Fatalf("collection container_name = %q, want empty so compose can scale", service.ContainerName)
	}
	if len(service.Ports) != 0 {
		t.Fatalf("collection host ports = %v, want none so replicas do not conflict", service.Ports)
	}
	for _, port := range []string{"8080", "6060"} {
		if !slices.Contains(service.Expose, port) {
			t.Errorf("collection expose = %v, want %s", service.Expose, port)
		}
	}
	if service.MemLimit != "1536m" || service.CPUs != "2" {
		t.Fatalf("collection per-replica resources = cpu %q memory %q, want 2/1536m", service.CPUs, service.MemLimit)
	}
	for _, env := range []string{"GOMEMLIMIT=1152MiB", "GOMAXPROCS=2"} {
		if !slices.Contains(service.Environment, env) {
			t.Errorf("collection environment = %v, want %q", service.Environment, env)
		}
	}
	for _, volume := range service.Volumes {
		if strings.Contains(volume, "/data/logs") {
			t.Fatalf("collection replica must not share rotating log volume %q", volume)
		}
	}
	if service.Logging.Driver != "json-file" ||
		service.Logging.Options["max-size"] != "100m" ||
		service.Logging.Options["max-file"] != "10" {
		t.Fatalf("collection logging = %#v, want bounded per-container json-file logs", service.Logging)
	}
	for _, network := range []string{"qs-network", "infra-network"} {
		if !slices.Contains(service.Networks[network].Aliases, "qs-collection-server") {
			t.Errorf("collection network %s aliases = %v, want stable qs-collection-server DNS", network, service.Networks[network].Aliases)
		}
	}
}

func TestWorkerComposeUsesReadableScalableNameAndStableDNS(t *testing.T) {
	t.Parallel()

	composePath := filepath.Join(repoRoot(t), "build", "docker", "docker-compose.prod.yml")
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read production compose: %v", err)
	}

	var contract collectionComposeContract
	if err := yaml.Unmarshal(content, &contract); err != nil {
		t.Fatalf("parse production compose: %v", err)
	}
	service, ok := contract.Services["runtime"]
	if !ok {
		t.Fatal("production compose must declare worker service key runtime")
	}
	if _, legacy := contract.Services["qs-worker"]; legacy {
		t.Fatal("production compose must not retain duplicated worker service key qs-worker")
	}
	if service.ContainerName != "" {
		t.Fatalf("worker container_name = %q, want empty so compose can scale", service.ContainerName)
	}
	for _, network := range []string{"qs-network", "infra-network"} {
		if !slices.Contains(service.Networks[network].Aliases, "qs-worker") {
			t.Errorf("worker network %s aliases = %v, want stable qs-worker DNS", network, service.Networks[network].Aliases)
		}
	}
}

func TestCollectionDeploymentPipelineScalesAndVerifiesEveryReplica(t *testing.T) {
	t.Parallel()

	workflow := readDeploymentContractFile(t, ".github", "workflows", "cd.yml")
	for _, required := range []string{
		"collection_replicas:",
		"vars.QS_COLLECTION_REPLICAS || '2'",
		"COLLECTION_REPLICAS: ${{ env.COLLECTION_REPLICAS }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("cd workflow must contain %q", required)
		}
	}

	upload := readDeploymentContractFile(t, "scripts", "cd", "runner-upload-and-deploy.sh")
	if !strings.Contains(upload, `emit_export COLLECTION_REPLICAS "${COLLECTION_REPLICAS:-}"`) {
		t.Error("runner upload must pass COLLECTION_REPLICAS to the target host")
	}

	remote := readDeploymentContractFile(t, "scripts", "cd", "remote-deploy.sh")
	for _, required := range []string{
		`COLLECTION_COMPOSE_PROJECT="${COLLECTION_COMPOSE_PROJECT:-qs-collection}"`,
		`WORKER_COMPOSE_PROJECT="${WORKER_COMPOSE_PROJECT:-qs-worker}"`,
		`: "${COLLECTION_REPLICAS:?COLLECTION_REPLICAS is required}"`,
		`--scale "${COMPOSE_SERVICE}=${COLLECTION_REPLICAS}"`,
		`ps --status running -q "$COMPOSE_SERVICE"`,
		`http://127.0.0.1:${INTERNAL_HTTP_PORT}/serve-readyz`,
		`http://127.0.0.1:${INTERNAL_HTTP_PORT}/readyz`,
		`grep -Eq 'HTTP/[0-9.]+[[:space:]]+404'`,
		`verify_collection_images`,
		`remove_legacy_compose_service "$COLLECTION_COMPOSE_PROJECT" "qs-collection-server"`,
		`remove_legacy_compose_service "$WORKER_COMPOSE_PROJECT" "qs-worker"`,
	} {
		if !strings.Contains(remote, required) {
			t.Errorf("remote deploy must contain %q", required)
		}
	}

	metadata := readDeploymentContractFile(t, "scripts", "cd", "image-metadata.sh")
	for _, required := range []string{
		"COMPOSE_SERVICE=server",
		"COMPOSE_SERVICE=runtime",
	} {
		if !strings.Contains(metadata, required) {
			t.Errorf("image metadata must contain %q", required)
		}
	}

	ping := readDeploymentContractFile(t, ".github", "workflows", "ping-runner.yml")
	for _, required := range []string{
		"EXPECTED_COLLECTION_REPLICAS",
		"com.docker.compose.project=qs-collection",
		"com.docker.compose.service=server",
		"com.docker.compose.project=qs-worker",
		"com.docker.compose.service=runtime",
		`http://127.0.0.1:8080/serve-readyz`,
		"https://collect.fangcunmount.cn/health",
		"nginx -T",
		"1.27.3",
		"getent ahostsv4 qs-collection-server",
	} {
		if !strings.Contains(ping, required) {
			t.Errorf("runner health workflow must contain %q", required)
		}
	}

	remoteNginxContractRequirements := []string{
		`verify_collection_nginx preflight`,
		`verify_collection_nginx install-and-verify`,
		`NGINX_CONFIG_BACKUP_DIR="$BACKUP_DIR"`,
		`PRIVILEGE_RUNNER="$SUDO"`,
	}
	for _, required := range remoteNginxContractRequirements {
		if !strings.Contains(remote, required) {
			t.Errorf("remote deploy must contain Nginx contract %q", required)
		}
	}

	preparePackage := readDeploymentContractFile(t, "scripts", "cd", "prepare-package.sh")
	if !strings.Contains(preparePackage, "verify-collection-nginx.sh") {
		t.Error("deployment package must include verify-collection-nginx.sh")
	}
	verifier := readDeploymentContractFile(t, "scripts", "cd", "verify-collection-nginx.sh")
	for _, required := range []string{
		`NGINX_MIN_VERSION="${NGINX_MIN_VERSION:-1.27.3}"`,
		`run_privileged docker exec "$NGINX_CONTAINER" nginx -T`,
		`getent ahostsv4 "$COLLECTION_DNS_NAME"`,
		`ROUTING_PROBE_REQUESTS="${ROUTING_PROBE_REQUESTS:-40}"`,
		`/^gin_requests_total\{/`,
		`rollback_config()`,
	} {
		if !strings.Contains(verifier, required) {
			t.Errorf("collection Nginx verifier must contain %q", required)
		}
	}
	for _, forbidden := range []string{
		`$SUDO env`,
		`verify-collection-nginx.sh must run as root`,
	} {
		if strings.Contains(remote, forbidden) || strings.Contains(verifier, forbidden) {
			t.Errorf("collection Nginx deployment must not contain privileged script execution %q", forbidden)
		}
	}

	ci := readDeploymentContractFile(t, ".github", "workflows", "ci.yml")
	for _, required := range []string{
		"github.com/rhysd/actionlint/cmd/actionlint@v1.7.7",
		"bash -n scripts/cd/*.sh",
		"docker compose -f build/docker/docker-compose.prod.yml -f - config -q",
		"'  server:'",
		"'  runtime:'",
	} {
		if !strings.Contains(ci, required) {
			t.Errorf("CI deployment contracts must contain %q", required)
		}
	}
}

func TestRedisDegradedSubmitAcceptanceContract(t *testing.T) {
	t.Parallel()

	runner := readDeploymentContractFile(t, "scripts", "perf", "run-submit-redis-degraded.sh")
	for _, required := range []string{
		`REDIS_FAILURE_CONFIRMED`,
		`PERF_ISOLATED_ENV`,
		`/serve-readyz`,
		`/readyz`,
		`want 200/503`,
		`strategy=\"local_fallback\"`,
		`metric_total "${ARTIFACT_DIR}/${container_id}-metrics-before.prom" degraded_open`,
		`metric_total "${ARTIFACT_DIR}/${container_id}-metrics-before.prom" rate_limited`,
		`metrics-before.prom`,
		`metrics-after.prom`,
		`k6-summary.json`,
	} {
		if !strings.Contains(runner, required) {
			t.Errorf("Redis degraded submit runner must contain %q", required)
		}
	}

	scenario := readDeploymentContractFile(t, "scripts", "perf", "k6-submit-redis-degraded.js")
	for _, required := range []string{
		`'low', 'global_overload', 'user_overload'`,
		`SUBMIT_CASES_JSON`,
		`response.status === 202`,
		`response.status === 429`,
		`Retry-After`,
		`degraded_submit_rate_limited_total`,
	} {
		if !strings.Contains(scenario, required) {
			t.Errorf("Redis degraded submit k6 scenario must contain %q", required)
		}
	}

	coalescing := readDeploymentContractFile(t, "scripts", "perf", "run-submit-coalescing.sh")
	if !strings.Contains(coalescing, `${base_url}/readyz`) {
		t.Error("SubmitCoalescer acceptance must keep strict /readyz preflight")
	}
}

func TestCollectionNginxUsesDynamicDockerDNSInsideUpstream(t *testing.T) {
	t.Parallel()

	config := readDeploymentContractFile(t, "configs", "nginx", "conf.d", "collect.fangcunmount.cn.conf")
	upstream := nginxNamedBlock(t, config, "upstream collect-api")

	for _, required := range []string{
		"zone collect_api 64k;",
		"resolver 127.0.0.11 valid=10s ipv6=off;",
		"resolver_timeout 5s;",
		"server qs-collection-server:8080 resolve;",
	} {
		if !strings.Contains(upstream, required) {
			t.Errorf("collect-api upstream must contain %q", required)
		}
	}
	for _, forbidden := range []string{"ip_hash;", "weight=", "backup"} {
		if strings.Contains(upstream, forbidden) {
			t.Errorf("collect-api upstream must not contain %q", forbidden)
		}
	}
	if count := strings.Count(config, "upstream collect-api"); count != 1 {
		t.Fatalf("upstream collect-api count = %d, want 1", count)
	}
	if count := strings.Count(config, "resolver 127.0.0.11"); count != 1 {
		t.Fatalf("Docker resolver count = %d, want exactly one inside collect-api", count)
	}
}

func TestCollectionNginxMinimumVersionComparison(t *testing.T) {
	t.Parallel()

	script := filepath.Join(repoRoot(t), "scripts", "cd", "verify-collection-nginx.sh")
	tests := []struct {
		name    string
		current string
		wantOK  bool
	}{
		{name: "below minimum", current: "1.27.2", wantOK: false},
		{name: "at minimum", current: "1.27.3", wantOK: true},
		{name: "above minimum", current: "1.28.0", wantOK: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("bash", script, "--version-at-least", tt.current, "1.27.3")
			err := cmd.Run()
			if tt.wantOK && err != nil {
				t.Fatalf("version %s should satisfy minimum: %v", tt.current, err)
			}
			if !tt.wantOK && err == nil {
				t.Fatalf("version %s should not satisfy minimum", tt.current)
			}
		})
	}
}

func TestCollectionNginxPreflightUsesGranularPrivilegeRunner(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "privileged-commands.log")
	runnerPath := filepath.Join(tempDir, "privilege-runner")
	runner := `#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' "$*" >>"$PRIVILEGE_LOG"
case "$*" in
  "docker inspect nginx --format {{.State.Running}}")
    printf '%s\n' true
    ;;
  "docker exec nginx nginx -v")
    printf '%s\n' "nginx version: nginx/1.27.3"
    ;;
  *)
    printf 'unexpected privileged command: %s\n' "$*" >&2
    exit 97
    ;;
esac
`
	if err := os.WriteFile(runnerPath, []byte(runner), 0o700); err != nil {
		t.Fatalf("write fake privilege runner: %v", err)
	}

	script := filepath.Join(repoRoot(t), "scripts", "cd", "verify-collection-nginx.sh")
	cmd := exec.Command("bash", script, "preflight")
	cmd.Env = append(os.Environ(),
		"PRIVILEGE_RUNNER="+runnerPath,
		"PRIVILEGE_LOG="+logPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run rootless collection Nginx preflight: %v\n%s", err, output)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read privileged command log: %v", err)
	}
	logText := string(logged)
	for _, required := range []string{
		"docker inspect nginx --format {{.State.Running}}",
		"docker exec nginx nginx -v",
	} {
		if !strings.Contains(logText, required) {
			t.Errorf("privileged command log must contain %q, got:\n%s", required, logText)
		}
	}
}

func TestCollectionProductionLoggingIsReplicaSafe(t *testing.T) {
	t.Parallel()

	config := readDeploymentContractFile(t, "configs", "collection-server.prod.yaml")
	for _, forbidden := range []string{
		"/data/logs/qs/qs-collection-server.log",
		"/data/logs/qs/qs-collection-server-error.log",
	} {
		if strings.Contains(config, forbidden) {
			t.Errorf("collection production config must not share rotating log file %q across replicas", forbidden)
		}
	}
	for _, required := range []string{
		"go-mem-limit: \"1152MiB\"",
		"- stdout",
		"- stderr",
	} {
		if !strings.Contains(config, required) {
			t.Errorf("collection production config must contain %q", required)
		}
	}
}

func nginxNamedBlock(t *testing.T, config, name string) string {
	t.Helper()

	start := strings.Index(config, name)
	if start < 0 {
		t.Fatalf("Nginx config does not contain %q", name)
	}
	openOffset := strings.Index(config[start:], "{")
	if openOffset < 0 {
		t.Fatalf("Nginx block %q has no opening brace", name)
	}
	open := start + openOffset
	depth := 0
	for i := open; i < len(config); i++ {
		switch config[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return config[start : i+1]
			}
		}
	}
	t.Fatalf("Nginx block %q has no matching closing brace", name)
	return ""
}

func readDeploymentContractFile(t *testing.T, parts ...string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(append([]string{repoRoot(t)}, parts...)...))
	if err != nil {
		t.Fatalf("read deployment contract file %v: %v", parts, err)
	}
	return string(content)
}
