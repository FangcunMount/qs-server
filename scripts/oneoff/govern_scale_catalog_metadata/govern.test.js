"use strict"

const assert = require("node:assert/strict")
const test = require("node:test")

const governance = require("./govern.js")

test("medical scale category contract is exact", () => {
  assert.deepEqual(governance.canonicalCategories, ["adhd", "td", "asd", "pressure", "sii", "efn", "emt", "slp"])
})

test("desired metadata uses heads except for the two verified contamination overrides", () => {
  const head = {record_role: "head", deleted_at: null, category: "slp", tags: [" 睡眠 ", "睡眠"]}
  assert.deepEqual(governance.desiredMetadata("ISI7", head), {category: "slp", tags: ["睡眠"], updateHead: false})

  assert.deepEqual(governance.desiredMetadata("fthN56", {
    record_role: "head", deleted_at: null, category: "adhd", tags: ["人格", "MBTI风格"]
  }), {category: "emt", tags: ["情绪", "症状筛查"], updateHead: true})
  assert.deepEqual(governance.desiredMetadata("zOO4eG", {
    record_role: "head", deleted_at: null, category: "adhd", tags: ["人格", "偏好"]
  }), {category: "adhd", tags: ["日常生活困难"], updateHead: true})
})

test("non-canonical head category is rejected", () => {
  assert.throws(() => governance.desiredMetadata("BAD", {
    record_role: "head", deleted_at: null, category: "personality", tags: []
  }), /non-canonical category/)
})

test("protected snapshot hash ignores only governed catalogue and release fields", () => {
  const source = {
    _id: "source", code: "ISI7", release_version: "v1", release_status: "active",
    category: "", tags: [], created_at: "old", updated_at: "old", published_at: "old",
    definition_v2: {schema_version: 3, measure: {factors: [{code: "sleep"}]}},
    source: {definition_content_hash: "stable"}
  }
  const governed = {
    ...source, _id: "clone", release_version: "v1-catalog", release_status: "active",
    category: "slp", tags: ["睡眠"], created_at: "new", updated_at: "new", published_at: "new",
    source: {...source.source, scale_catalog_metadata_governance: {source_release_version: "v1"}}
  }
  assert.equal(governance.protectedSnapshotHash(source), governance.protectedSnapshotHash(governed))
  assert.notEqual(governance.protectedSnapshotHash(source), governance.protectedSnapshotHash({
    ...governed, definition_v2: {schema_version: 3, measure: {factors: [{code: "changed"}]}}
  }))
})

test("write operations require exact confirmation", () => {
  assert.throws(() => governance.readConfig({
    SCALE_CATALOG_OPERATION: "apply", SCALE_CATALOG_MANIFEST_PATH: "/manifest.json"
  }), /apply requires/)
  assert.doesNotThrow(() => governance.readConfig({
    SCALE_CATALOG_OPERATION: "apply",
    SCALE_CATALOG_MANIFEST_PATH: "/manifest.json",
    SCALE_CATALOG_CONFIRM: governance.applyConfirmation,
  }))
  assert.throws(() => governance.readConfig({
    SCALE_CATALOG_OPERATION: "rollback", SCALE_CATALOG_MANIFEST_PATH: "/manifest.json"
  }), /rollback requires/)
})

test("manifest fingerprint and release derivation are immutable", () => {
  const records = [{
    code: "ISI7", source_id: "a".repeat(24), clone_id: "b".repeat(24),
    source_release_version: "1.0.0", target_release_version: "1.0.0-catalog-202608-v1",
    protected_snapshot_hash: "c".repeat(64), head_id: "d".repeat(24), head_revision: "4",
    desired_category: "slp", desired_tags: ["睡眠"], source_tags: [],
    head_source_tags: ["睡眠"], update_head: false, governed_at: "2026-08-07T00:00:00.000Z"
  }]
  const manifest = {
    schema_version: governance.schemaVersion,
    collection: "assessment_models",
    canonical_categories: governance.canonicalCategories,
    target_count: 1,
    records,
    records_fingerprint: governance.recordFingerprint(records),
  }
  assert.equal(governance.validateManifest(manifest), manifest)
  const tampered = JSON.parse(JSON.stringify(manifest))
  tampered.records[0].desired_category = "emt"
  assert.throws(() => governance.validateManifest(tampered), /fingerprint mismatch/)
  assert.equal(governance.targetReleaseVersion("v1-catalog-202608-v1"), "v1-catalog-202608-v1")
  assert.equal(governance.nextRevision("9223372036854775800", 1), "9223372036854775801")
})
