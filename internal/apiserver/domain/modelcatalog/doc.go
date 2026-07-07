// Package modelcatalog owns published assessment model assets.
//
// # Overview
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
//   - behavioral_rating / cognitive (CapabilityRoleModelFamily): executable families;
//     Brief-2 lands on behavioral_rating+AlgorithmBrief2, SPM on cognitive+AlgorithmSPM.
//
// API kind behavior_ability is a product channel that aggregates behavioral_rating and
// cognitive listings only; it does not map to a domain Kind. Domain KindBehavioralRating
// is the standalone behavioral_rating runtime (assessmentmodel.behavioral_rating.default.v1
// for legacy default models; assessmentmodel.behavioral_rating.brief2.v1 for Brief-2).
//
// KindCustom is a reserved catalog kind (API options disabled); it is unrelated to
// typology AlgorithmCustomTypology or plan scheduleType=custom.
// KindCognitive executes via ExecutionPathCognitiveDescriptor
// (assessmentmodel.cognitive.default.v1 legacy; assessmentmodel.cognitive.spm.v1 for SPM).
//
// # Mechanism naming (package vs enum)
//
// Execution code uses short package names; API/routing uses AlgorithmFamily enums:
//
//	Go package (evaluation)     AlgorithmFamily
//	scoring                     factor_scoring
//	typology                    factor_classification
//	norming                     factor_norm
//	task_performance            task_performance
//
// See docs/02-业务模块/mechanism-oriented-migration.md §包名与 AlgorithmFamily 对照表.
//
// # Root package file map (Round 16 baseline)
//
// Files below live in package modelcatalog. Round 16+ may move them into subpackages
// (identity/, routing/, catalog/, capability/, legacy/) with type aliases at the root.
//
// Identity — draft/published model identity and product taxonomy (identity/ subpackage):
//   - export.go: root type aliases
//   - identity/types.go: Kind, SubKind, Algorithm, DecisionKind
//   - identity/product_channel.go: ProductChannel
//   - identity/personality_decision.go: FallbackPersonalityDecisionKind
//
// Routing — execution family and materialization path (routing/ subpackage):
//   - export.go: root type aliases
//   - routing/algorithm_family.go: AlgorithmFamily, identity→family mapping
//   - routing/execution_path.go: ExecutionPath
//   - routing/payload_format.go: PayloadFormat constants and helpers
//
// Catalog — aggregates, envelopes, validation (catalog/ subpackage):
//   - export.go: root type aliases
//   - catalog/snapshot.go: PublishedModelSnapshot, ModelDefinition, QuestionnaireBinding
//   - catalog/aggregate.go: AssessmentModel, NewAssessmentModel
//   - catalog/definition.go: DefinitionPayload
//   - catalog/status.go: ModelStatus
//   - catalog/validation.go: domain validation issues
//
// Capability — API/catalog operation guards (capability/ subpackage):
//   - export.go: root type aliases
//   - capability/capability.go, capability_role.go: KindCapability matrix
//   - capability/operation.go: CatalogOperation
//
// Legacy / compatibility — migration readers and behavior_ability product channel (legacy/ subpackage):
//   - legacy/alias.go: v1 Snapshot, Definition, RuleSetKind aliases
//   - legacy/adapter.go: LegacyKindMapping, PublishedFromLegacy
//   - legacy/behavior_ability.go, legacy/behavior_ability_channel.go
//   - legacy/kind_mapping.go
//
// Shared:
//   - errors.go: domain errors
//   - export.go: root facade aliases to subpackages
//   - architecture_test.go: root package guard (doc/errors/export only)
//
// # Subpackages (by model family or mechanism metadata)
//
//   - identity/: Kind, Algorithm, SubKind, DecisionKind, ProductChannel
//   - routing/: AlgorithmFamily, ExecutionPath, PayloadFormat
//   - capability/: KindCapability, CatalogOperation
//   - factor/: shared FactorSnapshot, hierarchy, scoring/classification specs
//   - norming/: norm/composite-index metadata (AlgorithmFamily factor_norm)
//   - task_performance/: task metadata (AlgorithmFamily task_performance)
//   - personality/: typology payload, publish, validator
//   - scale/: scale definition and snapshot
//   - behavioral_rating/: behavioral_rating snapshot (incl. Brief-2 profile)
//   - cognitive/: cognitive snapshot (incl. SPM profile)
//   - catalog/: AssessmentModel aggregate, published snapshots, validation
//   - legacy/: v1 envelopes, adapters, behavior_ability channel
package modelcatalog
