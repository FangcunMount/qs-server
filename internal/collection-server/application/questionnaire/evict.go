package questionnaire

// EvictPublishedDetail 按 Redis 信令失效 L1 条目。
// version 为空时清除该 code 下全部版本；否则同时清除指定版本与默认（latest）条目。
func EvictPublishedDetail(cache PublishedDetailCache, code, version string) {
	if cache == nil || code == "" {
		return
	}
	if version == "" {
		cache.Delete(code, "")
		return
	}
	cache.Delete(code, version)
	cache.Delete(code, "")
}
