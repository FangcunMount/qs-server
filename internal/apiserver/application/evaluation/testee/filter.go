package testee

import (
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
)

func normalizePagination(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func parseDate(raw string, end bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		v, err := time.Parse(layout, raw)
		if err == nil {
			if layout == "2006-01-02" && end {
				v = v.Add(24 * time.Hour)
			}
			return &v, nil
		}
	}
	return nil, evalerrors.InvalidArgument("日期格式不正确")
}

func normalizeStatuses(raw string) []string {
	switch raw {
	case "":
		return nil
	case "pending":
		return []string{"pending", "submitted"}
	case "done":
		return []string{"evaluated"}
	default:
		return []string{raw}
	}
}
