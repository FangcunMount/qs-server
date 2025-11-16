package meta

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Code 业务编码类型，用于问卷编码、问题编码等业务场景
type Code string

// NewCode 创建业务编码
func NewCode(value string) Code {
	return Code(value)
}

// GenerateCode 生成新的业务编码（使用 UUID）
func GenerateCode() (Code, error) {
	id := uuid.New()
	// 使用短格式（去掉连字符）
	code := strings.ReplaceAll(id.String(), "-", "")
	return Code(code), nil
}

// GenerateNewCode 生成新的业务编码（别名，保持兼容）
func GenerateNewCode() (Code, error) {
	return GenerateCode()
}

// GenerateCodeWithPrefix 生成带前缀的业务编码
func GenerateCodeWithPrefix(prefix string) (Code, error) {
	id := uuid.New()
	// 使用时间戳+UUID的短格式
	timestamp := time.Now().Format("20060102")
	shortID := strings.ReplaceAll(id.String(), "-", "")[:8]
	code := fmt.Sprintf("%s%s%s", prefix, timestamp, shortID)
	return Code(code), nil
}

// ===== 基础方法 =====

func (c Code) String() string { return string(c) }
func (c Code) Value() string  { return string(c) }
func (c Code) IsEmpty() bool  { return c == "" }

// Equals 比较两个 Code 是否相等
func (c Code) Equals(other Code) bool {
	return c == other
}

// ===== DB 编解码 =====

// DBValue 实现 driver.Valuer 接口
func (c Code) DBValue() (driver.Value, error) {
	if c.IsEmpty() {
		return nil, nil
	}
	return string(c), nil
}

// Scan 实现 sql.Scanner 接口
func (c *Code) Scan(src any) error {
	if src == nil {
		*c = ""
		return nil
	}
	switch v := src.(type) {
	case string:
		*c = Code(v)
		return nil
	case []byte:
		*c = Code(v)
		return nil
	default:
		return fmt.Errorf("meta.Code.Scan: unsupported type %T", src)
	}
}

// ===== JSON 编解码 =====

// MarshalJSON 实现 json.Marshaler 接口
func (c Code) MarshalJSON() ([]byte, error) {
	if c.IsEmpty() {
		return []byte(`""`), nil
	}
	return json.Marshal(string(c))
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (c *Code) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || string(b) == `""` {
		*c = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("meta.Code.UnmarshalJSON: not a string")
	}
	*c = Code(s)
	return nil
}

// ===== 验证方法 =====

// Validate 验证编码格式
func (c Code) Validate() error {
	if c.IsEmpty() {
		return errors.New("code cannot be empty")
	}
	if len(c) > 100 {
		return errors.New("code too long (max 100 characters)")
	}
	return nil
}
