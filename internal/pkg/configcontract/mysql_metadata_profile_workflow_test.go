package configcontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Evaluate the small boolean subset used by db-ops job conditions. Unknown
// syntax fails the test rather than treating a possibly eligible job as false.
func metadataJobEligible(t *testing.T, condition, operation, database string) bool {
	return metadataJobEligibleForEvent(t, condition, operation, database, "workflow_dispatch")
}

func metadataJobEligibleForEvent(t *testing.T, condition, operation, database, event string) bool {
	t.Helper()
	condition = strings.TrimSpace(condition)
	condition = strings.TrimSuffix(strings.TrimPrefix(condition, "${{"), "}}")
	for key, value := range map[string]string{"github.event_name": event, "github.event.inputs.operation": operation, "github.event.inputs.database": database} {
		condition = strings.ReplaceAll(condition, key, "'"+value+"'")
	}
	var parse func(string) bool
	parse = func(expression string) bool {
		expression = strings.TrimSpace(expression)
		depth := 0
		quoted := false
		for _, operator := range []string{"||", "&&"} {
			depth, quoted = 0, false
			for i := 0; i < len(expression); i++ {
				switch expression[i] {
				case '\'':
					quoted = !quoted
				case '(':
					if !quoted {
						depth++
					}
				case ')':
					if !quoted {
						depth--
					}
				}
				if !quoted && depth == 0 && strings.HasPrefix(expression[i:], operator) {
					left, right := parse(expression[:i]), parse(expression[i+2:])
					if operator == "||" {
						return left || right
					}
					return left && right
				}
			}
		}
		if strings.HasPrefix(expression, "(") && strings.HasSuffix(expression, ")") {
			return parse(expression[1 : len(expression)-1])
		}
		if match := regexp.MustCompile(`^'([^']*)'\s*==\s*'([^']*)'$`).FindStringSubmatch(expression); match != nil {
			return match[1] == match[2]
		}
		if match := regexp.MustCompile(`^startsWith\('([^']*)',\s*'([^']*)'\)$`).FindStringSubmatch(expression); match != nil {
			return strings.HasPrefix(match[1], match[2])
		}
		t.Fatalf("unreviewed job condition %s", strconv.Quote(expression))
		return false
	}
	return parse(condition)
}

