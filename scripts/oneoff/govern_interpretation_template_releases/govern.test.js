"use strict"

const test = require("node:test")
const assert = require("node:assert/strict")

const governance = require("./govern.js")

test("expected releases match the Go manifest fingerprints", () => {
  const expected = {
    standard: "c5d758a0901ed1e0c77aec5aa6606dd47b12a98e914e619fb41f1271f571fa76",
    mbti: "38976e0b0c2a6d9b4ddb5250a9411011c294bc497e17f171ebe87db8a66349fb",
    sbti: "0e407ca505e837054ff273d2dabd1fc7d1d31a05593a73b0d6c87510988a8706",
    bigfive: "9b98c564a3d71b836f7099399ff0139132259696499a166de09ef395fd2c8cba",
    enneagram: "86bf584f4ea271b4f53bf7c0c237febf714215035074ae4da58543552294dbcd"
  }
  governance.validateExpectedReleases(governance.expectedReleases)
  for (const release of governance.expectedReleases) {
    assert.equal(release.manifest_fingerprint, expected[release.template_id])
  }
})

test("classifyRelease only accepts exact published release metadata", () => {
  const expected = governance.expectedReleases[0]
  const at = new Date("2026-08-06T00:00:00.000Z")
  const base = {
    status: "published", builder_identity: "factor-scoring",
    created_at: at, updated_at: at, published_at: at, published_by: "user:1"
  }
  assert.equal(governance.classifyRelease(base, expected), "update")
  assert.equal(governance.classifyRelease({
    ...base,
    report_type: "standard",
    manifest: expected.manifest,
    manifest_fingerprint: expected.manifest_fingerprint
  }, expected), "noop")
  assert.equal(governance.classifyRelease({...base, status: "draft"}, expected), "blocked")
  assert.equal(governance.classifyRelease({...base, published_by: ""}, expected), "blocked")
  assert.equal(governance.classifyRelease({...base, manifest: {}}, expected), "blocked")
  assert.equal(governance.classifyRelease(null, expected), "blocked")
  assert.equal(governance.classifyRelease(null, governance.expectedReleases[4]), "insert")
})

test("write operations require exact confirmations", () => {
  assert.throws(() => governance.readConfig({
    TEMPLATE_RELEASE_OPERATION: "apply",
    TEMPLATE_RELEASE_MANIFEST_PATH: "/tmp/manifest.json"
  }), /materialize-interpretation-template-manifest-v1/)
  assert.throws(() => governance.readConfig({
    TEMPLATE_RELEASE_OPERATION: "rollback",
    TEMPLATE_RELEASE_MANIFEST_PATH: "/tmp/manifest.json"
  }), /rollback-interpretation-template-manifest-v1/)
  assert.equal(governance.readConfig({
    TEMPLATE_RELEASE_OPERATION: "apply",
    TEMPLATE_RELEASE_MANIFEST_PATH: "/tmp/manifest.json",
    TEMPLATE_RELEASE_CONFIRM: governance.applyConfirmation
  }).operation, "apply")
})

test("governance manifest fingerprint rejects tampering", () => {
  const records = governance.expectedReleases.map((release, index) => ({
    template_id: release.template_id,
    action: index === 4 ? "insert" : "update",
    mongo_id: String(index + 1).padStart(24, "0"),
    domain_id: String(index + 1),
    protected_hash: index === 4 ? undefined : "a".repeat(64),
    created_at: index === 4 ? "2026-08-06T00:00:00.000Z" : undefined,
    expected_manifest_fingerprint: release.manifest_fingerprint
  }))
  const manifest = {
    schema_version: governance.governanceSchemaVersion,
    records,
    records_fingerprint: governance.sha256JSON(records)
  }
  governance.validateGovernanceManifest(manifest)
  manifest.records[0].action = "noop"
  assert.throws(() => governance.validateGovernanceManifest(manifest), /fingerprint mismatch/)
})
