package seedconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadNormalizesAndValidatesPlanSubmit(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "seeddata.yaml")
	content := `
global:
  orgId: 1
api:
  baseUrl: "https://qs.example.com"
dailySimulation:
  clinicianIds: ["1001"]
  targetType: "scale"
  targetCode: "SAS"
  planIds: ["614333603412718126"]
planSubmit:
  planIds: ["614333603412718126", "614187067651404334"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.PlanSubmit.PlanIDStrings()) != 2 {
		t.Fatalf("unexpected plan ids: %#v", cfg.PlanSubmit.PlanIDStrings())
	}
	if cfg.PlanSubmit.PlanIDStrings()[0] != "614333603412718126" || cfg.PlanSubmit.PlanIDStrings()[1] != "614187067651404334" {
		t.Fatalf("unexpected normalized plan ids: %#v", cfg.PlanSubmit.PlanIDStrings())
	}
	if cfg.PlanSubmit.Workers != DefaultPlanSubmitWorkers {
		t.Fatalf("unexpected default plan workers: %d", cfg.PlanSubmit.Workers)
	}
	if cfg.DailySimulation.CountPerRun != DefaultDailySimulationCountPerRun {
		t.Fatalf("unexpected default count per run: %d", cfg.DailySimulation.CountPerRun)
	}
	if cfg.DailySimulation.RunAt != DefaultDailySimulationRunAt {
		t.Fatalf("unexpected default runAt: %q", cfg.DailySimulation.RunAt)
	}
}

func TestLoadRequiresPlanSubmitPlanID(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "seeddata.yaml")
	content := `
global:
  orgId: 1
api:
  baseUrl: "https://qs.example.com"
dailySimulation:
  clinicianIds: ["1001"]
  targetType: "scale"
  targetCode: "SAS"
  planIds: ["614333603412718126"]
planSubmit:
  workers: 2
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "planSubmit.planIds is required") {
		t.Fatalf("expected missing planSubmit.planIds error, got %v", err)
	}
}

func TestLoadOverridesIAMCredentialsFromEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "seeddata.yaml")
	content := `
global:
  orgId: 1
api:
  baseUrl: "https://qs.example.com"
iam:
  username: "yaml-user"
  password: "yaml-pass"
dailySimulation:
  clinicianIds: ["1001"]
  targetType: "scale"
  targetCode: "SAS"
  planIds: ["614333603412718126"]
planSubmit:
  planIds: ["614333603412718126"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("IAM_USERNAME", "env-user")
	t.Setenv("IAM_PASSWORD", "env-pass")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IAM.Username != "env-user" {
		t.Fatalf("unexpected iam username: %q", cfg.IAM.Username)
	}
	if cfg.IAM.Password != "env-pass" {
		t.Fatalf("unexpected iam password: %q", cfg.IAM.Password)
	}
}
