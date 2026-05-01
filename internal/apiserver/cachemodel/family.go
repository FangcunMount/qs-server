package cachemodel

// Family identifies a logical cache/governance workload without exposing the
// Redis runtime implementation that currently backs it.
type Family string

const (
	FamilyDefault Family = "default"
	FamilyStatic  Family = "static_meta"
	FamilyObject  Family = "object_view"
	FamilyQuery   Family = "query_result"
	FamilyMeta    Family = "meta_hotset"
	FamilyRank    Family = "business_rank"
	FamilySDK     Family = "sdk_token"
	FamilyLock    Family = "lock_lease"
	FamilyOps     Family = "ops_runtime"
)
