package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// testeeRepository 受试者仓储实现
type testeeRepository struct {
	mysql.BaseRepository[*TesteePO]
	mapper *TesteeMapper
}

// NewTesteeRepository 创建受试者仓储
func NewTesteeRepository(db *gorm.DB) testee.Repository {
	repo := &testeeRepository{
		BaseRepository: mysql.NewBaseRepository[*TesteePO](db),
		mapper:         NewTesteeMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateError)
	return repo
}

// Save 保存受试者
func (r *testeeRepository) Save(ctx context.Context, t *testee.Testee) error {
	po := r.mapper.ToPO(t)

	// 确保 BeforeCreate 被调用以生成 ID
	if err := po.BeforeCreate(nil); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(po *TesteePO) {
		r.mapper.SyncID(po, t)
		t.SetCreatedAt(po.CreatedAt)
		t.SetUpdatedAt(po.UpdatedAt)
	})
}

// Update 更新受试者
func (r *testeeRepository) Update(ctx context.Context, t *testee.Testee) error {
	po := r.mapper.ToPO(t)

	return r.UpdateAndSync(ctx, po, func(po *TesteePO) {
		r.mapper.SyncID(po, t)
		t.SetCreatedAt(po.CreatedAt)
		t.SetUpdatedAt(po.UpdatedAt)
	})
}

// FindByID 根据ID查找受试者
func (r *testeeRepository) FindByID(ctx context.Context, id testee.ID) (*testee.Testee, error) {
	po, err := r.BaseRepository.FindByID(ctx, uint64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

func (r *testeeRepository) FindByIDs(ctx context.Context, ids []testee.ID) ([]*testee.Testee, error) {
	if len(ids) == 0 {
		return []*testee.Testee{}, nil
	}

	rawIDs := make([]uint64, 0, len(ids))
	for _, id := range ids {
		rawIDs = append(rawIDs, uint64(id))
	}

	var pos []*TesteePO
	err := r.WithContext(ctx).
		Where("id IN ? AND deleted_at IS NULL", rawIDs).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// FindByProfile 根据用户档案ID查找受试者
// 注意：当前 ProfileID 对应 IAM.Child.ID
func (r *testeeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*testee.Testee, error) {
	var po TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND profile_id = ? AND deleted_at IS NULL", orgID, profileID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByOrgAndName 根据机构和姓名查找受试者列表（用于模糊匹配）
func (r *testeeRepository) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*testee.Testee, error) {
	var pos []*TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND name LIKE ? AND deleted_at IS NULL", orgID, "%"+name+"%").
		Order("created_at DESC").
		Order("id DESC").
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListByOrg 列出机构下的受试者
func (r *testeeRepository) ListByOrg(
	ctx context.Context,
	orgID int64,
	filter testee.ListFilter,
	offset, limit int,
) ([]*testee.Testee, error) {
	var pos []*TesteePO
	err := r.filteredByOrg(ctx, orgID, filter).
		Order("created_at DESC").
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListByOrgAndIDs 在机构范围内按受试者 ID 集合查询。
func (r *testeeRepository) ListByOrgAndIDs(
	ctx context.Context,
	orgID int64,
	ids []testee.ID,
	filter testee.ListFilter,
	offset, limit int,
) ([]*testee.Testee, error) {
	if len(ids) == 0 {
		return []*testee.Testee{}, nil
	}

	var pos []*TesteePO
	query := r.filteredByOrgAndIDs(ctx, orgID, ids, filter)

	err := query.
		Order("created_at DESC").
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListByTags 根据标签查找受试者
func (r *testeeRepository) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*testee.Testee, error) {
	return r.ListByOrg(ctx, orgID, testee.ListFilter{Tags: tags}, offset, limit)
}

// ListKeyFocus 列出重点关注的受试者
func (r *testeeRepository) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*testee.Testee, error) {
	keyFocus := true
	return r.ListByOrg(ctx, orgID, testee.ListFilter{KeyFocus: &keyFocus}, offset, limit)
}

// ListByProfileIDs 根据多个用户档案ID查找受试者列表
func (r *testeeRepository) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]*testee.Testee, error) {
	if len(profileIDs) == 0 {
		return []*testee.Testee{}, nil
	}

	var pos []*TesteePO
	err := r.WithContext(ctx).
		Where("profile_id IN ? AND deleted_at IS NULL", profileIDs).
		Order("created_at DESC").
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// Delete 删除受试者（软删除）
func (r *testeeRepository) Delete(ctx context.Context, id testee.ID) error {
	return r.DeleteByID(ctx, uint64(id))
}

// Count 统计机构下的受试者数量
func (r *testeeRepository) Count(ctx context.Context, orgID int64, filter testee.ListFilter) (int64, error) {
	var count int64
	err := r.filteredByOrg(ctx, orgID, filter).
		Model(&TesteePO{}).
		Count(&count).Error

	return count, err
}

// CountByOrgAndIDs 在机构范围内按受试者 ID 集合统计数量。
func (r *testeeRepository) CountByOrgAndIDs(ctx context.Context, orgID int64, ids []testee.ID, filter testee.ListFilter) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	var count int64
	err := r.filteredByOrgAndIDs(ctx, orgID, ids, filter).
		Model(&TesteePO{}).
		Count(&count).Error
	return count, err
}

func (r *testeeRepository) filteredByOrg(
	ctx context.Context,
	orgID int64,
	filter testee.ListFilter,
) *gorm.DB {
	query := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID)

	return r.applyFilter(query, filter)
}

func (r *testeeRepository) filteredByOrgAndIDs(
	ctx context.Context,
	orgID int64,
	ids []testee.ID,
	filter testee.ListFilter,
) *gorm.DB {
	rawIDs := make([]uint64, 0, len(ids))
	for _, id := range ids {
		rawIDs = append(rawIDs, uint64(id))
	}

	query := r.WithContext(ctx).
		Where("org_id = ? AND id IN ? AND deleted_at IS NULL", orgID, rawIDs)

	return r.applyFilter(query, filter)
}

func (r *testeeRepository) applyFilter(query *gorm.DB, filter testee.ListFilter) *gorm.DB {
	if filter.Name != "" {
		query = query.Where("name LIKE ?", "%"+filter.Name+"%")
	}
	if filter.KeyFocus != nil {
		query = query.Where("is_key_focus = ?", *filter.KeyFocus)
	}
	for _, tag := range filter.Tags {
		query = query.Where("JSON_CONTAINS(tags, ?)", `"`+tag+`"`)
	}
	if filter.CreatedAtStart != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAtStart)
	}
	if filter.CreatedAtEnd != nil {
		query = query.Where("created_at < ?", *filter.CreatedAtEnd)
	}

	return query
}

// translateError 将数据库错误转换为领域错误
func translateError(err error) error {
	if err == nil {
		return nil
	}

	// 处理唯一约束冲突
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrUserAlreadyExists, "testee or staff already exists")
	}

	// 处理记录不存在
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrUserNotFound, "record not found")
	}

	return err
}
