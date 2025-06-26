package dto

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// UserDTO 用户数据传输对象
type UserDTO struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FromDomain 从领域对象创建DTO
func (dto *UserDTO) FromDomain(u *user.User) {
	dto.ID = u.ID().Value()
	dto.Username = u.Username()
	dto.Email = u.Email()
	dto.Status = int(u.Status())
	dto.CreatedAt = u.CreatedAt()
	dto.UpdatedAt = u.UpdatedAt()
}

// UserListDTO 用户列表DTO
type UserListDTO struct {
	Items      []*UserDTO          `json:"items"`
	Pagination *PaginationResponse `json:"pagination"`
}

// PaginationResponse 分页响应（临时定义，后续可能移到shared）
type PaginationResponse struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalCount int64 `json:"total_count"`
	HasMore    bool  `json:"has_more"`
}

// UserFilterDTO 用户过滤条件DTO
type UserFilterDTO struct {
	Status    *user.Status `json:"status,omitempty"`
	Keyword   *string      `json:"keyword,omitempty"`
	SortBy    string       `json:"sort_by,omitempty"`
	SortOrder string       `json:"sort_order,omitempty"`
}

// SetDefaults 设置默认值
func (f *UserFilterDTO) SetDefaults() {
	if f.SortBy == "" {
		f.SortBy = "created_at"
	}
	if f.SortOrder == "" {
		f.SortOrder = "desc"
	}
}

// HasStatusFilter 检查是否有状态过滤
func (f *UserFilterDTO) HasStatusFilter() bool {
	return f.Status != nil
}

// HasKeyword 检查是否有关键字搜索
func (f *UserFilterDTO) HasKeyword() bool {
	return f.Keyword != nil && *f.Keyword != ""
}

// GetStatus 获取状态过滤值
func (f *UserFilterDTO) GetStatus() user.Status {
	if f.Status != nil {
		return *f.Status
	}
	return user.StatusActive // 默认值
}

// GetKeyword 获取关键字
func (f *UserFilterDTO) GetKeyword() string {
	if f.Keyword != nil {
		return *f.Keyword
	}
	return ""
}

// FromDomainList 从领域对象列表创建DTO列表
func FromDomainList(users []*user.User) []*UserDTO {
	dtos := make([]*UserDTO, len(users))
	for i, u := range users {
		dto := &UserDTO{}
		dto.FromDomain(u)
		dtos[i] = dto
	}
	return dtos
}
