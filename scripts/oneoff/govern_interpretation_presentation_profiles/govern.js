// Versioned, manifest-driven governance for historical Interpretation presentation profiles.
// Run with mongosh. Requiring this file from Node only exports pure helpers for tests.
"use strict"

const crypto = require("crypto")

const schemaVersion = "interpretation-presentation-profile-governance/v1"
const legacySource = "legacy_artifact_dimensions/v1"
const allowedCollections = new Set(["interpret_report_artifacts"])
const applyConfirmation = "materialize-legacy-artifact-dimensions-v1"
const rollbackConfirmation = "rollback-legacy-artifact-dimensions-v1"

function missingProfileFilter() {
  return {
    deleted_at: null,
    $or: [
      {presentation_profile: {$exists: false}},
      {presentation_profile: null}
    ]
  }
}

function visibleFactorCodes(dimensions) {
  if (!Array.isArray(dimensions) || dimensions.length === 0) {
    return {eligible: false, codes: []}
  }
  const seen = new Set()
  const codes = []
  for (const dimension of dimensions) {
    const code = dimension == null ? "" : dimension.factor_code
    if (typeof code !== "string") {
      throw new Error("dimension.factor_code must be a string")
    }
    if (code !== "" && !seen.has(code)) {
      seen.add(code)
      codes.push(code)
    }
  }
  return {eligible: true, codes}
}

function expectedProfile(codes) {
  return {
    visible_factor_codes: [...codes],
    source: legacySource
  }
}

function isPlainObject(value) {
  if (value == null || typeof value !== "object" || Array.isArray(value)) {
    return false
  }
  const prototype = Object.getPrototypeOf(value)
  return prototype === Object.prototype || prototype === null
}

function sortCanonical(value) {
  if (Array.isArray(value)) {
    return value.map(sortCanonical)
  }
  if (!isPlainObject(value)) {
    return value
  }
  const result = {}
  for (const key of Object.keys(value).sort()) {
    result[key] = sortCanonical(value[key])
  }
  return result
}

function canonicalJSONString(value, ejson) {
  const serialized = ejson == null ? value : ejson.serialize(value, {relaxed: false})
  return JSON.stringify(sortCanonical(serialized))
}

function protectedDocumentHash(document, ejson) {
  const protectedDocument = {...document}
  delete protectedDocument.presentation_profile
  return crypto.createHash("sha256").update(canonicalJSONString(protectedDocument, ejson)).digest("hex")
}

function recordsFingerprint(records) {
  return crypto.createHash("sha256").update(canonicalJSONString(records)).digest("hex")
}

function profilesEqual(actual, expected, ejson) {
  if (actual == null || expected == null) {
    return actual == null && expected == null
  }
  return canonicalJSONString(actual, ejson) === canonicalJSONString(expected, ejson)
}

function parsePositiveInteger(value, name, fallback) {
  if (value == null || value === "") {
    return fallback
  }
  if (!/^[0-9]+$/.test(value)) {
    throw new Error(`${name} must be a non-negative integer`)
  }
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed)) {
    throw new Error(`${name} exceeds the safe integer range`)
  }
  return parsed
}

function readConfig(env) {
  const operation = env.PRESENTATION_OPERATION || "audit"
  if (!["audit", "apply", "verify", "rollback"].includes(operation)) {
    throw new Error("PRESENTATION_OPERATION must be audit, apply, verify or rollback")
  }
  const collection = env.PRESENTATION_COLLECTION || "interpret_report_artifacts"
  if (!allowedCollections.has(collection)) {
    throw new Error(`unsupported collection: ${collection}`)
  }
  const manifestPath = env.PRESENTATION_MANIFEST_PATH || ""
  if (operation !== "audit" && manifestPath === "") {
    throw new Error("PRESENTATION_MANIFEST_PATH is required outside audit")
  }
  const maxRecords = parsePositiveInteger(env.PRESENTATION_MAX_RECORDS, "PRESENTATION_MAX_RECORDS", 0)
  const afterID = env.PRESENTATION_AFTER_ID || "0"
  if (!/^[0-9]+$/.test(afterID)) {
    throw new Error("PRESENTATION_AFTER_ID must be an unsigned decimal string")
  }
  const requireComplete = env.PRESENTATION_REQUIRE_COMPLETE === "true"
  if (operation === "apply" && env.PRESENTATION_CONFIRM !== applyConfirmation) {
    throw new Error(`apply requires PRESENTATION_CONFIRM=${applyConfirmation}`)
  }
  if (operation === "rollback" && env.PRESENTATION_CONFIRM !== rollbackConfirmation) {
    throw new Error(`rollback requires PRESENTATION_CONFIRM=${rollbackConfirmation}`)
  }
  return {operation, collection, manifestPath, maxRecords, afterID, requireComplete}
}

