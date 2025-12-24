package statistics

// ==================== 统计模块全局常量 ====================

const (
	// DefaultOrgID 默认机构ID（单租户场景）
	DefaultOrgID int64 = 1
)

// OriginTypes 测评来源类型列表
var OriginTypes = []string{
	"adhoc",     // 一次性测评
	"plan",      // 测评计划
	"screening", // 入校筛查
}

