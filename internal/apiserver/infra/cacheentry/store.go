package cacheentry

import (
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// Cache is a transitional alias to the shared cache Store contract.
type Cache = sharedcache.Store

// ErrCacheNotFound is kept during adapter migration and preserves error identity.
var ErrCacheNotFound = sharedcache.ErrMiss
