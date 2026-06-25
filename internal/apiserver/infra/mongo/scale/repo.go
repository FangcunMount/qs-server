package scale

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// Repository Scale MongoDB 存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *ScaleMapper
}

// NewRepository 创建 Scale MongoDB 存储库
func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *Repository {
	po := &ScalePO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:         NewScaleMapper(),
	}
}

// Create 创建量表
func (r *Repository) Create(ctx context.Context, domain *scale.MedicalScale) error {
	domain.SetRecordRole(scale.RecordRoleHead)
	domain.SetActivePublished(false)

	po := r.mapper.ToPO(domain)
	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()
	po.RecordRole = scale.RecordRoleHead.String()
	po.IsActivePublished = false

	insertData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	_, err = r.InsertOne(ctx, insertData)
	return err
}

// CreatePublishedSnapshot 创建或更新已发布量表快照。
func (r *Repository) CreatePublishedSnapshot(ctx context.Context, domain *scale.MedicalScale, active bool) error {
	po := r.mapper.ToPO(domain)
	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()
	po.RecordRole = scale.RecordRolePublishedSnapshot.String()
	po.IsActivePublished = active

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	filter := bson.M{
		"code":          domain.GetCode().String(),
		"scale_version": domain.GetScaleVersion(),
		"record_role":   scale.RecordRolePublishedSnapshot.String(),
		"deleted_at":    nil,
	}
	_, err = r.Collection().UpdateOne(ctx, filter, bson.M{"$set": updateData}, options.Update().SetUpsert(true))
	return err
}

// FindByCode 根据编码查询量表
func (r *Repository) FindByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	return r.findOne(ctx, headFilter(code))
}

// FindByCodeVersion 根据编码和量表版本查询量表。
func (r *Repository) FindByCodeVersion(ctx context.Context, code, scaleVersion string) (*scale.MedicalScale, error) {
	if scaleVersion == "" {
		return r.FindByCode(ctx, code)
	}
	domain, err := r.findOne(ctx, publishedVersionFilter(code, scaleVersion))
	if err == nil {
		return domain, nil
	}
	if err != scale.ErrNotFound {
		return nil, err
	}
	return r.findOne(ctx, headVersionFilter(code, scaleVersion))
}

// FindPublishedByCode 根据编码查询当前激活的已发布快照。
func (r *Repository) FindPublishedByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	return r.findOne(ctx, publishedCodeFilter(code))
}

// FindByQuestionnaireCode 根据问卷编码查询量表
func (r *Repository) FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	return r.findOne(ctx, headQuestionnaireFilter(questionnaireCode))
}

// FindPublishedByQuestionnaireCode 根据问卷编码查询当前激活的已发布量表快照。
func (r *Repository) FindPublishedByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	return r.findOne(ctx, publishedQuestionnaireCodeFilter(questionnaireCode))
}

// ListActivePublishedSnapshots 列出所有当前激活的已发布量表快照（backfill 用）。
func (r *Repository) ListActivePublishedSnapshots(ctx context.Context) ([]*scale.MedicalScale, error) {
	cursor, err := r.Collection().Find(ctx, activePublishedSnapshotsFilter())
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var results []*scale.MedicalScale
	for cursor.Next(ctx) {
		var po ScalePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		results = append(results, r.mapper.ToDomain(ctx, &po))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// FindByQuestionnaireRef 根据问卷编码和版本查询量表。
func (r *Repository) FindByQuestionnaireRef(ctx context.Context, questionnaireCode, questionnaireVersion string) (*scale.MedicalScale, error) {
	if questionnaireVersion == "" {
		return r.FindPublishedByQuestionnaireCode(ctx, questionnaireCode)
	}
	return r.findOne(ctx, publishedQuestionnaireRefFilter(questionnaireCode, questionnaireVersion))
}

func (r *Repository) findOne(ctx context.Context, filter bson.M) (*scale.MedicalScale, error) {
	var po ScalePO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, scale.ErrNotFound
		}
		return nil, err
	}

	return r.mapper.ToDomain(ctx, &po), nil
}

func scaleVersionCompatibilityFilter(scaleVersion string) bson.A {
	return bson.A{
		bson.M{"scale_version": scaleVersion},
		bson.M{
			"questionnaire_version": scaleVersion,
			"$or": bson.A{
				bson.M{"scale_version": ""},
				bson.M{"scale_version": nil},
				bson.M{"scale_version": bson.M{"$exists": false}},
			},
		},
	}
}

func headRoleCandidates() bson.A {
	return bson.A{
		bson.M{"record_role": scale.RecordRoleHead.String()},
		bson.M{"record_role": bson.M{"$exists": false}},
		bson.M{"record_role": ""},
	}
}

