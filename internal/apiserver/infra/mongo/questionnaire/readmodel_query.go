package questionnaire

import (
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func questionnaireHeadReadModelPipeline(filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) []bson.M {
	pipeline := []bson.M{
		{"$match": questionnaireHeadReadModelFilter(filter)},
		{"$sort": bson.M{"updated_at": -1}},
	}
	if skip := questionnaireReadModelPaginationSkip(page.Page, page.PageSize); skip > 0 {
		pipeline = append(pipeline, bson.M{"$skip": skip})
	}
	if limit := questionnaireReadModelPaginationLimit(page.Page, page.PageSize); limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": limit})
	}
	pipeline = append(pipeline, questionnaireReadModelProjectStage())
	return pipeline
}

func questionnairePublishedReadModelPipeline(filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) []bson.M {
	pipeline := []bson.M{
		{"$match": questionnairePublishedReadModelFilter(filter)},
		{"$addFields": bson.M{"published_priority": questionnairePublishedReadModelPriorityExpr()}},
		{"$sort": bson.M{"code": 1, "published_priority": -1, "updated_at": -1}},
		{"$group": bson.M{"_id": "$code", "doc": bson.M{"$first": "$$ROOT"}}},
		{"$replaceRoot": bson.M{"newRoot": "$doc"}},
		{"$sort": bson.M{"updated_at": -1}},
	}
	if skip := questionnaireReadModelPaginationSkip(page.Page, page.PageSize); skip > 0 {
		pipeline = append(pipeline, bson.M{"$skip": skip})
	}
	if limit := questionnaireReadModelPaginationLimit(page.Page, page.PageSize); limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": limit})
	}
	pipeline = append(pipeline, questionnaireReadModelProjectStage())
	return pipeline
}

func questionnairePublishedReadModelCountPipeline(filter surveyreadmodel.QuestionnaireFilter) []bson.M {
	return []bson.M{
		{"$match": questionnairePublishedReadModelFilter(filter)},
		{"$addFields": bson.M{"published_priority": questionnairePublishedReadModelPriorityExpr()}},
		{"$sort": bson.M{"code": 1, "published_priority": -1, "updated_at": -1}},
		{"$group": bson.M{"_id": "$code"}},
		{"$count": "total"},
	}
}

func questionnaireHeadReadModelFilter(filter surveyreadmodel.QuestionnaireFilter) bson.M {
	query := questionnaireReadModelCommonFilter(filter)
	query["$or"] = questionnaireReadModelHeadRoleCandidates()
	return query
}

func questionnairePublishedReadModelFilter(filter surveyreadmodel.QuestionnaireFilter) bson.M {
	query := questionnaireReadModelCommonFilter(filter)
	statusValue, hasStatus := query["status"]
	if !hasStatus {
		statusValue = domainQuestionnaire.STATUS_PUBLISHED.String()
	}
	delete(query, "status")

	query["$or"] = bson.A{
		bson.M{
			"record_role":         domainQuestionnaire.RecordRolePublishedSnapshot.String(),
			"is_active_published": true,
			"status":              statusValue,
		},
		bson.M{
			"status": statusValue,
			"$or":    questionnaireReadModelHeadRoleCandidates(),
		},
	}
	return query
}

func questionnaireReadModelCommonFilter(filter surveyreadmodel.QuestionnaireFilter) bson.M {
	query := bson.M{"deleted_at": nil}
	if filter.Title != "" {
		query["title"] = bson.M{"$regex": filter.Title, "$options": "i"}
	}
	if filter.Status != "" {
		if parsed, ok := domainQuestionnaire.ParseStatus(filter.Status); ok {
			query["status"] = parsed.String()
		}
	}
	if filter.Type != "" {
		query["type"] = filter.Type
	}
	return query
}

func questionnaireReadModelHeadRoleCandidates() bson.A {
	return bson.A{
		bson.M{"record_role": domainQuestionnaire.RecordRoleHead.String()},
		bson.M{"record_role": bson.M{"$exists": false}},
		bson.M{"record_role": ""},
	}
}

func questionnaireReadModelPaginationLimit(page, pageSize int) int64 {
	if page <= 0 || pageSize <= 0 {
		return 0
	}
	return int64(pageSize)
}

func questionnaireReadModelPaginationSkip(page, pageSize int) int64 {
	if page <= 1 || pageSize <= 0 {
		return 0
	}
	return int64((page - 1) * pageSize)
}

func questionnairePublishedReadModelPriorityExpr() bson.M {
	return bson.M{"$cond": bson.A{
		bson.M{"$and": bson.A{
			bson.M{"$eq": bson.A{"$record_role", domainQuestionnaire.RecordRolePublishedSnapshot.String()}},
			bson.M{"$eq": bson.A{"$is_active_published", true}},
		}},
		2,
		1,
	}}
}

func questionnaireReadModelProjectStage() bson.M {
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
