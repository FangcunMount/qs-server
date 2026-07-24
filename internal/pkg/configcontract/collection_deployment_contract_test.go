package configcontract

import (
	"os"
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
		`http://127.0.0.1:${INTERNAL_HTTP_PORT}/readyz`,
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
		`http://127.0.0.1:8080/readyz`,
		"https://collect.fangcunmount.cn/health",
	} {
		if !strings.Contains(ping, required) {
			t.Errorf("runner health workflow must contain %q", required)
		}
	}

	nginx := readDeploymentContractFile(t, "configs", "nginx", "conf.d", "collect.fangcunmount.cn.conf")
	for _, required := range []string{
		"resolver 127.0.0.11",
		"server qs-collection-server:8080 resolve;",
	} {
		if !strings.Contains(nginx, required) {
			t.Errorf("collection Nginx service discovery must contain %q", required)
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

func readDeploymentContractFile(t *testing.T, parts ...string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(append([]string{repoRoot(t)}, parts...)...))
	if err != nil {
		t.Fatalf("read deployment contract file %v: %v", parts, err)
	}
	return string(content)
}
