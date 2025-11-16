package meta

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

const defaultRegion = "CN"

var allowedRegions = []string{"CN"}

// E.164 快速语法校验
var e164Re = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// Phone 电话号码值对象
type Phone struct {
	e164 string // 一律保存 E.164；不存 raw
}

// NewPhone 解析并创建 Phone 实例
func NewPhone(raw string) (Phone, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Phone{}, errors.New("empty phone")
	}

	n, perr := phonenumbers.Parse(raw, defaultRegion)
	if perr != nil {
		return Phone{}, fmt.Errorf("parse phone failed: %w", perr)
	}
	if !phonenumbers.IsValidNumber(n) {
		return Phone{}, errors.New("invalid phone number by numbering plan")
	}

	t := phonenumbers.GetNumberType(n)
	if t != phonenumbers.MOBILE && t != phonenumbers.FIXED_LINE_OR_MOBILE {
		return Phone{}, fmt.Errorf("unsupported number type: %v", t)
	}

	region := phonenumbers.GetRegionCodeForNumber(n)
	if len(allowedRegions) > 0 && !slices.Contains(allowedRegions, region) {
		return Phone{}, fmt.Errorf("region %s not allowed", region)
	}

	e164 := phonenumbers.Format(n, phonenumbers.E164)
	if !e164Re.MatchString(e164) {
		return Phone{}, errors.New("normalized E.164 not matching syntax")
	}

	return Phone{e164: e164}, nil
}

// ===== API =====

func (p Phone) String() string     { return p.e164 }
func (p Phone) Number() string     { return p.e164 }
func (p Phone) IsEmpty() bool      { return p.e164 == "" }
func (p Phone) Equal(o Phone) bool { return p.e164 == o.e164 }

// （可选）拿地区；惰性解析
func (p Phone) Region() (string, error) {
	if p.IsEmpty() {
		return "", errors.New("empty phone")
	}
	n, err := phonenumbers.Parse(p.e164, "")
	if err != nil {
		return "", err
	}
	return phonenumbers.GetRegionCodeForNumber(n), nil
}

// ===== DB 编解码：守不变式（只接受/写出 E.164）=====

// Value 实现 driver.Valuer 接口，返回数据库存储值
func (p Phone) Value() (driver.Value, error) {
	if p.IsEmpty() {
		return "", nil // 返回空字符串以符合 NOT NULL 约束
	}
	if !e164Re.MatchString(p.e164) {
		return nil, errors.New("Phone.Value: non-E164 in memory")
	}
	return p.e164, nil
}

// Scan 实现 sql.Scanner 接口，从数据库读取值
func (p *Phone) Scan(src any) error {
	if src == nil {
		*p = Phone{}
		return nil
	}
	switch v := src.(type) {
	case string:
		if v == "" {
			*p = Phone{}
			return nil
		}
		if !e164Re.MatchString(v) {
			return errors.New("Phone.Scan: db value is not E.164")
		}
		*p = Phone{e164: v}
		return nil
	case []byte:
		s := string(v)
		if s == "" {
			*p = Phone{}
			return nil
		}
		if !e164Re.MatchString(s) {
			return errors.New("Phone.Scan: db value is not E.164")
		}
		*p = Phone{e164: s}
		return nil
	default:
		return fmt.Errorf("Phone.Scan: unsupported type %T", src)
	}
}

// ===== JSON 编解码（可选，但强烈推荐）=====

// MarshalJSON 实现 json.Marshaler 接口，返回 JSON 编码值
func (p Phone) MarshalJSON() ([]byte, error) {
	if p.IsEmpty() {
		return []byte(`""`), nil
	}
	if !e164Re.MatchString(p.e164) {
		return nil, errors.New("Phone.MarshalJSON: non-E164 in memory")
	}
	// 简单起见不转义
	return []byte(`"` + p.e164 + `"`), nil
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，解析 JSON 编码值
func (p *Phone) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" {
		*p = Phone{}
		return nil
	}
	np, err := NewPhone(s) // 统一走解析与业务校验
	if err != nil {
		return err
	}
	*p = np
	return nil
}
