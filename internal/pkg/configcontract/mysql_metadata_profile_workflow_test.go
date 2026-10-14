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
	t.Helper()
	condition = strings.TrimSpace(condition)
	condition = strings.TrimSuffix(strings.TrimPrefix(condition, "${{"), "}}")
	for key, value := range map[string]string{"github.event_name": "workflow_dispatch", "github.event.inputs.operation": operation, "github.event.inputs.database": database} {
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
	if len(workflow.Jobs) != 14 {
		t.Fatal("db-ops job inventory changed; review metadata exclusion")
	}
	for _, operation := range []string{"metadata-visibility-profile", "metadata-visibility-admin-profile"} {
		for _, database := range []string{"mysql", "mongodb", "redis", "all"} {
			for name, job := range workflow.Jobs {
				got := metadataJobEligible(t, job.If, operation, database)
				want := name == "mysql-metadata-visibility-profile" && database == "mysql"
				if got != want {
					t.Fatalf("%s/%s: job %s eligible=%t want=%t", operation, database, name, got, want)
				}
			}
		}
	}
}