function domainIDString(value) {
  if (value == null) {
    throw new Error("domain_id is required")
  }
  if (typeof value === "string") {
    return value
  }
  if (typeof value.toString === "function") {
    const result = value.toString()
    if (/^[0-9]+$/.test(result)) {
      return result
    }
  }
  throw new Error("domain_id must be an unsigned integer")
}

function buildRecord(document, ejson) {
  const derived = visibleFactorCodes(document.dimensions)
  if (!derived.eligible) {
    return null
  }
  return {
    domain_id: domainIDString(document.domain_id),
    protected_hash: protectedDocumentHash(document, ejson),
    visible_factor_codes: derived.codes
  }
}

function validateManifest(manifest) {
  if (manifest == null || manifest.schema_version !== schemaVersion) {
    throw new Error("unsupported presentation governance manifest")
  }
  if (!allowedCollections.has(manifest.collection)) {
    throw new Error("manifest collection is invalid")
  }
  if (!Array.isArray(manifest.records)) {
    throw new Error("manifest records are required")
  }
  let previous = "0"
  const ids = new Set()
  for (const record of manifest.records) {
    if (record == null || !/^[0-9]+$/.test(record.domain_id || "")) {
      throw new Error("manifest record domain_id is invalid")
    }
    if (compareUnsignedDecimal(record.domain_id, previous) <= 0 || ids.has(record.domain_id)) {
      throw new Error("manifest record IDs must be unique and ascending")
    }
    if (!/^[a-f0-9]{64}$/.test(record.protected_hash || "")) {
      throw new Error(`manifest protected hash is invalid for ${record.domain_id}`)
    }
    if (!Array.isArray(record.visible_factor_codes) || record.visible_factor_codes.some(code => typeof code !== "string" || code === "")) {
      throw new Error(`manifest factor codes are invalid for ${record.domain_id}`)
    }
    previous = record.domain_id
    ids.add(record.domain_id)
  }
  if (manifest.records_fingerprint !== recordsFingerprint(manifest.records)) {
    throw new Error("manifest records fingerprint mismatch")
  }
  return manifest
}

function compareUnsignedDecimal(left, right) {
  const normalizedLeft = left.replace(/^0+(?=\d)/, "")
  const normalizedRight = right.replace(/^0+(?=\d)/, "")
  if (normalizedLeft.length !== normalizedRight.length) {
    return normalizedLeft.length < normalizedRight.length ? -1 : 1
  }
  return normalizedLeft === normalizedRight ? 0 : (normalizedLeft < normalizedRight ? -1 : 1)
}

function selectedRecords(manifest, config) {
  const selected = manifest.records.filter(record => compareUnsignedDecimal(record.domain_id, config.afterID) > 0)
  return config.maxRecords > 0 ? selected.slice(0, config.maxRecords) : selected
}

function printSummary(summary) {
  print("INTERPRETATION_PRESENTATION_GOVERNANCE_RESULT")
  print(EJSON.stringify(summary, null, 2, {relaxed: false}))
}

function loadManifest(fs, path) {
  return validateManifest(JSON.parse(fs.readFileSync(path, "utf8")))
}

function writeManifest(fs, path, manifest) {
  const parent = require("path").dirname(path)
  fs.mkdirSync(parent, {recursive: true})
  const temporary = `${path}.partial`
  fs.writeFileSync(temporary, `${JSON.stringify(manifest, null, 2)}\n`, {mode: 0o600})
  fs.renameSync(temporary, path)
}

function audit(collection, config, fs) {
  const records = []
  let missingTotal = 0
  let noDimensions = 0
  let blocked = 0
  const cursor = collection.find(missingProfileFilter()).sort({domain_id: 1})
  while (cursor.hasNext()) {
    const document = cursor.next()
    missingTotal += 1
    try {
      const record = buildRecord(document, EJSON)
      if (record == null) {
        noDimensions += 1
        continue
      }
      records.push(record)
    } catch (error) {
      blocked += 1
      print(`BLOCKED domain_id=${domainIDString(document.domain_id)} reason=${error.message}`)
    }
  }
  if (blocked > 0) {
    throw new Error(`audit found ${blocked} blocked records`)
  }
  const manifest = {
    schema_version: schemaVersion,
    database: db.getName(),
    collection: config.collection,
    generated_at: new Date().toISOString(),
    source: legacySource,
    initial_missing_total: missingTotal,
    no_dimensions: noDimensions,
    records,
    records_fingerprint: recordsFingerprint(records)
  }
  if (config.manifestPath !== "") {
    writeManifest(fs, config.manifestPath, manifest)
  }
  printSummary({
    operation: "audit",
    collection: config.collection,
    missing_total: missingTotal,
    eligible: records.length,
    no_dimensions: noDimensions,
    blocked,
    manifest_path: config.manifestPath,
    records_fingerprint: manifest.records_fingerprint
  })
}

function currentDocument(collection, record) {
  return collection.findOne({domain_id: Long.fromString(record.domain_id), deleted_at: null})
}

