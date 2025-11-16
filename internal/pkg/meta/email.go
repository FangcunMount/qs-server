package meta

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

const (
	// 是否把本地部分（@ 前）折叠为小写。
	// 绝大多数邮箱提供商不区分大小写；若你要 RFC 严格区分，改为 false。
	foldLocalLower = true
)

// dot-atom：更贴近网站输入习惯（不接受引号式本地名）
var dotAtom = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[A-Za-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*$`)

// Email 邮箱值对象
type Email struct {
	addr string // 规范化后的 local@ascii-domain
}

// NewEmail 解析并创建 Email 实例
func NewEmail(raw string) (Email, error) {
	norm, err := normalizeEmail(raw, true /*strictDotAtom*/)
	if err != nil {
		return Email{}, err
	}
	return Email{addr: norm}, nil
}

// ==== API ====

func (e Email) String() string  { return e.addr }
func (e Email) Address() string { return e.addr }
func (e Email) IsEmpty() bool   { return e.addr == "" }

// 两边都已规范化，直接字符串相等即可
func (e Email) Equal(o Email) bool { return e.addr == o.addr }

// 取本地部分与域（域为 ASCII 小写）
func (e Email) LocalPart() (string, error) {
	if e.IsEmpty() {
		return "", errors.New("empty email")
	}
	i := strings.LastIndexByte(e.addr, '@')
	if i < 0 {
		return "", errors.New("corrupted email")
	}
	return e.addr[:i], nil
}
func (e Email) DomainASCII() (string, error) {
	if e.IsEmpty() {
		return "", errors.New("empty email")
	}
	i := strings.LastIndexByte(e.addr, '@')
	if i < 0 || i == len(e.addr)-1 {
		return "", errors.New("corrupted email")
	}
	return e.addr[i+1:], nil
}

// ==== DB 编解码：强制守不变式（DB 必须已是规范形态） ====

// Value 实现 driver.Valuer 接口，返回数据库存储值
func (e Email) Value() (driver.Value, error) {
	if e.IsEmpty() {
		return "", nil // 返回空字符串以符合 NOT NULL 约束
	}
	// 防御：内存中的必须已是规范形态
	norm, err := normalizeEmail(e.addr, true)
	if err != nil {
		return nil, err
	}
	if norm != e.addr {
		return nil, errors.New("Email.Value: non-normalized value in memory")
	}
	return e.addr, nil
}

// Scan 实现 sql.Scanner 接口，从数据库读取值
func (e *Email) Scan(src any) error {
	if src == nil {
		*e = Email{}
		return nil
	}
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("Email.Scan: unsupported type %T", src)
	}
	if s == "" {
		*e = Email{}
		return nil
	}
	norm, err := normalizeEmail(s, true)
	if err != nil {
		return err
	}
	// DB 中必须已是规范形态；否则直接报错，及早暴露脏数据
	if norm != s {
		return errors.New("Email.Scan: db value is not normalized local@ascii-domain")
	}
	*e = Email{addr: norm}
	return nil
}

// ==== JSON：入口统一走规范化 ====

// MarshalJSON 实现 json.Marshaler 接口，返回 JSON 编码值
func (e Email) MarshalJSON() ([]byte, error) {
	if e.IsEmpty() {
		return []byte(`""`), nil
	}
	// 简单安全：确保是规范形态
	norm, err := normalizeEmail(e.addr, true)
	if err != nil {
		return nil, err
	}
	if norm != e.addr {
		return nil, errors.New("Email.MarshalJSON: non-normalized value")
	}
	return []byte(`"` + e.addr + `"`), nil
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，解析 JSON 编码值
func (e *Email) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" {
		*e = Email{}
		return nil
	}
	ne, err := NewEmail(s)
	if err != nil {
		return err
	}
	*e = ne
	return nil
}

// ==== 规范化与校验核心逻辑 ====

// normalizeEmail 规范化邮箱地址，返回 local@ascii-domain 形式
// strictDotAtom：是否强制 local 部分为 dot-atom 形式（不接受引号式）
// 返回错误表示语法或语义无效
func normalizeEmail(raw string, strictDotAtom bool) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, "\r\n") {
		return "", errors.New("empty email")
	}
	// 语法解析：只接受裸地址（拒绝 "Bob <a@b.com>"）
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return "", errors.New("invalid email syntax")
	}
	if addr.Name != "" {
		return "", errors.New("display name not allowed; use local@domain only")
	}
	at := strings.LastIndexByte(addr.Address, '@')
	if at <= 0 || at == len(addr.Address)-1 {
		return "", errors.New("missing or bad @")
	}
	local, domain := addr.Address[:at], addr.Address[at+1:]

	if strictDotAtom && !dotAtom.MatchString(local) {
		return "", errors.New("unsupported local-part (quoted strings not allowed)")
	}
	if foldLocalLower {
		local = strings.ToLower(local)
	}

	// 域名：IDN → ASCII，小写；检查 label 规则与至少含 TLD
	asciiDomain, err := idna.Lookup.ToASCII(strings.ToLower(domain))
	if err != nil || asciiDomain == "" {
		return "", errors.New("invalid domain (IDNA)")
	}
	labels := strings.Split(asciiDomain, ".")
	if len(labels) < 2 {
		return "", errors.New("domain must contain a TLD")
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return "", errors.New("invalid domain label")
		}
	}

	norm := local + "@" + asciiDomain
	// RFC 5321 字节长度限制
	if len(local) > 64 || len(norm) > 254 {
		return "", errors.New("email too long")
	}
	return norm, nil
}
