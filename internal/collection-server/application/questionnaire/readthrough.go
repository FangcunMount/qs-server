package questionnaire

import localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"

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
	return localcache.ReadThrough(key, get, setFn, load, cloneResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}
