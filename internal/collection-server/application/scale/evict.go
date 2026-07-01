package scale

// EvictCatalogOnSignal 按 scale_cache_changed 信令失效 L1。
func EvictCatalogOnSignal(cache CatalogCache, code string) {
	if cache == nil || code == "" {
		return
	}
	cache.EvictOnSignal(code)
}
