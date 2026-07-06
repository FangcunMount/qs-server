// Package modelcatalog owns published assessment model assets.
//
// v2 introduces Kind/SubKind/Algorithm identity, PublishedModelSnapshot,
// and unified personality typology payloads. Legacy ruleset.* payload formats
// remain readable for migration; new writes use modelcatalog.* formats.
package modelcatalog
