// Package store owns company service stores within the Actor module.
package store

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Store 公司内的服务门店聚合根；编号和公司归属在创建后不可变。
type Store struct {
	id        uint64
	orgID     int64
	code      string
	name      string
	address   string
	isActive  bool
	version   uint32
	createdAt time.Time
	updatedAt time.Time
	createdBy int64
	updatedBy int64
}

// State 用于恢复聚合状态，不包含数据库或传输协议信息。
type State struct {
	ID        uint64
	OrgID     int64
	Code      string
	Name      string
	Address   string
	IsActive  bool
	Version   uint32
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy int64
	UpdatedBy int64
}

func Restore(v State) *Store {
	return &Store{id: v.ID, orgID: v.OrgID, code: v.Code, name: v.Name, address: v.Address, isActive: v.IsActive, version: v.Version, createdAt: v.CreatedAt, updatedAt: v.UpdatedAt, createdBy: v.CreatedBy, updatedBy: v.UpdatedBy}
}
func (s *Store) State() State {
	return State{ID: s.id, OrgID: s.orgID, Code: s.code, Name: s.name, Address: s.address, IsActive: s.isActive, Version: s.version, CreatedAt: s.createdAt, UpdatedAt: s.updatedAt, CreatedBy: s.createdBy, UpdatedBy: s.updatedBy}
}
func (s *Store) ID() uint64           { return s.id }
func (s *Store) OrgID() int64         { return s.orgID }
func (s *Store) Code() string         { return s.code }
func (s *Store) Name() string         { return s.name }
func (s *Store) Address() string      { return s.address }
func (s *Store) IsActive() bool       { return s.isActive }
func (s *Store) Version() uint32      { return s.version }
func (s *Store) CreatedAt() time.Time { return s.createdAt }
func (s *Store) UpdatedAt() time.Time { return s.updatedAt }
func (s *Store) CreatedBy() int64     { return s.createdBy }
func (s *Store) UpdatedBy() int64     { return s.updatedBy }
func New(id uint64, orgID int64, c, name, address string, actor int64, now time.Time) (*Store, error) {
	c, name, address, err := Normalize(c, name, address)
	if err != nil {
		return nil, err
	}
	if id == 0 || orgID <= 0 || actor <= 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "store identity required")
	}
	return Restore(State{ID: id, OrgID: orgID, Code: c, Name: name, Address: address, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor}), nil
}
func (s *Store) CheckVersion(expected uint32) error {
	if expected == 0 || expected != s.version {
		return errors.WithCode(code.ErrConflict, "configuration changed; refresh and retry")
	}
	return nil
}
func (s *Store) UpdateProfile(name, address string, expected uint32, actor int64, now time.Time) error {
	if err := s.CheckVersion(expected); err != nil {
		return err
	}
	_, name, address, err := Normalize(s.code, name, address)
	if err != nil {
		return err
	}
	s.name = name
	s.address = address
	s.touch(actor, now)
	return nil
}
func (s *Store) SetActive(active bool, currentClinicians int64, expected uint32, actor int64, now time.Time) error {
	if err := s.CheckVersion(expected); err != nil {
		return err
	}
	if !active && currentClinicians > 0 {
		return errors.WithCode(code.ErrConflict, "store still has assigned clinicians; reassign them before deactivation")
	}
	s.isActive = active
	s.touch(actor, now)
	return nil
}
func (s *Store) touch(actor int64, now time.Time) {
	s.version++
	s.updatedBy = actor
	s.updatedAt = now
}
func Normalize(c, name, address string) (string, string, string, error) {
	c = strings.ToUpper(strings.TrimSpace(c))
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	if c == "" || len(c) > 32 || name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(address) > 255 {
		return "", "", "", errors.WithCode(code.ErrInvalidArgument, "invalid store code, name or address")
	}
	for _, r := range c {
		valid := r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !valid {
			return "", "", "", errors.WithCode(code.ErrInvalidArgument, "store code accepts letters, digits, hyphen and underscore")
		}
	}
	return c, name, address, nil
}
