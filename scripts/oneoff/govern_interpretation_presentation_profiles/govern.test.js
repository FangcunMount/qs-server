"use strict"

const assert = require("node:assert/strict")
const test = require("node:test")

const governance = require("./govern.js")

test("visibleFactorCodes reproduces legacy read ordering and deduplication", () => {
  assert.deepEqual(governance.visibleFactorCodes([
    {factor_code: "B"},
    {factor_code: ""},
    {factor_code: "A"},
    {factor_code: "B"}
  ]), {eligible: true, codes: ["B", "A"]})
  assert.deepEqual(governance.visibleFactorCodes([]), {eligible: false, codes: []})
  assert.throws(() => governance.visibleFactorCodes([{factor_code: 7}]), /must be a string/)
})

test("protected hash ignores only presentation_profile", () => {
  const before = {domain_id: "7", dimensions: [{factor_code: "A"}], conclusion: "stable"}
  const after = {...before, presentation_profile: governance.expectedProfile(["A"])}
  assert.equal(governance.protectedDocumentHash(before), governance.protectedDocumentHash(after))
  assert.notEqual(governance.protectedDocumentHash(before), governance.protectedDocumentHash({...before, conclusion: "changed"}))
})

test("config is dry-run by default and writes require exact confirmation", () => {
  assert.deepEqual(governance.readConfig({}), {
    operation: "audit",
    collection: "interpret_report_artifacts",
    manifestPath: "",
    maxRecords: 0,
    afterID: "0",
    requireComplete: false
  })
  assert.throws(() => governance.readConfig({PRESENTATION_OPERATION: "apply", PRESENTATION_MANIFEST_PATH: "/manifest.json"}), /apply requires/)
  assert.doesNotThrow(() => governance.readConfig({
    PRESENTATION_OPERATION: "apply",
    PRESENTATION_MANIFEST_PATH: "/manifest.json",
    PRESENTATION_CONFIRM: governance.applyConfirmation
  }))
  assert.throws(() => governance.readConfig({PRESENTATION_OPERATION: "rollback", PRESENTATION_MANIFEST_PATH: "/manifest.json"}), /rollback requires/)
})

test("manifest validation protects ordering, hashes and fingerprint", () => {
  const records = [
    {domain_id: "9", protected_hash: "a".repeat(64), visible_factor_codes: ["A"]},
    {domain_id: "10", protected_hash: "b".repeat(64), visible_factor_codes: ["B"]}
  ]
  const manifest = {
    schema_version: governance.schemaVersion,
    collection: "interpret_report_artifacts",
    records,
    records_fingerprint: governance.recordsFingerprint(records)
  }
  assert.equal(governance.validateManifest(manifest), manifest)
  assert.deepEqual(governance.selectedRecords(manifest, {afterID: "9", maxRecords: 1}), [records[1]])

  const tampered = JSON.parse(JSON.stringify(manifest))
  tampered.records[1].visible_factor_codes = ["C"]
  assert.throws(() => governance.validateManifest(tampered), /fingerprint mismatch/)
})

test("domain ID comparison does not pass through unsafe JS numbers", () => {
  assert.equal(governance.compareUnsignedDecimal("631012088902332974", "631012088902332973"), 1)
  assert.equal(governance.compareUnsignedDecimal("9", "10"), -1)
  assert.equal(governance.compareUnsignedDecimal("0009", "9"), 0)
})
