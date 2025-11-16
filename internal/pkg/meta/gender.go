package meta

type Gender uint8

const (
	GenderMale   Gender = 1 // 男性
	GenderFemale Gender = 2 // 女性
	GenderOther  Gender = 0 // 其他
)

// NewGender 创建性别
func NewGender(g uint8) Gender {
	return Gender(g)
}

// Value 获取性别值
func (g Gender) Value() uint8 {
	return uint8(g)
}

// String 获取性别字符串
func (g Gender) String() string {
	switch g {
	case GenderMale:
		return "男"
	case GenderFemale:
		return "女"
	case GenderOther:
		return "其他"
	default:
		return "未知"
	}
}
