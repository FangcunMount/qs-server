// Package modelcatalog owns published assessment model assets.
//
// v2 introduces Kind/SubKind/Algorithm identity, PublishedModelSnapshot,
// and unified personality typology payloads. Legacy ruleset.* payload formats
// remain readable for migration; new writes use assessmentmodel.* payload formats.
//
// KindCapability.Role separates product channels from executable model families.
// ProductChannel on AssessmentModel is a taxonomy field for product-facing classification;
// it must not drive runtime execution path selection. Use ModelFamilyCapabilities for
// executable model-family guards.
//
//   - behavior_ability (CapabilityRoleProductChannel): legacy API aggregation slot; new drafts
//     must use behavioral_rating or cognitive instead.
//   - behavioral_rating / cognitive (CapabilityRoleModelFamily): executable families;
//     Brief-2 lands on behavioral_rating+AlgorithmBrief2, SPM on cognitive+AlgorithmSPM.
//
// API kind behavior_ability maps to domain KindBehaviorAbility and executes as a
// scale adapter (assessmentmodel.behavior_ability.scale.v1). The API kind is also a
// product channel that aggregates behavioral_rating and cognitive listings. Domain KindBehavioralRating
// is the standalone behavioral_rating runtime (assessmentmodel.behavioral_rating.default.v1
// for legacy default models; assessmentmodel.behavioral_rating.brief2.v1 for Brief-2).
//
// KindCustom is a reserved catalog kind (API options disabled); it is unrelated to
// typology AlgorithmCustomTypology or plan scheduleType=custom.
// KindCognitive executes via ExecutionPathCognitiveDescriptor
// (assessmentmodel.cognitive.default.v1 legacy; assessmentmodel.cognitive.spm.v1 for SPM).
package modelcatalog
