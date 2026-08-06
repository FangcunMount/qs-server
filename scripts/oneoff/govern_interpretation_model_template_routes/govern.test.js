"use strict"

const test = require("node:test")
const assert = require("node:assert/strict")
const governance = require("./govern.js")

function snapshot(kind = "scale", templateID = "") {
  return {
    kind,
    code: "MODEL",
    release_version: "v3",
    definition_v2: {
      measure: {factors: [{code: "total"}, {code: "detail"}]},
      report_map: {sections: [{code: "result", kind: "factor_scores", template_id: templateID}]},
      interpretation_assets: {outcomes: [{outcome_code: "normal"}], report_spec: {sections: [{code: "stale"}]}}
    }
  }
}

test("materializeDefinition freezes standard release in both canonical layers", () => {
  const source = snapshot()
  const governed = governance.materializeDefinition(source.definition_v2, governance.resolveTemplateID(source), source.kind)
  assert.equal(governed.report_map.sections[0].template_id, "standard")
  assert.equal(governed.report_map.sections[0].template_version, "2026-08-v1")
  assert.deepEqual(governed.interpretation_assets.report_spec.sections, governed.report_map.sections)
  assert.equal(source.definition_v2.report_map.sections[0].template_version, undefined)
})

test("factor model without a report map gets one deterministic factor_scores section", () => {
  const source = snapshot()
  delete source.definition_v2.report_map
  delete source.definition_v2.interpretation_assets.report_spec
  const plan = governance.factorScorePlan(source.definition_v2, source.kind)
  assert.equal(plan.added, true)
  assert.deepEqual(plan.sourceRefs, ["total", "detail"])

  const governed = governance.materializeDefinition(source.definition_v2, "standard", source.kind)
  assert.deepEqual(governed.report_map.sections, [{
    code: "factor_scores",
    kind: "factor_scores",
    source_refs: ["total", "detail"],
    template_id: "standard",
    template_version: "2026-08-v1"
  }])
  assert.deepEqual(governed.interpretation_assets.report_spec.sections, governed.report_map.sections)
})

test("factor model preserves an explicit visibility subset and rejects ambiguous factors", () => {
  const source = snapshot()
  source.definition_v2.report_map.sections[0].source_refs = ["detail"]
  const plan = governance.factorScorePlan(source.definition_v2, source.kind)
  assert.equal(plan.added, false)
  assert.deepEqual(plan.sourceRefs, ["detail"])

  source.definition_v2.measure.factors.push({code: "detail"})
  assert.throws(() => governance.factorScorePlan(source.definition_v2, source.kind), /duplicated/)
  source.definition_v2.measure.factors[2].code = " unknown "
  assert.throws(() => governance.factorScorePlan(source.definition_v2, source.kind), /normalized/)
})

test("factor model rejects multiple factor_scores sections", () => {
  const source = snapshot()
  source.definition_v2.report_map.sections.push({code: "duplicate", kind: "factor_scores"})
  assert.throws(() => governance.factorScorePlan(source.definition_v2, source.kind), /only one/)
})

test("typology requires one known explicit template id", () => {
  assert.equal(governance.resolveTemplateID(snapshot("typology", "mbti")), "mbti")
  assert.throws(() => governance.resolveTemplateID(snapshot("typology", "")), /one registered template_id/)
  assert.throws(() => governance.resolveTemplateID(snapshot("typology", "unknown")), /one registered template_id/)
})

test("target release version is deterministic and idempotent", () => {
  assert.equal(governance.targetReleaseVersion("v3"), "v3-report-202608-v1")
  assert.equal(governance.targetReleaseVersion("v3-report-202608-v1"), "v3-report-202608-v1")
  assert.throws(() => governance.targetReleaseVersion(""), /release_version is required/)
})

test("hasTargetRoute rejects a partial or drifted materialization", () => {
  const source = snapshot()
  assert.equal(governance.hasTargetRoute(source), false)
  source.definition_v2 = governance.materializeDefinition(source.definition_v2, "standard", source.kind)
  assert.equal(governance.hasTargetRoute(source), true)
  source.definition_v2.interpretation_assets.report_spec.sections[0].template_version = "legacy-v1"
  assert.equal(governance.hasTargetRoute(source), false)
})

test("write operations require exact confirmations and selection is stable", () => {
  assert.throws(() => governance.readConfig({
    MODEL_TEMPLATE_ROUTE_OPERATION: "apply",
    MODEL_TEMPLATE_ROUTE_MANIFEST_PATH: "/tmp/manifest.json"
  }), /activate-model-template-route-2026-08-v1/)
  const config = governance.readConfig({
    MODEL_TEMPLATE_ROUTE_OPERATION: "apply",
    MODEL_TEMPLATE_ROUTE_MANIFEST_PATH: "/tmp/manifest.json",
    MODEL_TEMPLATE_ROUTE_CONFIRM: governance.applyConfirmation,
    MODEL_TEMPLATE_ROUTE_AFTER_ID: "000000000000000000000001",
    MODEL_TEMPLATE_ROUTE_MAX_RECORDS: "1"
  })
  const selected = governance.selectedRecords({records: [
    {source_id: "000000000000000000000001"},
    {source_id: "000000000000000000000002"},
    {source_id: "000000000000000000000003"}
  ]}, config)
  assert.deepEqual(selected.map(record => record.source_id), ["000000000000000000000002"])

  const defaultCursor = governance.readConfig({
    MODEL_TEMPLATE_ROUTE_OPERATION: "audit",
    MODEL_TEMPLATE_ROUTE_MANIFEST_PATH: "/tmp/manifest.json",
    MODEL_TEMPLATE_ROUTE_AFTER_ID: "0"
  })
  assert.equal(defaultCursor.afterID, "")
})

test("governance manifest fingerprint rejects tampering", () => {
  const records = [{
    source_id: "000000000000000000000001",
    clone_id: "000000000000000000000002",
    code: "MODEL",
    template_id: "standard",
    source_release_version: "v3",
    target_release_version: "v3-report-202608-v1",
    governed_at: "2026-08-06T00:00:00.000Z",
    source_content_hash: "a".repeat(64),
    source_definition_hash: "b".repeat(64),
    target_definition_hash: "c".repeat(64),
    factor_score_section_added: false,
    factor_source_refs: ["total", "detail"]
  }]
  const manifest = {
    schema_version: governance.governanceSchemaVersion,
    target_template_version: governance.targetTemplateVersion,
    records,
    records_fingerprint: governance.sha256JSON(records)
  }
  governance.validateGovernanceManifest(manifest)
  records[0].template_id = "mbti"
  assert.throws(() => governance.validateGovernanceManifest(manifest), /fingerprint mismatch/)
})

test("governed clone persists the canonical target definition hash", () => {
  global.ObjectId = {createFromHexString: value => value}
  try {
    const record = {
      clone_id: "000000000000000000000002",
      source_release_version: "v3",
      target_release_version: "v3-report-202608-v1",
      template_id: "standard",
      governed_at: "2026-08-06T00:00:00.000Z",
      source_definition_hash: "b".repeat(64),
      target_definition_hash: "c".repeat(64)
    }
    const clone = governance.desiredClone({
      kind: "scale",
      definition_v2: snapshot().definition_v2,
      source: {definition_content_hash: "b".repeat(64)}
    }, record, "active")
    assert.equal(clone.source.definition_content_hash, "c".repeat(64))
    assert.equal(clone.source.definition_hash_schema, "definition-v2/v1")
    assert.equal(clone.source.interpretation_template_route_governance.source_definition_hash, "b".repeat(64))
  } finally {
    delete global.ObjectId
  }
})
