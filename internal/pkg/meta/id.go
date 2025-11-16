package meta

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// 0 ID 常量
const ZeroID = ID(0)

// 0 是否当成 NULL 处理
const ZeroAsNull = true

// ID：用 int64 映射数据库 BIGINT，天然规避 > MaxInt64 的问题
type ID int64

// ===== 内部无错构造（业务内调用，不处理外部垃圾输入） =====

// New 内部生成（放你的雪花/序列器），保证 <= MaxInt64
func New() ID {
	// TODO: 替换为你的发号器
	// v := yourGenerator() // must be <= math.MaxInt64
	// return ID(v)
	panic("meta.New() not wired: plug in your generator")
}

// FromUint64 内部可信数据来源（若越界就 panic —— 逻辑错误尽早暴露）
func FromUint64(v uint64) ID {
	if v > math.MaxInt64 {
		panic(fmt.Errorf("meta.FromUint64: %d exceeds int64", v))
	}
	return ID(v)
}

// MustFromUint64 显式 Must 版本（更清晰）
func MustFromUint64(v uint64) ID { return FromUint64(v) }

// ===== 边界有错构造（只在解析 URL/JSON/表单时用） =====

// FromInt64 从 int64 解析，负数报错
func FromInt64(v int64) (ID, error) {
	if v < 0 {
		return 0, errors.New("meta.FromInt64: negative")
	}
	return ID(v), nil
}

// ParseID 解析字符串形式的 ID
func ParseID(s string) (ID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		if ZeroAsNull {
			return 0, nil
		}
		return 0, errors.New("empty id")
	}
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse id: %w", err)
	}
	if u > math.MaxInt64 {
		return 0, fmt.Errorf("id %d exceeds int64", u)
	}
	return ID(u), nil
}

// ===== 基础方法 =====

func (id ID) Int64() int64 { return int64(id) }
func (id ID) Uint64() uint64 {
	// 非负保证：我们的构造路径不允许负数
	return uint64(id)
}
func (id ID) String() string { return strconv.FormatInt(int64(id), 10) }
func (id ID) IsZero() bool   { return id == 0 }

// ===== DB 编解码 =====

// Value 实现 driver.Valuer 接口
func (id ID) Value() (driver.Value, error) {
	if id.IsZero() && ZeroAsNull {
		return nil, nil
	}
	// 直接写 int64
	return int64(id), nil
}

// Scan 实现 sql.Scanner 接口
func (id *ID) Scan(src any) error {
	if src == nil {
		*id = 0
		return nil
	}
	switch v := src.(type) {
	case int64:
		if v < 0 {
			return fmt.Errorf("meta.ID.Scan: negative %d", v)
		}
		*id = ID(v)
		return nil
	case uint64:
		if v > math.MaxInt64 {
			return fmt.Errorf("meta.ID.Scan: %d exceeds int64", v)
		}
		*id = ID(v)
		return nil
	case []byte:
		parsed, err := ParseID(string(v))
		if err != nil {
			return err
		}
		*id = parsed
		return nil
	case string:
		parsed, err := ParseID(v)
		if err != nil {
			return err
		}
		*id = parsed
		return nil
	default:
		return fmt.Errorf("meta.ID.Scan: unsupported type %T", src)
	}
}

// ===== JSON：统一输出“字符串”，兼容输入数字/字符串 =====

// MarshalJSON 实现 json.Marshaler 接口
func (id ID) MarshalJSON() ([]byte, error) {
	if id.IsZero() && ZeroAsNull {
		return []byte(`null`), nil
	}
	// 字符串，避免 JS 精度丢失
	return json.Marshal(id.String())
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (id *ID) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*id = 0
		return nil
	}
	// 先当字符串
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := ParseID(s)
		if err != nil {
			return err
		}
		*id = parsed
		return nil
	}
	// 再兼容数字
	var num json.Number
	if err := json.Unmarshal(b, &num); err != nil {
		return errors.New("meta.ID.UnmarshalJSON: not string/number")
	}
	u, err := strconv.ParseUint(num.String(), 10, 64)
	if err != nil {
		return err
	}
	if u > math.MaxInt64 {
		return fmt.Errorf("id %d exceeds int64", u)
	}
	*id = ID(u)
	return nil
}
