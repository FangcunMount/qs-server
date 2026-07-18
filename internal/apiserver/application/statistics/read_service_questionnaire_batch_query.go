package statistics

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type contentBatchQuery struct {
	readModel ContentStatisticsReader
}

func (q *contentBatchQuery) GetContentBatchStatistics(ctx context.Context, orgID int64, refs []domainStatistics.ContentReference, access ContentStatisticsAccess) (*domainStatistics.ContentBatchStatisticsResponse, error) {
	cleanRefs, err := normalizeContentReferences(refs)
	if err != nil {
		return nil, err
	}
	if err := validateContentStatisticsAccess(cleanRefs, access); err != nil {
		return nil, err
	}
	items := make([]*domainStatistics.ContentBatchStatisticsItem, 0, len(cleanRefs))
	if len(cleanRefs) == 0 {
		return &domainStatistics.ContentBatchStatisticsResponse{Items: items}, nil
	}

	readRefs := make([]ContentReference, 0, len(cleanRefs))
	for _, ref := range cleanRefs {
		readRefs = append(readRefs, ContentReference{Type: string(ref.Type), Code: ref.Code})
	}
	totals, err := q.readModel.GetContentBatchTotals(ctx, orgID, readRefs)
	if err != nil {
		return nil, err
	}

	resultByRef := make(map[domainStatistics.ContentReference]*domainStatistics.ContentBatchStatisticsItem, len(cleanRefs))
	for _, ref := range cleanRefs {
		resultByRef[ref] = &domainStatistics.ContentBatchStatisticsItem{Type: ref.Type, Code: ref.Code}
	}
	for _, total := range totals {
		ref := domainStatistics.ContentReference{Type: domainStatistics.ContentType(total.Type), Code: total.Code}
		item := resultByRef[ref]
		if item == nil {
			continue
		}
		item.TotalSubmissions = total.TotalSubmissions
		item.TotalCompletions = total.TotalCompletions
		item.CompletionRate = domainStatistics.CompletionRate(item.TotalSubmissions, item.TotalCompletions)
	}

	for _, ref := range cleanRefs {
		items = append(items, resultByRef[ref])
	}

	return &domainStatistics.ContentBatchStatisticsResponse{Items: items}, nil
}

func validateContentStatisticsAccess(refs []domainStatistics.ContentReference, access ContentStatisticsAccess) error {
	if len(refs) == 0 && !access.Questionnaire && !access.Scale {
		return errors.WithCode(code.ErrPermissionDenied, "content statistics capability is required")
	}
	for _, ref := range refs {
		switch ref.Type {
		case domainStatistics.ContentTypeQuestionnaire:
			if !access.Questionnaire {
				return errors.WithCode(code.ErrPermissionDenied, "questionnaire statistics capability is required")
			}
		case domainStatistics.ContentTypeScale:
			if !access.Scale {
				return errors.WithCode(code.ErrPermissionDenied, "scale statistics capability is required")
			}
		}
	}
	return nil
}

func normalizeContentReferences(refs []domainStatistics.ContentReference) ([]domainStatistics.ContentReference, error) {
	cleanRefs := make([]domainStatistics.ContentReference, 0, len(refs))
	seen := make(map[domainStatistics.ContentReference]struct{}, len(refs))
	for _, ref := range refs {
		ref.Type = domainStatistics.ContentType(strings.ToLower(strings.TrimSpace(string(ref.Type))))
		ref.Code = strings.TrimSpace(ref.Code)
		if ref.Code == "" {
			return nil, errors.WithCode(code.ErrInvalidArgument, "content code is required")
		}
		switch ref.Type {
		case domainStatistics.ContentTypeQuestionnaire, domainStatistics.ContentTypeScale:
		default:
			return nil, errors.WithCode(code.ErrInvalidArgument, "unsupported content type: %s", ref.Type)
		}
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		cleanRefs = append(cleanRefs, ref)
	}
	return cleanRefs, nil
}
