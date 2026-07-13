package local

import (
	"reflect"
	"strings"
	"time"
)

const defaultTTL = 180 * time.Second

// MultiHooks 描述多桶 catalog L1 的 key 与 clone 策略。
type MultiHooks[TDetail, TList, TCategories, THot any] struct {
	DetailKey       func(code string) string
	ListKey         func(req any) string
	CategoriesKey   string
	HotKey          func(req any) string
	ListPrefix      string
	HotPrefix       string
	CloneDetail     func(TDetail) TDetail
	CloneList       func(TList) TList
	CloneCategories func(TCategories) TCategories
	CloneHot        func(THot) THot
}

// MultiCache 量表/人格模型等多桶 catalog L1。
type MultiCache[TDetail, TList, TCategories, THot any] struct {
	hooks      MultiHooks[TDetail, TList, TCategories, THot]
	detail     *Cache[TDetail]
	list       *Cache[TList]
	categories *Cache[TCategories]
	hot        *Cache[THot]
}

// NewMultiCache 创建多桶 catalog L1。
func NewMultiCache[TDetail, TList, TCategories, THot any](opts Options, hooks MultiHooks[TDetail, TList, TCategories, THot]) *MultiCache[TDetail, TList, TCategories, THot] {
	opts = opts.withDefaults(defaultTTL, 256)
	c := &MultiCache[TDetail, TList, TCategories, THot]{hooks: hooks}
	if hooks.CloneDetail != nil {
		c.detail = New(opts, hooks.CloneDetail)
	}
	if hooks.CloneList != nil {
		c.list = New(opts, hooks.CloneList)
	}
	if hooks.CloneCategories != nil {
		c.categories = New(opts, hooks.CloneCategories)
	}
	if hooks.CloneHot != nil {
		c.hot = New(opts, hooks.CloneHot)
	}
	return c
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) GetDetail(code string) (TDetail, bool) {
	var zero TDetail
	if c == nil || c.detail == nil || c.hooks.DetailKey == nil {
		return zero, false
	}
	return c.detail.Get(c.hooks.DetailKey(code))
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) SetDetail(code string, value TDetail) {
	if c == nil || c.detail == nil || c.hooks.DetailKey == nil || isNilValue(value) {
		return
	}
	c.detail.Set(c.hooks.DetailKey(code), value)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) GetList(key string) (TList, bool) {
	var zero TList
	if c == nil || c.list == nil {
		return zero, false
	}
	return c.list.Get(key)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) SetList(key string, value TList) {
	if c == nil || c.list == nil || isNilValue(value) {
		return
	}
	c.list.Set(key, value)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) GetListByRequest(req any) (TList, bool) {
	if c == nil || c.list == nil || c.hooks.ListKey == nil {
		var zero TList
		return zero, false
	}
	return c.list.Get(c.hooks.ListKey(req))
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) SetListByRequest(req any, value TList) {
	if c == nil || c.list == nil || c.hooks.ListKey == nil || isNilValue(value) {
		return
	}
	c.list.Set(c.hooks.ListKey(req), value)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) GetCategories() (TCategories, bool) {
	var zero TCategories
	if c == nil || c.categories == nil || c.hooks.CategoriesKey == "" {
		return zero, false
	}
	return c.categories.Get(c.hooks.CategoriesKey)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) SetCategories(value TCategories) {
	if c == nil || c.categories == nil || c.hooks.CategoriesKey == "" || isNilValue(value) {
		return
	}
	c.categories.Set(c.hooks.CategoriesKey, value)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) GetHot(key string) (THot, bool) {
	var zero THot
	if c == nil || c.hot == nil {
		return zero, false
	}
	return c.hot.Get(key)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) SetHot(key string, value THot) {
	if c == nil || c.hot == nil || isNilValue(value) {
		return
	}
	c.hot.Set(key, value)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) GetHotByRequest(req any) (THot, bool) {
	if c == nil || c.hot == nil || c.hooks.HotKey == nil {
		var zero THot
		return zero, false
	}
	return c.hot.Get(c.hooks.HotKey(req))
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) SetHotByRequest(req any, value THot) {
	if c == nil || c.hot == nil || c.hooks.HotKey == nil || isNilValue(value) {
		return
	}
	c.hot.Set(c.hooks.HotKey(req), value)
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) EvictOnSignal(code string) {
	if c == nil {
		return
	}
	code = strings.ToLower(strings.TrimSpace(code))
	if code != "" && c.detail != nil && c.hooks.DetailKey != nil {
		c.detail.Delete(c.hooks.DetailKey(code))
	}
	if c.list != nil && c.hooks.ListPrefix != "" {
		c.list.DeletePrefix(c.hooks.ListPrefix)
	}
	if c.categories != nil && c.hooks.CategoriesKey != "" {
		c.categories.Delete(c.hooks.CategoriesKey)
	}
	if c.hot != nil && c.hooks.HotPrefix != "" {
		c.hot.DeletePrefix(c.hooks.HotPrefix)
	}
}

func (c *MultiCache[TDetail, TList, TCategories, THot]) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	for _, part := range []*Cache[TDetail]{c.detail} {
		if part == nil {
			continue
		}
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*Cache[TList]{c.list} {
		if part == nil {
			continue
		}
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*Cache[TCategories]{c.categories} {
		if part == nil {
			continue
		}
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*Cache[THot]{c.hot} {
		if part == nil {
			continue
		}
		h, m := part.Stats()
		hits += h
		misses += m
	}
	return hits, misses
}

func isNilValue[T any](v T) bool {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Interface, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
