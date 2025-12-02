package questionnaire

import (
	"errors"
	"strings"
)

// Status 问卷状态
type Status uint8

const (
	STATUS_DRAFT     Status = 0 // 草稿
	STATUS_PUBLISHED Status = 1 // 已发布
	STATUS_ARCHIVED  Status = 2 // 已归档
)

// Value 获取状态值
func (s Status) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s Status) String() string {
	statusMap := map[uint8]string{
		0: "草稿",
		1: "已发布",
		2: "已归档",
	}
	return statusMap[s.Value()]
}

// Version 问卷版本
type Version string

// NewVersion 创建问卷版本
func NewVersion(value string) Version {
	return Version(value)
}

// Value 获取版本值
func (v Version) Value() string {
	return string(v)
}

// String 获取版本字符串（实现 fmt.Stringer 接口）
func (v Version) String() string {
	return string(v)
}

// Equals 判断版本是否相等
func (v Version) Equals(other Version) bool {
	return v == other
}

// IsEmpty 判断版本是否为空
func (v Version) IsEmpty() bool {
	return v == ""
}

// IncrementMinor 递增小版本号 (第三位)
// 例如：0.0.1 -> 0.0.2, 1.0.5 -> 1.0.6
// 如果格式不是 x.y.z，则尝试智能转换
func (v Version) IncrementMinor() Version {
	s := string(v)

	// 解析版本号，分离出主版本、次版本、小版本
	parts := splitByDot(s)

	if len(parts) >= 3 {
		// 标准格式 x.y.z，递增第三位
		minor := parseNumber(parts[2])
		minor++
		return Version(parts[0] + "." + parts[1] + "." + intToString(minor))
	}

	if len(parts) == 2 {
		// 格式为 x.y，转换为 x.y.1
		return Version(s + ".1")
	}

	if len(parts) == 1 {
		// 格式为 x，转换为 x.0.1
		return Version(s + ".0.1")
	}

	// 无法解析，返回 0.0.1
	return Version("0.0.1")
}

// IncrementMajor 递增大版本号 (第一位) 并重置为 x.0.1
// 例如：0.0.5 -> 1.0.1, 1.0.3 -> 2.0.1, 5.2.8 -> 6.0.1
func (v Version) IncrementMajor() Version {
	s := string(v)
	prefix := ""
	if strings.HasPrefix(s, "v") {
		prefix = "v"
		s = s[1:]
	}

	// 解析版本号
	parts := splitByDot(s)

	if len(parts) >= 1 {
		// 递增第一位，重置后面的为 0.1
		major := parseNumber(parts[0])
		major++
		// 保持带 v 前缀的版本格式，例如 v1 -> v2
		if prefix != "" && len(parts) == 1 {
			return Version(prefix + intToString(major))
		}
		return Version(prefix + intToString(major) + ".0.1")
	}

	// 无法解析，返回 1.0.1
	return Version(prefix + "1.0.1")
}

// parseNumber 解析字符串为数字
func parseNumber(s string) int {
	if !isDigit(s) {
		return 0
	}
	num := 0
	for i := 0; i < len(s); i++ {
		num = num*10 + int(s[i]-'0')
	}
	return num
}

// Validate 验证版本格式是否合法
// 支持格式：v1, v1.0, v1.0.0, 1, 1.0, 1.0.0
func (v Version) Validate() error {
	s := string(v)

	if s == "" {
		return errors.New("version cannot be empty")
	}

	// 去除 "v" 前缀
	if s[0] == 'v' {
		s = s[1:]
	}

	if s == "" {
		return errors.New("version cannot be only 'v'")
	}

	// 验证格式：数字.数字.数字 或 数字
	parts := splitByDot(s)
	for _, part := range parts {
		if part == "" {
			return errors.New("version part cannot be empty")
		}
		if !isDigit(part) {
			return errors.New("version part must be numeric")
		}
	}

	return nil
}

// isDigit 检查字符串是否全为数字
func isDigit(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// splitByDot 按点号分割字符串
func splitByDot(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// intToString 整数转字符串
func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// QuestionType 题型
type QuestionType string

func (t QuestionType) Value() string {
	return string(t)
}

const (
	TypeSection  QuestionType = "Section"  // 段落
	TypeRadio    QuestionType = "Radio"    // 单选
	TypeCheckbox QuestionType = "Checkbox" // 多选
	TypeText     QuestionType = "Text"     // 文本
	TypeTextarea QuestionType = "Textarea" // 文本域
	TypeNumber   QuestionType = "Number"   // 数字
)