func headFilter(code string) bson.M {
	return bson.M{
		"code":       code,
		"deleted_at": nil,
		"$or":        headRoleCandidates(),
	}
}

func headVersionFilter(code, scaleVersion string) bson.M {
	filter := headFilter(code)
	filter["$or"] = bson.A{
		bson.M{
			"record_role": scale.RecordRoleHead.String(),
			"$or":         scaleVersionCompatibilityFilter(scaleVersion),
		},
		bson.M{
			"record_role": bson.M{"$exists": false},
			"$or":         scaleVersionCompatibilityFilter(scaleVersion),
		},
		bson.M{
			"record_role": "",
			"$or":         scaleVersionCompatibilityFilter(scaleVersion),
		},
	}
	return filter
}

func publishedVersionFilter(code, scaleVersion string) bson.M {
	return bson.M{
		"code":        code,
		"record_role": scale.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
		"$or":         scaleVersionCompatibilityFilter(scaleVersion),
	}
}

func publishedCodeFilter(code string) bson.M {
	return bson.M{
		"code":                code,
		"record_role":         scale.RecordRolePublishedSnapshot.String(),
		"is_active_published": true,
		"status":              scale.StatusPublished.String(),
		"deleted_at":          nil,
	}
}

func activePublishedSnapshotsFilter() bson.M {
	return bson.M{
		"record_role":         scale.RecordRolePublishedSnapshot.String(),
		"is_active_published": true,
		"status":              scale.StatusPublished.String(),
		"deleted_at":          nil,
	}
}

func headQuestionnaireFilter(questionnaireCode string) bson.M {
	return bson.M{
		"questionnaire_code": questionnaireCode,
		"deleted_at":         nil,
		"$or":                headRoleCandidates(),
	}
}

func publishedQuestionnaireCodeFilter(questionnaireCode string) bson.M {
	return bson.M{
		"questionnaire_code":  questionnaireCode,
		"record_role":         scale.RecordRolePublishedSnapshot.String(),
		"is_active_published": true,
		"status":              scale.StatusPublished.String(),
		"deleted_at":          nil,
	}
}

func publishedQuestionnaireRefFilter(questionnaireCode, questionnaireVersion string) bson.M {
	return bson.M{
		"questionnaire_code":    questionnaireCode,
		"questionnaire_version": questionnaireVersion,
		"record_role":           scale.RecordRolePublishedSnapshot.String(),
		"status":                scale.StatusPublished.String(),
		"deleted_at":            nil,
	}
}

// Update 更新量表
func (r *Repository) Update(ctx context.Context, domain *scale.MedicalScale) error {
	domain.SetRecordRole(scale.RecordRoleHead)
	domain.SetActivePublished(false)

	po := r.mapper.ToPO(domain)
	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()
	po.RecordRole = scale.RecordRoleHead.String()
	po.IsActivePublished = false

	filter := headFilter(domain.GetCode().String())

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	update := bson.M{"$set": updateData}

	_, err = r.Collection().UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// SetActivePublishedVersion 切换当前对外生效的已发布量表快照。
func (r *Repository) SetActivePublishedVersion(ctx context.Context, code, scaleVersion string) error {
	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)

	_, err := r.Collection().UpdateMany(ctx, bson.M{
		"code":        code,
		"record_role": scale.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}, bson.M{"$set": bson.M{
		"is_active_published": false,
		"updated_at":          now,
		"updated_by":          userID,
	}})
	if err != nil {
		return err
	}

	result, err := r.Collection().UpdateOne(ctx, bson.M{
		"code":          code,
		"scale_version": scaleVersion,
		"record_role":   scale.RecordRolePublishedSnapshot.String(),
		"deleted_at":    nil,
	}, bson.M{"$set": bson.M{
		"is_active_published": true,
		"updated_at":          now,
		"updated_by":          userID,
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return scale.ErrNotFound
	}
	return nil
}

// ClearActivePublishedVersion 清空当前激活的已发布量表快照。
func (r *Repository) ClearActivePublishedVersion(ctx context.Context, code string) error {
	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)
	_, err := r.Collection().UpdateMany(ctx, bson.M{
		"code":        code,
		"record_role": scale.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}, bson.M{"$set": bson.M{
		"is_active_published": false,
		"updated_at":          now,
		"updated_by":          userID,
	}})
	return err
}

// Remove 删除量表（软删除）
func (r *Repository) Remove(ctx context.Context, code string) error {
	filter := bson.M{
		"code":       code,
		"deleted_at": nil,
	}

	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"updated_at": now,
			"updated_by": userID,
			"deleted_by": userID,
		},
	}

	result, err := r.Collection().UpdateMany(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return scale.ErrNotFound
	}

	return nil
}

// ExistsByCode 检查编码是否存在
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	count, err := r.Collection().CountDocuments(ctx, headFilter(code))
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
