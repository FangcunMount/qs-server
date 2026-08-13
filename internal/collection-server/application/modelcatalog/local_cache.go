package modelcatalog

import localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"

// LocalPublishedModelCache stores detail, list and options in independent
// bounded FIFO buckets.
type LocalPublishedModelCache struct {
	detail  *localcache.Cache[*ModelResponse]
	list    *localcache.Cache[*ListResponse]
	options *localcache.Cache[*OptionsResponse]
}

func NewLocalPublishedModelCache(detail, list, options localcache.Options) *LocalPublishedModelCache {
	return &LocalPublishedModelCache{
		detail:  localcache.New(detail, cloneModelResponse),
		list:    localcache.New(list, cloneListResponse),
		options: localcache.New(options, cloneOptionsResponse),
	}
}

func (c *LocalPublishedModelCache) GetDetail(code string) (*ModelResponse, bool) {
	if c == nil || c.detail == nil || normalizedModelCode(code) == "" {
		return nil, false
	}
	return c.detail.Get(publishedModelDetailCacheKey(code))
}

func (c *LocalPublishedModelCache) SetDetail(code string, value *ModelResponse) {
	if c == nil || c.detail == nil || normalizedModelCode(code) == "" || value == nil {
		return
	}
	c.detail.Set(publishedModelDetailCacheKey(code), value)
}

func (c *LocalPublishedModelCache) GetListByRequest(request *ListRequest) (*ListResponse, bool) {
	if c == nil || c.list == nil {
		return nil, false
	}
	key, err := publishedModelListCacheKey(request)
	if err != nil {
		return nil, false
	}
	return c.list.Get(key)
}

func (c *LocalPublishedModelCache) SetListByRequest(request *ListRequest, value *ListResponse) {
	if c == nil || c.list == nil || value == nil {
		return
	}
	key, err := publishedModelListCacheKey(request)
	if err != nil {
		return
	}
	c.list.Set(key, value)
}

func (c *LocalPublishedModelCache) GetOptions(kind string) (*OptionsResponse, bool) {
	if c == nil || c.options == nil {
		return nil, false
	}
	return c.options.Get(publishedModelOptionsCacheKey(kind))
}

func (c *LocalPublishedModelCache) SetOptions(kind string, value *OptionsResponse) {
	if c == nil || c.options == nil || value == nil {
		return
	}
	c.options.Set(publishedModelOptionsCacheKey(kind), value)
}

func (c *LocalPublishedModelCache) EvictOnSignal(code string) {
	if c == nil {
		return
	}
	if c.detail != nil && normalizedModelCode(code) != "" {
		c.detail.DeleteWithReason(publishedModelDetailCacheKey(code), localcache.EvictionReasonSignal)
	}
	if c.list != nil {
		c.list.DeletePrefixWithReason(publishedModelListKeyPrefix, localcache.EvictionReasonSignal)
	}
	if c.options != nil {
		c.options.DeletePrefixWithReason(publishedModelOptionsKeyPrefix, localcache.EvictionReasonSignal)
	}
}

func (c *LocalPublishedModelCache) RuntimeBuckets() []localcache.BucketSnapshot {
	if c == nil {
		return nil
	}
	result := make([]localcache.BucketSnapshot, 0, 3)
	if c.detail != nil {
		result = append(result, localcache.BucketSnapshot{Bucket: "detail", Stats: c.detail.RuntimeSnapshot()})
	}
	if c.list != nil {
		result = append(result, localcache.BucketSnapshot{Bucket: "list", Stats: c.list.RuntimeSnapshot()})
	}
	if c.options != nil {
		result = append(result, localcache.BucketSnapshot{Bucket: "options", Stats: c.options.RuntimeSnapshot()})
	}
	return result
}

var _ PublishedModelCache = (*LocalPublishedModelCache)(nil)
