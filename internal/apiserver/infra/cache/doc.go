// Package cache contains the apiserver object repository cache decorators.
//
// Query/list cache primitives live in infra/cachequery, Redis payload primitives
// live in infra/cacheentry, and hotset governance storage lives in infra/cachehotset.
// Keep this package focused on repository object cache wiring and read-through
// behavior.
package cache
