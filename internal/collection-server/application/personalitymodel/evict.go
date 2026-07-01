package personalitymodel

// EvictCatalogOnSignal 按 personality_model_cache_changed 信令失效 L1。
func EvictCatalogOnSignal(cache CatalogCache, code string) {
	if cache == nil || code == "" {
		return
	}
	cache.EvictOnSignal(code)
}
