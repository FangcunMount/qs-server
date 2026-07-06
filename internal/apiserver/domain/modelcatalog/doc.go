// Package modelcatalog owns published assessment model assets.
//
// v2 introduces Kind/SubKind/Algorithm identity, PublishedModelSnapshot,
// and unified personality typology payloads. Legacy ruleset.* payload formats
// remain readable for migration; new writes use assessmentmodel.* payload formats.
//
// API kind behavior_ability maps to domain KindBehaviorAbility and executes as a
// scale adapter (assessmentmodel.behavior_ability.scale.v1). Domain KindBehavioralRating
// is the standalone behavioral_rating runtime (assessmentmodel.behavioral_rating.default.v1).
package modelcatalog
