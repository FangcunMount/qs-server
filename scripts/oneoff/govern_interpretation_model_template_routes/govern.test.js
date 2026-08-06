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
      report_map: {sections: [{code: "result", kind: "factor_scores", template_id: templateID}]},
      interpretation_assets: {outcomes: [{outcome_code: "normal"}], report_spec: {sections: [{code: "stale"}]}}
    }
  }
}

test("materializeDefinition freezes standard release in both canonical layers", () => {
  const source = snapshot()
  const governed = governance.materializeDefinition(source.definition_v2, governance.resolveTemplateID(source))
  assert.equal(governed.report_map.sections[0].template_id, "standard")
  assert.equal(governed.report_map.sections[0].template_version, "2026-08-v1")
  assert.deepEqual(governed.interpretation_assets.report_spec.sections, governed.report_map.sections)
  assert.equal(source.definition_v2.report_map.sections[0].template_version, undefined)
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
  source.definition_v2 = governance.materializeDefinition(source.definition_v2, "standard")
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
    source_content_hash: "a".repeat(64)
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
