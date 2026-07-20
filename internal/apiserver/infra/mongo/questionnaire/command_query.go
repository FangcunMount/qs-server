package questionnaire

import (
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"go.mongodb.org/mongo-driver/bson"
)

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

func headRevisionFilter(code string, expectedRevision int64) bson.M {
	filter := headFilter(code)
	if expectedRevision == 0 {
		filter["$and"] = bson.A{
			bson.M{"$or": headRoleCandidates()},
			bson.M{"$or": bson.A{
				bson.M{"revision": 0},
				bson.M{"revision": bson.M{"$exists": false}},
			}},
		}
		delete(filter, "$or")
		return filter
	}
	filter["revision"] = expectedRevision
	return filter
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

func commandPublishedCodeMatch(code string) bson.M {
	statusValue := domainQuestionnaire.STATUS_PUBLISHED.String()
	return bson.M{
		"code":       code,
		"deleted_at": nil,
		"$or": bson.A{
			bson.M{"$and": bson.A{
				bson.M{
					"record_role": domainQuestionnaire.RecordRolePublishedSnapshot.String(),
					"status":      statusValue,
				},
				questionnaireActiveReleaseClause(),
			}},
			bson.M{
				"status": statusValue,
				"$or":    headRoleCandidates(),
			},
		},
	}
}

func commandHeadBasePipeline(filter bson.M) []bson.M {
	return []bson.M{
		{"$match": filter},
		{"$sort": bson.M{"updated_at": -1}},
		commandBaseProjectStage(),
	}
}

func commandPublishedBasePipeline(filter bson.M) []bson.M {
	return []bson.M{
		{"$match": filter},
		{"$addFields": bson.M{"published_priority": commandPublishedPriorityExpr()}},
		{"$sort": bson.M{"code": 1, "published_priority": -1, "updated_at": -1}},
		{"$group": bson.M{"_id": "$code", "doc": bson.M{"$first": "$$ROOT"}}},
		{"$replaceRoot": bson.M{"newRoot": "$doc"}},
		{"$sort": bson.M{"updated_at": -1}},
		commandBaseProjectStage(),
	}
}

func commandPublishedPriorityExpr() bson.M {
	return bson.M{"$cond": bson.A{
		bson.M{"$and": bson.A{
			bson.M{"$eq": bson.A{"$record_role", domainQuestionnaire.RecordRolePublishedSnapshot.String()}},
			bson.M{"$or": bson.A{
				bson.M{"$eq": bson.A{"$release_status", string(domainQuestionnaire.ReleaseStatusActive)}},
				bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$is_active_published", true}},
					bson.M{"$eq": bson.A{bson.M{"$type": "$release_status"}, "missing"}},
				}},
			}},
		}},
		2,
		1,
	}}
}

func questionnaireActiveReleaseClause() bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"release_status": string(domainQuestionnaire.ReleaseStatusActive)},
		bson.M{
			"release_status":      bson.M{"$exists": false},
			"is_active_published": true,
		},
	}}
}

func commandBaseProjectStage() bson.M {
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
		"release_status":      1,
		"published_at":        1,
		"release_archived_at": 1,
		"question_count":      1,
		"created_by":          1,
		"created_at":          1,
		"updated_by":          1,
		"updated_at":          1,
	}}
}
