package rest

import (
	"testing"
)

// k6 mixed.js 访问 apiserver 的路径与 OpenAPI 对齐守卫。
func TestApiserverOpenAPICoversK6PerfPaths(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	required := map[string][]string{
		"/testees":                          {"get"},
		"/statistics/overview":              {"get"},
		"/statistics/system":                {"get"},
		"/statistics/questionnaires/{code}": {"get"},
		"/evaluations/assessments":          {"get"},
	}
	for path, methods := range required {
		for _, method := range methods {
			assertOpenAPIOperation(t, spec, path, method)
		}
	}
}
