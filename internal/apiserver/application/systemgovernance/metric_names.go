package systemgovernance

import "strings"

func metricNamePart(value string) string {
	replacer := strings.NewReplacer(".", "_", "-", "_", ":", "_", "/", "_")
	return replacer.Replace(value)
}
