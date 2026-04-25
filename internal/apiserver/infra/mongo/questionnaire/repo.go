package questionnaire

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// Repository 问卷 MongoDB 存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷 MongoDB 存储库
func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) domainQuestionnaire.Repository {
	po := &QuestionnairePO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:         NewQuestionnaireMapper(),
	}
}

// Create 创建问卷 head
func (r *Repository) Create(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	qDomain.SetRecordRole(domainQuestionnaire.RecordRoleHead)
	qDomain.SetActivePublished(false)

	po := r.mapper.ToPO(qDomain)
	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()
	po.RecordRole = domainQuestionnaire.RecordRoleHead.String()
	po.IsActivePublished = false

	insertData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	_, err = r.InsertOne(ctx, insertData)
	return err
}

// CreatePublishedSnapshot 创建或更新已发布快照
func (r *Repository) CreatePublishedSnapshot(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire, active bool) error {
	po := r.mapper.ToPO(qDomain)
	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()
	po.RecordRole = domainQuestionnaire.RecordRolePublishedSnapshot.String()
	po.IsActivePublished = active

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	filter := bson.M{
		"code":        qDomain.GetCode().Value(),
		"version":     qDomain.GetVersion().Value(),
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}
	update := bson.M{"$set": updateData}

	_, err = r.Collection().UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// FindByCode 根据编码查询工作版本
func (r *Repository) FindByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	q, err := r.FindBaseByCode(ctx, code)
	if err != nil || q == nil {
		return q, err
	}
	if err := r.LoadQuestions(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// FindPublishedByCode 根据编码查询当前已发布版本
func (r *Repository) FindPublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	q, err := r.FindBasePublishedByCode(ctx, code)
	if err != nil || q == nil {
		return q, err
	}
	if err := r.LoadQuestions(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// FindLatestPublishedByCode 根据编码查询最新已发布快照（无激活快照时用于恢复 head）
func (r *Repository) FindLatestPublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	po, err := r.findLatestPublishedPO(ctx, code)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	q := r.mapper.ToBO(po)
	if err := r.LoadQuestions(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// FindByCodeVersion 根据编码和版本查询问卷
func (r *Repository) FindByCodeVersion(ctx context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	q, err := r.FindBaseByCodeVersion(ctx, code, version)
	if err != nil || q == nil {
		return q, err
	}
	if err := r.LoadQuestions(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// FindBaseByCode 根据编码查询工作版本基础信息
func (r *Repository) FindBaseByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	po, err := r.aggregateOne(ctx, buildHeadBasePipeline(headFilter(code), 0, 1))
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return r.mapper.ToBO(po), nil
}

// FindBasePublishedByCode 根据编码查询当前已发布版本基础信息。
// 为兼容历史未迁移数据，若没有 active snapshot，会回退到已发布 head。
func (r *Repository) FindBasePublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	pipeline := buildPublishedBasePipeline(codeOnlyPublishedMatch(code), 0, 1)
	po, err := r.aggregateOne(ctx, pipeline)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return r.mapper.ToBO(po), nil
}

// FindBaseByCodeVersion 根据编码和版本查询问卷基础信息
func (r *Repository) FindBaseByCodeVersion(ctx context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	if version == "" {
		return nil, nil
	}

	snapshotFilter := bson.M{
		"code":        code,
		"version":     version,
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}
	po, err := r.findOnePO(ctx, snapshotFilter)
	if err == nil {
		return r.mapper.ToBO(po), nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	po, err = r.findOnePO(ctx, headVersionFilter(code, version))
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return r.mapper.ToBO(po), nil
}

// LoadQuestions 加载问卷问题详情
func (r *Repository) LoadQuestions(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	filter := roleAwareQuestionFilter(qDomain)
	projection := bson.M{"questions": 1}

	logger.L(ctx).Debugw("Mongo 查询问卷题目",
		"collection", r.Collection().Name(),
		"filter", filter,
		"projection", projection,
	)

	var po QuestionnairePO
	if err := r.Collection().FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&po); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return err
	}

	qDomain.SetQuestions(r.mapper.mapQuestions(po.Questions))
	return nil
}

// FindBaseList 查询工作版本列表
func (r *Repository) FindBaseList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	pipeline := buildHeadBasePipeline(buildHeadListFilter(conditions), paginationSkip(page, pageSize), paginationLimit(page, pageSize))
	return r.aggregateList(ctx, pipeline)
}

// FindBasePublishedList 查询已发布问卷列表（按 code 去重，优先 active snapshot）
func (r *Repository) FindBasePublishedList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	pipeline := buildPublishedBasePipeline(buildPublishedListFilter(conditions), paginationSkip(page, pageSize), paginationLimit(page, pageSize))
	return r.aggregateList(ctx, pipeline)
}

// CountWithConditions 统计工作版本数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	return r.CountDocuments(ctx, buildHeadListFilter(conditions))
}