function assertProtectedDocument(document, record) {
  if (document == null) {
    throw new Error(`artifact ${record.domain_id} is missing or inactive`)
  }
  const actualHash = protectedDocumentHash(document, EJSON)
  if (actualHash !== record.protected_hash) {
    throw new Error(`artifact ${record.domain_id} protected hash changed`)
  }
}

function apply(collection, manifest, config) {
  const records = selectedRecords(manifest, config)
  let updated = 0
  let alreadyApplied = 0
  let lastID = config.afterID
  for (const record of records) {
    const document = currentDocument(collection, record)
    assertProtectedDocument(document, record)
    const expected = expectedProfile(record.visible_factor_codes)
    if (document.presentation_profile != null) {
      if (!profilesEqual(document.presentation_profile, expected, EJSON)) {
        throw new Error(`artifact ${record.domain_id} already has a conflicting presentation profile`)
      }
      alreadyApplied += 1
      lastID = record.domain_id
      continue
    }
    const result = collection.updateOne({
      domain_id: Long.fromString(record.domain_id),
      deleted_at: null,
      $or: [
        {presentation_profile: {$exists: false}},
        {presentation_profile: null}
      ],
      dimensions: document.dimensions
    }, {$set: {presentation_profile: expected}})
    if (result.matchedCount !== 1 || result.modifiedCount !== 1) {
      throw new Error(`artifact ${record.domain_id} lost the apply CAS`)
    }
    updated += 1
    lastID = record.domain_id
  }
  printSummary({operation: "apply", collection: config.collection, selected: records.length, updated, already_applied: alreadyApplied, last_domain_id: lastID})
}

function verify(collection, manifest, config) {
  const records = selectedRecords(manifest, config)
  let verified = 0
  let lastID = config.afterID
  for (const record of records) {
    const document = currentDocument(collection, record)
    assertProtectedDocument(document, record)
    if (!profilesEqual(document.presentation_profile, expectedProfile(record.visible_factor_codes), EJSON)) {
      throw new Error(`artifact ${record.domain_id} presentation profile does not match the manifest`)
    }
    verified += 1
    lastID = record.domain_id
  }
  const remaining = collection.countDocuments(missingProfileFilter())
  if (config.requireComplete && (records.length !== manifest.records.length || remaining !== manifest.no_dimensions)) {
    throw new Error(`complete verification failed: selected=${records.length}/${manifest.records.length} remaining=${remaining}/${manifest.no_dimensions}`)
  }
  printSummary({operation: "verify", collection: config.collection, selected: records.length, verified, remaining_missing: remaining, last_domain_id: lastID, complete: config.requireComplete})
}

function rollback(collection, manifest, config) {
  const records = selectedRecords(manifest, config)
  let rolledBack = 0
  let alreadyMissing = 0
  let lastID = config.afterID
  for (const record of records) {
    const document = currentDocument(collection, record)
    assertProtectedDocument(document, record)
    const expected = expectedProfile(record.visible_factor_codes)
    if (document.presentation_profile == null) {
      alreadyMissing += 1
      lastID = record.domain_id
      continue
    }
    if (!profilesEqual(document.presentation_profile, expected, EJSON)) {
      throw new Error(`artifact ${record.domain_id} has a conflicting presentation profile`)
    }
    const result = collection.updateOne({
      domain_id: Long.fromString(record.domain_id),
      deleted_at: null,
      presentation_profile: expected
    }, {$unset: {presentation_profile: ""}})
    if (result.matchedCount !== 1 || result.modifiedCount !== 1) {
      throw new Error(`artifact ${record.domain_id} lost the rollback CAS`)
    }
    rolledBack += 1
    lastID = record.domain_id
  }
  printSummary({operation: "rollback", collection: config.collection, selected: records.length, rolled_back: rolledBack, already_missing: alreadyMissing, last_domain_id: lastID})
}

function main() {
  const fs = require("fs")
  const config = readConfig(process.env)
  const collection = db.getCollection(config.collection)
  if (config.operation === "audit") {
    audit(collection, config, fs)
    return
  }
  const manifest = loadManifest(fs, config.manifestPath)
  if (manifest.database !== db.getName() || manifest.collection !== config.collection) {
    throw new Error("manifest database or collection does not match the target")
  }
  if (config.operation === "apply") {
    apply(collection, manifest, config)
  } else if (config.operation === "verify") {
    verify(collection, manifest, config)
  } else {
    rollback(collection, manifest, config)
  }
}

if (typeof module !== "undefined") {
  module.exports = {
    applyConfirmation,
    rollbackConfirmation,
    schemaVersion,
    legacySource,
    visibleFactorCodes,
    expectedProfile,
    protectedDocumentHash,
    profilesEqual,
    recordsFingerprint,
    readConfig,
    buildRecord,
    validateManifest,
    compareUnsignedDecimal,
    selectedRecords
  }
}

if (typeof db !== "undefined" && typeof EJSON !== "undefined" && typeof Long !== "undefined") {
  main()
}
