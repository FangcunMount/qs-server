package handler

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/gin-gonic/gin"
)

func metaIDPtrToUint64(id *meta.ID) *uint64 {
	if id == nil || id.IsZero() {
		return nil
	}

	value := id.Uint64()
	return &value
}

func flexibleTimePtrToTimePtr(v *request.FlexibleTime) *time.Time {
	if v == nil || v.IsZero() {
		return nil
	}

	value := v.Time
	return &value
}

func paginationFromContext(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func parseGender(value string) int8 {
	switch value {
	case "male", "男":
		return 1
	case "female", "女":
		return 2
	default:
		return 0
	}
}

func buildRelationResponse(
	id uint64,
	orgID int64,
	clinicianID uint64,
	testeeID uint64,
	relationType string,
	sourceType string,
	sourceID *uint64,
	isActive bool,
	boundAt time.Time,
	unboundAt *time.Time,
) *response.RelationResponse {
	return &response.RelationResponse{
		ID:                strconv.FormatUint(id, 10),
		OrgID:             strconv.FormatInt(orgID, 10),
		ClinicianID:       strconv.FormatUint(clinicianID, 10),
		TesteeID:          strconv.FormatUint(testeeID, 10),
		RelationType:      relationType,
		RelationTypeLabel: response.LabelForRelationType(relationType),
		SourceType:        sourceType,
		SourceTypeLabel:   response.LabelForRelationSource(sourceType),
		SourceID:          uint64StringPtr(sourceID),
		IsActive:          isActive,
		IsActiveLabel:     boolLabel(isActive, "有效", "失效"),
		BoundAt:           response.FormatDateTimeValue(boundAt),
		UnboundAt:         response.FormatDateTimePtr(unboundAt),
	}
}

func buildTesteeSummaryResponse(
	id uint64,
	orgID int64,
	profileID *uint64,
	name string,
	genderValue int8,
	birthday *time.Time,
	tags []string,
	source string,
	isKeyFocus bool,
) *response.TesteeResponse {
	gender := response.GenderCodeFromValue(genderValue)
	profileIDStr := uint64StringPtr(profileID)

	return &response.TesteeResponse{
		ID:              strconv.FormatUint(id, 10),
		OrgID:           strconv.FormatInt(orgID, 10),
		ProfileID:       profileIDStr,
		IAMChildID:      response.LegacyIAMChildIDAlias(profileIDStr),
		Name:            name,
		Gender:          gender,
		GenderLabel:     response.LabelForGender(gender),
		Birthday:        response.FormatDatePtr(birthday),
		Tags:            tags,
		TagsLabel:       response.LabelTags(tags),
		Source:          source,
		SourceLabel:     response.LabelForTesteeSource(source),
		IsKeyFocus:      isKeyFocus,
		IsKeyFocusLabel: response.LabelForKeyFocus(isKeyFocus),
	}
}

func uint64StringPtr(value *uint64) *string {
	if value == nil {
		return nil
	}
	text := strconv.FormatUint(*value, 10)
	return &text
}

func boolLabel(value bool, trueLabel, falseLabel string) string {
	if value {
		return trueLabel
	}
	return falseLabel
}
