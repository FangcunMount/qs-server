package statistics

// parseDailyKey 解析每日统计键
// 格式：stats:daily:{org_id}:{type}:{key}:{date}
func parseDailyKey(key string) []string {
	parts := make([]string, 0)
	current := ""
	for _, char := range key {
		if char == ':' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

