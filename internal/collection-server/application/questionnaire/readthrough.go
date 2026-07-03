package questionnaire

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
)

func (s *QueryService) readThroughDetail(
	key string,
	get func() (*QuestionnaireResponse, bool),
	set func(*QuestionnaireResponse),
	load func() (*QuestionnaireResponse, error),
) (*QuestionnaireResponse, error) {
	var setFn func(*QuestionnaireResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, cloneResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}
