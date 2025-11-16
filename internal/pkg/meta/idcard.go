package meta

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ===== IDCard：含姓名的组合对象（把号码封装为值对象） =====

// IDCard 身份证值对象
type IDCard struct {
	name string
	id   IDNumber
}

// NewIDCard 创建身份证值对象
func NewIDCard(name, number string) (IDCard, error) {
	id, err := NewIDNumber(number)
	if err != nil {
		return IDCard{}, err
	}
	return IDCard{name: strings.TrimSpace(name), id: id}, nil
}

// ===== IDCard API =====

func (c IDCard) Name() string        { return c.name }
func (c IDCard) Number() string      { return c.id.Number() } // 总是返回规范18位
func (c IDCard) ID() IDNumber        { return c.id }
func (c IDCard) String() string      { return c.id.String() }
func (c IDCard) Equal(o IDCard) bool { return c.name == o.name && c.id.Equal(o.id) }

// ============ DB 编解码：只存储身份证号码 ============

// Value 实现 database/sql/driver.Valuer 接口，用于数据库写入
func (c IDCard) Value() (driver.Value, error) {
	// 只存储身份证号码，不存储姓名（姓名通常在其他字段）
	return c.id.Value()
}

// Scan 实现 database/sql.Scanner 接口，用于数据库读取
func (c *IDCard) Scan(src any) error {
	// 只能扫描身份证号码，姓名需要从其他字段获取
	// 创建零值 IDCard（姓名为空）
	var id IDNumber
	if err := id.Scan(src); err != nil {
		return err
	}
	c.id = id
	c.name = "" // 姓名字段不从数据库此列读取
	return nil
}

// ====== IDNumber：仅代表"已规范化的二代身份证号（18位，末位可为X）" ======

type IDNumber struct {
	v string // 规范化后的 18 位：前17位数字 + 校验位（0-9 或 X）
}

// NewIDNumber 解析/校验/归一化（支持 15 位旧号，自动转成 18 位）
func NewIDNumber(raw string) (IDNumber, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return IDNumber{}, errors.New("身份证号为空")
	}

	switch len(s) {
	case 18:
		norm, err := normalize18(s)
		if err != nil {
			return IDNumber{}, err
		}
		return IDNumber{v: norm}, nil
	case 15:
		norm, err := normalize15to18(s)
		if err != nil {
			return IDNumber{}, err
		}
		return IDNumber{v: norm}, nil
	default:
		return IDNumber{}, errors.New("身份证号长度必须为15或18位")
	}
}

func (id IDNumber) String() string        { return id.v }
func (id IDNumber) Number() string        { return id.v }
func (id IDNumber) IsEmpty() bool         { return id.v == "" }
func (id IDNumber) Equal(o IDNumber) bool { return id.v == o.v }

// BirthDate 解析出生日期
func (id IDNumber) BirthDate() (time.Time, error) {
	if id.IsEmpty() {
		return time.Time{}, errors.New("empty id")
	}
	b := id.v[6:14] // YYYYMMDD
	t, err := time.Parse("20060102", b)
	if err != nil {
		return time.Time{}, errors.New("出生日期不合法")
	}
	return t, nil
}

// IsMale 性别：奇数为男，偶数为女
func (id IDNumber) IsMale() (bool, error) {
	if id.IsEmpty() {
		return false, errors.New("empty id")
	}
	n := id.v[16] // 第17位（从0计数）是顺序码最后一位
	return (n-'0')%2 == 1, nil
}

// ProvinceCode 省级代码（前两位）
func (id IDNumber) ProvinceCode() string {
	if id.IsEmpty() {
		return ""
	}
	return id.v[:2]
}

// ===== DB 编解码：只接受/写出规范化的18位 =====

func (id IDNumber) Value() (driver.Value, error) {
	if id.IsEmpty() {
		return nil, nil // 返回 NULL 值，支持数据库可空字段
	}
	// 防御检查：必须是规范化的 18 位
	if _, err := normalize18(id.v); err != nil {
		return nil, fmt.Errorf("IDNumber.Value: 非规范18位: %w", err)
	}
	return id.v, nil
}

