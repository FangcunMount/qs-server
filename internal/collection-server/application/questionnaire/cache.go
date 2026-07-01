package questionnaire

// PublishedDetailCache 缓存已发布问卷 REST DTO（collection BFF 进程内 L1）。
type PublishedDetailCache interface {
	Get(code, version string) (*QuestionnaireResponse, bool)
	Set(code, version string, value *QuestionnaireResponse)
	Delete(code, version string)
	Stats() (hits, misses uint64)
}

const defaultLocalCacheTTLSeconds = 180
