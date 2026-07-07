package legacy

// Flat 迁移-仅 类型 不得 be 用于 creating 新的草稿模型。
const (
	KindMBTIMigration = "mbti"
	KindSBTIMigration = "sbti"
)

// KindMapping 解析 deprecated flat 类型 到 v2 身份 triples（仅 scale 迁移读路径保留）。
func KindMapping(kind string) (mappedKind, subKind, algorithm string, ok bool) {
	switch kind {
	case "scale":
		return "scale", "", "scale_default", true
	default:
		return "", "", "", false
	}
}

// IsMigrationOnlyKind 报告旧版 flat 类型 that 不得 be 用于 新的草稿模型。
func IsMigrationOnlyKind(kind string) bool {
	switch kind {
	case KindMBTIMigration, KindSBTIMigration:
		return true
	default:
		return false
	}
}