func (id *IDNumber) Scan(src any) error {
	if src == nil {
		*id = IDNumber{}
		return nil
	}
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("IDNumber.Scan: 不支持类型 %T", src)
	}
	if s == "" {
		*id = IDNumber{}
		return nil
	}
	norm, err := normalize18(strings.ToUpper(strings.TrimSpace(s)))
	if err != nil {
		return fmt.Errorf("IDNumber.Scan: DB 中不是规范18位: %w", err)
	}
	id.v = norm
	return nil
}

// ===== JSON：入口统一规范化 =====

func (id IDNumber) MarshalJSON() ([]byte, error) {
	if id.IsEmpty() {
		return []byte(`""`), nil
	}
	// 确保内存中是规范18位
	if _, err := normalize18(id.v); err != nil {
		return nil, err
	}
	return []byte(`"` + id.v + `"`), nil
}

func (id *IDNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" {
		*id = IDNumber{}
		return nil
	}
	ni, err := NewIDNumber(s)
	if err != nil {
		return err
	}
	*id = ni
	return nil
}

// ===== 规范化/校验细节 =====

// 仅校验省级代码前两位是否在合法集合；不校验 6 位地区码（变更频繁）
var validProv = map[string]struct{}{
	"11": {}, "12": {}, "13": {}, "14": {}, "15": {},
	"21": {}, "22": {}, "23": {},
	"31": {}, "32": {}, "33": {}, "34": {}, "35": {}, "36": {}, "37": {},
	"41": {}, "42": {}, "43": {}, "44": {}, "45": {}, "46": {},
	"50": {}, "51": {}, "52": {}, "53": {}, "54": {},
	"61": {}, "62": {}, "63": {}, "64": {}, "65": {},
	"71": {}, "81": {}, "82": {}, // 台湾/香港/澳门 省级代码
}

func normalize18(s string) (string, error) {
	if len(s) != 18 {
		return "", errors.New("长度不是18位")
	}
	// 前17位必须是数字
	for i := 0; i < 17; i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", errors.New("前17位必须为数字")
		}
	}
	// 省级代码
	if _, ok := validProv[s[:2]]; !ok {
		return "", errors.New("省级代码不合法")
	}
	// 出生日期
	if err := checkBirthYYYYMMDD(s[6:14]); err != nil {
		return "", err
	}
	// 顺序码不能为 "000"
	if s[14:17] == "000" {
		return "", errors.New("顺序码不能为000")
	}
	// 校验位
	want := computeCheckDigit(s[:17])
	last := s[17]
	if last == 'x' {
		last = 'X'
	}
	if last != want {
		return "", errors.New("校验位不匹配")
	}
	// 统一大写
	return s[:17] + string(want), nil
}

func normalize15to18(s string) (string, error) {
	if len(s) != 15 {
		return "", errors.New("长度不是15位")
	}
	for i := 0; i < 15; i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", errors.New("15位身份证应全为数字")
		}
	}
	if _, ok := validProv[s[:2]]; !ok {
		return "", errors.New("省级代码不合法")
	}
	// 15位出生日期 YYMMDD -> 推断为 19YYMMDD（历史卡普遍如此）
	yy := s[6:8]
	mm := s[8:10]
	dd := s[10:12]
	birth := "19" + yy + mm + dd
	if err := checkBirthYYYYMMDD(birth); err != nil {
		return "", err
	}
	base17 := s[:6] + birth + s[12:15]
	cd := computeCheckDigit(base17)
	return base17 + string(cd), nil
}

func checkBirthYYYYMMDD(b string) error {
	if len(b) != 8 {
		return errors.New("出生日期格式错误")
	}
	t, err := time.Parse("20060102", b)
	if err != nil {
		return errors.New("出生日期不合法")
	}
	// 合理区间：1900-01-01 ~ 今天（可按需调整上/下限）
	if t.Before(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return errors.New("出生日期过早")
	}
	today := time.Now()
	if t.After(today) {
		return errors.New("出生日期晚于当前日期")
	}
	return nil
}

var weights = [...]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2} // 对应前17位
var mods = [...]byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'} // sum%11 -> 校验位

func computeCheckDigit(d17 string) byte {
	var sum int
	for i := 0; i < 17; i++ {
		sum += int(d17[i]-'0') * weights[i]
	}
	return mods[sum%11]
}