func TestMetadataProfileInputsExcludeEveryOtherDBOpsJob(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/db-ops.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			If string `yaml:"if"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Jobs) != 16 {
		t.Fatal("db-ops job inventory changed; review metadata exclusion")
	}
	for _, operation := range []string{"metadata-visibility-profile", "metadata-visibility-admin-profile", "metadata-readiness-admin"} {
		for _, database := range []string{"mysql", "mongodb", "redis", "all"} {
			for name, job := range workflow.Jobs {
				got := metadataJobEligible(t, job.If, operation, database)
				want := name == "mysql-metadata-visibility-profile" && database == "mysql" && operation != "metadata-readiness-admin"
				want = want || (name == "mysql-metadata-readiness-admin" && database == "mysql" && operation == "metadata-readiness-admin")
				want = want || (name == "mongodb-metadata-admin-profile" && database == "mongodb" && operation == "metadata-visibility-admin-profile")
				if got != want {
					t.Fatalf("%s/%s: job %s eligible=%t want=%t", operation, database, name, got, want)
				}
			}
		}
	}
}

func TestBoundedMetadataReadinessRequiresDispatchAndReviewedRevision(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/db-ops.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			If          string `yaml:"if"`
			Environment string `yaml:"environment"`
			Steps       []struct {
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		name, operation, database, confirmKey, backupKey string
	}{
		{"mysql-metadata-readiness-admin", "metadata-readiness-admin", "mysql", "PROFILE_REVIEW_SOURCE_SHA", "PROFILE_ARCHIVE_INPUT"},
		{"mongodb-metadata-admin-profile", "metadata-visibility-admin-profile", "mongodb", "MONGO_PROFILE_CONFIRM_SHA", "MONGO_PROFILE_ARCHIVE_INPUT"},
	} {
		t.Run(target.name, func(t *testing.T) {
			job, ok := workflow.Jobs[target.name]
			if !ok || job.Environment != "production" || len(job.Steps) < 3 {
				t.Fatal("bounded metadata job missing or environment changed")
			}
			if !metadataJobEligible(t, job.If, target.operation, target.database) || metadataJobEligibleForEvent(t, job.If, target.operation, target.database, "schedule") {
				t.Fatal("bounded metadata job must be dispatch-only")
			}
			gate := job.Steps[0]
			if gate.Uses != "" || gate.Env[target.confirmKey] != "${{ github.event.inputs.governance_confirm }}" || gate.Env[target.backupKey] != "${{ github.event.inputs.backup_name }}" {
				t.Fatal("review and empty archive gate must precede checkout")
			}
			for _, required := range []string{`^[0-9a-f]{40}$`, `"$` + target.confirmKey + `" == "$GITHUB_SHA"`, `[[ -z "$` + target.backupKey + `" ]]`} {
				if !strings.Contains(gate.Run, required) {
					t.Fatal("reviewed revision or empty archive input gate missing")
				}
			}
			for key := range gate.Env {
				if strings.Contains(key, "PASSWORD") || strings.Contains(key, "SSH_KEY") {
					t.Fatal("credentials exposed before revision gate")
				}
			}
			if job.Steps[1].Uses != "actions/checkout@v6" || job.Steps[1].With["ref"] != "${{ github.sha }}" {
				t.Fatal("bounded metadata checkout must bind immutable source")
			}
		})
	}
}

func TestMetadataNativeCIUsesOwnedRequiredFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Environment string `yaml:"environment"`
			Services    map[string]struct {
				Image string   `yaml:"image"`
				Ports []string `yaml:"ports"`
			} `yaml:"services"`
			Steps []struct {
				Run string            `yaml:"run"`
				Env map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	mysql, mongo := workflow.Jobs["metadata-readiness-mysql-native"], workflow.Jobs["metadata-readiness-mongodb-native"]
	if mysql.Environment != "" || mongo.Environment != "" || mysql.Services["mysql"].Image != "mysql:8.0.36" || strings.Join(mysql.Services["mysql"].Ports, " ") != "3306/tcp" {
		t.Fatal("native fixtures must be isolated from production and use a random MySQL host port")
	}
	mysqlRuns, mongoRuns := "", ""
	mysqlRequired := false
	for _, step := range mysql.Steps {
		mysqlRuns += step.Run
		mysqlRequired = mysqlRequired || step.Env["QS_METADATA_PROFILE_NATIVE_REQUIRED"] == "1"
	}
	for _, step := range mongo.Steps {
		mongoRuns += step.Run
	}
	if !mysqlRequired || !strings.Contains(mysqlRuns, "make test-metadata-readiness-mysql-native") || !strings.Contains(mysqlRuns, "qs_profile_owned_${GITHUB_RUN_ID}") {
		t.Fatal("MySQL required native selector or owned schema guard missing")
	}
	for _, value := range []string{"mongo@sha256:43fddee7e532a920f3dfdee9e8f4834398c155c26bcb92d790cc1cd3c630fc40", "qs.mongo-metadata.fixture", "--publish 127.0.0.1::27017", "--network-alias mongo", "--keyFile /tmp/metadata-fixture.key", "export QS_MONGO_METADATA_NATIVE_REQUIRED=1", "make test-metadata-readiness-mongodb-native"} {
		if !strings.Contains(mongoRuns, value) {
			t.Fatal("pinned authenticated owned Mongo native fixture guard missing")
		}
	}
	for _, selector := range []struct{ path, name string }{
		{"scripts/oneoff/mysql_metadata_profile/native_test.go", "TestMetadataProfileNativeOwnedMySQL"},
		{"scripts/oneoff/mysql_metadata_profile/readiness_native_test.go", "TestMetadataReadinessNativeOwnedMySQL"},
		{"scripts/oneoff/mongodb_metadata_profile/native_test.go", "TestMongoMetadataProfileNativeOwnedReplicaSet"},
	} {
		source, err := os.ReadFile(filepath.Join(repoRoot(t), selector.path))
		if err != nil || !strings.Contains(string(source), "func "+selector.name+"(") {
			t.Fatal("required native test selector has no implementation")
		}
	}
}