// CountPublishedWithConditions 统计已发布问卷数量（按 code 去重）
func (r *Repository) CountPublishedWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	pipeline := []bson.M{
		{"$match": buildPublishedListFilter(conditions)},
		{"$addFields": bson.M{"published_priority": publishedPriorityExpr()}},
		{"$sort": bson.M{"code": 1, "published_priority": -1, "updated_at": -1}},
		{"$group": bson.M{"_id": "$code"}},
		{"$count": "total"},
	}

	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	var result struct {
		Total int64 `bson:"total"`
	}
	if err := cursor.Decode(&result); err != nil {
		return 0, err
	}
	return result.Total, nil
}

// Update 更新或恢复 head 记录
func (r *Repository) Update(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	qDomain.SetRecordRole(domainQuestionnaire.RecordRoleHead)
	qDomain.SetActivePublished(false)

	po := r.mapper.ToPO(qDomain)
	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()
	po.RecordRole = domainQuestionnaire.RecordRoleHead.String()
	po.IsActivePublished = false

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	filter := headFilter(qDomain.GetCode().Value())
	update := bson.M{"$set": updateData}

	_, err = r.Collection().UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// SetActivePublishedVersion 切换当前对外生效的已发布快照
func (r *Repository) SetActivePublishedVersion(ctx context.Context, code, version string) error {
	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)

	_, err := r.Collection().UpdateMany(ctx, bson.M{
		"code":        code,
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
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
		"code":        code,
		"version":     version,
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}, bson.M{"$set": bson.M{
		"is_active_published": true,
		"updated_at":          now,
		"updated_by":          userID,
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// ClearActivePublishedVersion 清空当前激活的已发布快照
func (r *Repository) ClearActivePublishedVersion(ctx context.Context, code string) error {
	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)
	_, err := r.Collection().UpdateMany(ctx, bson.M{
		"code":        code,
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}, bson.M{"$set": bson.M{
		"is_active_published": false,
		"updated_at":          now,
		"updated_by":          userID,
	}})
	return err
}

// Remove 软删除问卷族
func (r *Repository) Remove(ctx context.Context, code string) error {
	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)
	result, err := r.Collection().UpdateMany(ctx, bson.M{
		"code":       code,
		"deleted_at": nil,
	}, bson.M{"$set": bson.M{
		"deleted_at": now,
		"deleted_by": userID,
		"updated_at": now,
		"updated_by": userID,
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// HardDelete 物理删除 head
func (r *Repository) HardDelete(ctx context.Context, code string) error {
	result, err := r.DeleteOne(ctx, headFilter(code))
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// HardDeleteFamily 物理删除整个问卷族
func (r *Repository) HardDeleteFamily(ctx context.Context, code string) error {
	result, err := r.Collection().DeleteMany(ctx, bson.M{"code": code})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// ExistsByCode 检查工作版本是否存在
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.ExistsByFilter(ctx, headFilter(code))
}

// HasPublishedSnapshots 检查是否存在已发布快照
func (r *Repository) HasPublishedSnapshots(ctx context.Context, code string) (bool, error) {
	return r.ExistsByFilter(ctx, bson.M{
		"code":        code,
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	})
}

func (r *Repository) aggregateList(ctx context.Context, pipeline []bson.M) ([]*domainQuestionnaire.Questionnaire, error) {
	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	var questionnaires []*domainQuestionnaire.Questionnaire
	for cursor.Next(ctx) {
		var po QuestionnairePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		questionnaires = append(questionnaires, r.mapper.ToBO(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return questionnaires, nil
}

func (r *Repository) aggregateOne(ctx context.Context, pipeline []bson.M) (*QuestionnairePO, error) {
	logger.L(ctx).Debugw("Mongo 查询问卷基础信息",
		"collection", r.Collection().Name(),
		"pipeline", pipeline,
	)

	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		return nil, mongo.ErrNoDocuments
	}

	var po QuestionnairePO
	if err := cursor.Decode(&po); err != nil {
		return nil, err
	}
	return &po, nil
}

func (r *Repository) findOnePO(ctx context.Context, filter bson.M, opts ...*options.FindOneOptions) (*QuestionnairePO, error) {
	var po QuestionnairePO
	if err := r.Collection().FindOne(ctx, filter, opts...).Decode(&po); err != nil {
		return nil, err
	}
	return &po, nil
}

func (r *Repository) findLatestPublishedPO(ctx context.Context, code string) (*QuestionnairePO, error) {
	po, err := r.findOnePO(ctx, bson.M{
		"code":        code,
		"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
		"deleted_at":  nil,
	}, options.FindOne().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err == nil {
		return po, nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	return r.findOnePO(ctx, bson.M{
		"code":       code,
		"status":     domainQuestionnaire.STATUS_PUBLISHED.String(),
		"deleted_at": nil,
		"$or":        headRoleCandidates(),
	}, options.FindOne().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
}

func headRoleCandidates() bson.A {
	return bson.A{
		bson.M{"record_role": domainQuestionnaire.RecordRoleHead.String()},
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

func headVersionFilter(code, version string) bson.M {
	filter := headFilter(code)
	filter["version"] = version
	return filter
}

func roleAwareQuestionFilter(q *domainQuestionnaire.Questionnaire) bson.M {
	filter := bson.M{
		"code":       q.GetCode().Value(),
		"version":    q.GetVersion().Value(),
		"deleted_at": nil,
	}
	if q.IsPublishedSnapshot() {
		filter["record_role"] = domainQuestionnaire.RecordRolePublishedSnapshot.String()
		return filter
	}
	filter["$or"] = headRoleCandidates()
	return filter
}

func applyCommonConditions(filter bson.M, conditions map[string]interface{}) bson.M {
	if filter == nil {
		filter = bson.M{}
	}
	filter["deleted_at"] = nil
	if code, ok := conditions["code"].(string); ok && code != "" {
		filter["code"] = code
	}
	if title, ok := conditions["title"].(string); ok && title != "" {
		filter["title"] = bson.M{"$regex": title, "$options": "i"}
	}
	if status, ok := conditions["status"]; ok && status != nil {
		if value, ok := status.(string); ok && value != "" {
			if parsed, ok := domainQuestionnaire.ParseStatus(value); ok {
				filter["status"] = parsed.String()
			}
		}
	}
	if typ, ok := conditions["type"].(string); ok && typ != "" {
		filter["type"] = typ
	}
	return filter
}

func buildHeadListFilter(conditions map[string]interface{}) bson.M {
	filter := applyCommonConditions(bson.M{}, conditions)
	filter["$or"] = headRoleCandidates()
	return filter
}

func buildPublishedListFilter(conditions map[string]interface{}) bson.M {
	filter := applyCommonConditions(bson.M{}, conditions)
	statusValue, hasStatus := filter["status"]
	if !hasStatus {
		filter["status"] = domainQuestionnaire.STATUS_PUBLISHED.String()
		statusValue = filter["status"]
	}
	delete(filter, "status")

	filter["$or"] = bson.A{
		bson.M{
			"record_role":         domainQuestionnaire.RecordRolePublishedSnapshot.String(),
			"is_active_published": true,
			"status":              statusValue,
		},
		bson.M{
			"status": statusValue,
			"$or":    headRoleCandidates(),
		},
	}
	return filter
}

func codeOnlyPublishedMatch(code string) bson.M {
	return buildPublishedListFilter(map[string]interface{}{
		"code":   code,
		"status": domainQuestionnaire.STATUS_PUBLISHED.String(),
	})
}

func paginationLimit(page, pageSize int) int64 {
	if page <= 0 || pageSize <= 0 {
		return 0
	}
	return int64(pageSize)
}

func paginationSkip(page, pageSize int) int64 {
	if page <= 1 || pageSize <= 0 {
		return 0
	}
	return int64((page - 1) * pageSize)
}

func publishedPriorityExpr() bson.M {
	return bson.M{"$cond": bson.A{
		bson.M{"$and": bson.A{
			bson.M{"$eq": bson.A{"$record_role", domainQuestionnaire.RecordRolePublishedSnapshot.String()}},
			bson.M{"$eq": bson.A{"$is_active_published", true}},
		}},
		2,
		1,
	}}
}

func buildHeadBasePipeline(filter bson.M, skip, limit int64) []bson.M {
	pipeline := []bson.M{
		{"$match": filter},
		{"$sort": bson.M{"updated_at": -1}},
	}
	if skip > 0 {
		pipeline = append(pipeline, bson.M{"$skip": skip})
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": limit})
	}
	pipeline = append(pipeline, baseProjectStage())
	return pipeline
}

func buildPublishedBasePipeline(filter bson.M, skip, limit int64) []bson.M {
	pipeline := []bson.M{
		{"$match": filter},
		{"$addFields": bson.M{"published_priority": publishedPriorityExpr()}},
		{"$sort": bson.M{"code": 1, "published_priority": -1, "updated_at": -1}},
		{"$group": bson.M{"_id": "$code", "doc": bson.M{"$first": "$$ROOT"}}},
		{"$replaceRoot": bson.M{"newRoot": "$doc"}},
		{"$sort": bson.M{"updated_at": -1}},
	}
	if skip > 0 {
		pipeline = append(pipeline, bson.M{"$skip": skip})
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": limit})
	}
	pipeline = append(pipeline, baseProjectStage())
	return pipeline
}

func baseProjectStage() bson.M {
	return bson.M{"$project": bson.M{
		"code":                1,
		"title":               1,
		"description":         1,
		"img_url":             1,
		"version":             1,
		"status":              1,
		"type":                1,
		"record_role":         1,
		"is_active_published": 1,
		"question_count":      1,
		"created_by":          1,
		"created_at":          1,
		"updated_by":          1,
		"updated_at":          1,
	}}
}
