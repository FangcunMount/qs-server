package role

import (
	"time"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
)

// Testee 被测者/考生（用户体系中的角色）
// 值对象：用于表示参与心理测评、能力测试的用户角色
// 包含用户的基本信息，其中 Birthday 是用户的真实生日
type Testee struct {
	UserID   user.UserID // 用户ID
	Name     string      // 姓名
	Sex      uint8       // 性别 (0-未知, 1-男, 2-女)
	Birthday time.Time   // 生日
}

// NewTestee 创建被测者
func NewTestee(userID user.UserID, name string) *Testee {
	return &Testee{
		UserID: userID,
		Name:   name,
	}
}

// GetUserID 获取用户ID
func (t *Testee) GetUserID() user.UserID {
	return t.UserID
}

// GetName 获取姓名
func (t *Testee) GetName() string {
	return t.Name
}

// GetSex 获取性别
func (t *Testee) GetSex() uint8 {
	return t.Sex
}

// GetBirthday 获取生日
func (t *Testee) GetBirthday() time.Time {
	return t.Birthday
}

// GetAge 计算当前年龄（根据生日计算）
func (t *Testee) GetAge() int {
	if t.Birthday.IsZero() {
		return 0
	}
	now := time.Now()
	age := now.Year() - t.Birthday.Year()
	if now.YearDay() < t.Birthday.YearDay() {
		age--
	}
	return age
}

// WithSex 设置性别
func (t *Testee) WithSex(sex uint8) *Testee {
	t.Sex = sex
	return t
}

// WithBirthday 设置生日
func (t *Testee) WithBirthday(birthday time.Time) *Testee {
	t.Birthday = birthday
	return t
}
