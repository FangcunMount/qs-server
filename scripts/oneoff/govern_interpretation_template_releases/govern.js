// Manifest-driven governance for Interpretation report-template release metadata.
// Run with mongosh. Requiring this file from Node only exports pure helpers.
"use strict"

const crypto = require("crypto")

const governanceSchemaVersion = "interpretation-template-release-governance/v1"
const releaseManifestSchemaVersion = "interpretation-report-template-manifest/v1"
const legacyTemplateVersion = "legacy-v1"
const currentTemplateVersion = "2026-08-v1"
const templateVersion = process.env.TEMPLATE_RELEASE_TARGET_VERSION || legacyTemplateVersion
if (![legacyTemplateVersion, currentTemplateVersion].includes(templateVersion)) {
  throw new Error(`unsupported TEMPLATE_RELEASE_TARGET_VERSION: ${templateVersion}`)
}
const reportType = "standard"
const applyConfirmation = templateVersion === legacyTemplateVersion
  ? "materialize-interpretation-template-manifest-v1"
  : "publish-interpretation-template-2026-08-v1"
const rollbackConfirmation = templateVersion === legacyTemplateVersion
  ? "rollback-interpretation-template-manifest-v1"
  : "rollback-interpretation-template-2026-08-v1"

function releaseManifest(templateID, routes) {
  return {
    schema_version: releaseManifestSchemaVersion,
    template_id: templateID,
    template_version: templateVersion,
    report_type: reportType,
    routes: [...routes].sort((left, right) => left.decision_kind.localeCompare(right.decision_kind))
  }
}

function route(decisionKind, builderIdentity, adapterKey) {
  const result = {
    decision_kind: decisionKind,
    builder_identity: builderIdentity,
    content_schema_version: "report-content/v1"
  }
  if (adapterKey !== "") {
    result.adapter_key = adapterKey
  }
  return result
}

const expectedReleases = [
  {
    template_id: "standard", builder_identity: "factor-scoring", adapter_key: "",
    manifest: releaseManifest("standard", [
      route("score_range", "factor-scoring", ""),
      route("norm_lookup", "norm-profile", ""),
      route("ability_level", "task-performance", "")
    ])
  },
  {
    template_id: "mbti", builder_identity: "typology", adapter_key: "personality_type",
    manifest: releaseManifest("mbti", [
      route("pole_composition", "typology", "personality_type"),
      route("nearest_pattern", "typology", "personality_type"),
      route("dominant_factor", "typology", "personality_type")
    ])
  },
  {
    template_id: "sbti", builder_identity: "typology", adapter_key: "personality_type",
    manifest: releaseManifest("sbti", [
      route("pole_composition", "typology", "personality_type"),
      route("nearest_pattern", "typology", "personality_type"),
      route("dominant_factor", "typology", "personality_type")
    ])
  },
  {
    template_id: "bigfive", builder_identity: "typology", adapter_key: "trait_profile",
    manifest: releaseManifest("bigfive", [route("trait_profile", "typology", "trait_profile")])
  },
  {
    template_id: "enneagram", builder_identity: "typology", adapter_key: "trait_profile",
    manifest: releaseManifest("enneagram", [route("trait_profile", "typology", "trait_profile")])
  }
].map(release => ({...release, manifest_fingerprint: releaseManifestFingerprint(release.manifest)}))

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

function sha256JSON(value, ejson) {
  return crypto.createHash("sha256").update(canonicalJSONString(value, ejson)).digest("hex")
}

// Go's ReleaseManifest fingerprint uses the stable exported field order.
function releaseManifestFingerprint(manifest) {
  return crypto.createHash("sha256").update(JSON.stringify(manifest)).digest("hex")
}

function valuesEqual(left, right, ejson) {
  return canonicalJSONString(left, ejson) === canonicalJSONString(right, ejson)
}

function releaseProtectedHash(document, ejson) {
  const protectedDocument = {...document}
  delete protectedDocument.report_type
  delete protectedDocument.manifest
  delete protectedDocument.manifest_fingerprint
  return sha256JSON(protectedDocument, ejson)
}

function readConfig(env) {
  const operation = env.TEMPLATE_RELEASE_OPERATION || "audit"
  if (!["audit", "apply", "verify", "rollback"].includes(operation)) {
    throw new Error("TEMPLATE_RELEASE_OPERATION must be audit, apply, verify or rollback")
  }
  const manifestPath = env.TEMPLATE_RELEASE_MANIFEST_PATH || ""
  if (manifestPath === "") {
    throw new Error("TEMPLATE_RELEASE_MANIFEST_PATH is required")
  }
  if (operation === "apply" && env.TEMPLATE_RELEASE_CONFIRM !== applyConfirmation) {
    throw new Error(`apply requires TEMPLATE_RELEASE_CONFIRM=${applyConfirmation}`)
  }
  if (operation === "rollback" && env.TEMPLATE_RELEASE_CONFIRM !== rollbackConfirmation) {
    throw new Error(`rollback requires TEMPLATE_RELEASE_CONFIRM=${rollbackConfirmation}`)
  }
  return {operation, manifestPath}
}

function validateExpectedReleases(releases) {
  if (!Array.isArray(releases) || releases.length !== 5) {
    throw new Error("expected release catalog must contain five entries")
  }
  const ids = new Set()
  for (const release of releases) {
    if (ids.has(release.template_id)) {
      throw new Error(`duplicate expected template release: ${release.template_id}`)
    }
    ids.add(release.template_id)
    if (release.manifest.template_id !== release.template_id || release.manifest.template_version !== templateVersion) {
      throw new Error(`release manifest identity mismatch: ${release.template_id}`)
    }
    if (releaseManifestFingerprint(release.manifest) !== release.manifest_fingerprint) {
      throw new Error(`release manifest fingerprint mismatch: ${release.template_id}`)
    }
  }
  return releases
}

function validateGovernanceManifest(manifest) {
  if (manifest == null || manifest.schema_version !== governanceSchemaVersion ||
      manifest.template_version !== templateVersion) {
    throw new Error("unsupported template release governance manifest")
  }
  if (!Array.isArray(manifest.records) || manifest.records.length !== expectedReleases.length) {
    throw new Error("template release governance records are incomplete")
  }
  if (manifest.records_fingerprint !== sha256JSON(manifest.records)) {
    throw new Error("template release governance fingerprint mismatch")
  }
  const ids = new Set()
  for (const record of manifest.records) {
    if (!expectedReleases.some(release => release.template_id === record.template_id)) {
      throw new Error(`unexpected governed template release: ${record.template_id}`)
    }
    if (ids.has(record.template_id)) {
      throw new Error(`duplicate governed template release: ${record.template_id}`)
    }
    ids.add(record.template_id)
    if (!["update", "insert", "noop"].includes(record.action)) {
      throw new Error(`invalid governance action for ${record.template_id}`)
    }
    if (!/^[a-f0-9]{64}$/.test(record.expected_manifest_fingerprint || "")) {
      throw new Error(`invalid expected manifest fingerprint for ${record.template_id}`)
    }
    if (record.action === "insert") {
      if (!/^[a-f0-9]{24}$/.test(record.mongo_id || "") || !/^[0-9]+$/.test(record.domain_id || "") || !record.created_at) {
        throw new Error(`invalid insert identity for ${record.template_id}`)
      }
    } else if (!/^[a-f0-9]{24}$/.test(record.mongo_id || "") || !/^[0-9]+$/.test(record.domain_id || "") || !/^[a-f0-9]{64}$/.test(record.protected_hash || "")) {
      throw new Error(`invalid existing release identity for ${record.template_id}`)
    }
  }
  return manifest
}

function classifyRelease(document, expected, ejson) {
  if (document == null) {
    return templateVersion === currentTemplateVersion || expected.template_id === "enneagram" ? "insert" : "blocked"
  }
  if (document.status !== "published" || document.builder_identity !== expected.builder_identity ||
    (document.adapter_key || "") !== expected.adapter_key) {
    return "blocked"
  }
  if (!(document.created_at instanceof Date) || !(document.updated_at instanceof Date) ||
    !(document.published_at instanceof Date) || typeof document.published_by !== "string" ||
    document.published_by.trim() === "" || document.updated_at < document.created_at ||
    document.published_at < document.created_at || document.disabled_at != null ||
    (document.disabled_by || "") !== "") {
    return "blocked"
  }
  const fields = [document.report_type, document.manifest, document.manifest_fingerprint]
  const missing = fields.every(value => value == null)
  if (missing) {
    return "update"
  }
  if (document.report_type === reportType &&
    valuesEqual(document.manifest, expected.manifest, ejson) &&
    document.manifest_fingerprint === expected.manifest_fingerprint) {
    return "noop"
  }
  return "blocked"
}

function allocateDomainID(collection) {
  let candidate = BigInt(Date.now()) * 1000000n
  while (collection.countDocuments({domain_id: Long.fromString(candidate.toString())}) !== 0) {
    candidate += 1n
  }
  return candidate.toString()
}

function audit(collection, config, fs) {
  validateExpectedReleases(expectedReleases)
  const records = []
  const blocked = []
  const generatedAt = new Date().toISOString()
  for (const expected of expectedReleases) {
    const document = collection.findOne({template_id: expected.template_id, template_version: templateVersion})
    const action = classifyRelease(document, expected, EJSON)
    if (action === "blocked") {
      blocked.push(expected.template_id)
      continue
    }
    if (action === "insert") {
      records.push({
        template_id: expected.template_id, action,
        mongo_id: new ObjectId().toHexString(), domain_id: allocateDomainID(collection), created_at: generatedAt,
        expected_manifest_fingerprint: expected.manifest_fingerprint
      })
      continue
    }
    records.push({
      template_id: expected.template_id, action,
      mongo_id: document._id.toHexString(), domain_id: document.domain_id.toString(),
      protected_hash: releaseProtectedHash(document, EJSON),
      expected_manifest_fingerprint: expected.manifest_fingerprint
    })
  }
  const unexpected = collection.countDocuments({
    template_version: templateVersion,
    template_id: {$nin: expectedReleases.map(release => release.template_id)}
  })
  if (unexpected !== 0) {
    blocked.push(`unexpected:${unexpected}`)
  }
  if (blocked.length > 0) {
    throw new Error(`template release audit blocked: ${blocked.join(",")}`)
  }
  const manifest = {
    schema_version: governanceSchemaVersion,
    database: db.getName(), collection: collection.getName(), generated_at: generatedAt,
    template_version: templateVersion, records,
    records_fingerprint: sha256JSON(records)
  }
  writeManifest(fs, config.manifestPath, manifest)
  printSummary({operation: "audit", records: records.length, update: records.filter(record => record.action === "update").length, insert: records.filter(record => record.action === "insert").length, noop: records.filter(record => record.action === "noop").length, manifest_path: config.manifestPath, records_fingerprint: manifest.records_fingerprint})
}

function expectedByID(templateID) {
  const expected = expectedReleases.find(release => release.template_id === templateID)
  if (expected == null) {
    throw new Error(`unknown template release: ${templateID}`)
  }
  return expected
}

function currentByRecord(collection, record) {
  return collection.findOne({_id: ObjectId.createFromHexString(record.mongo_id)})
}

function assertProtected(document, record) {
  if (document == null || document.domain_id.toString() !== record.domain_id) {
    throw new Error(`template release identity changed: ${record.template_id}`)
  }
  if (releaseProtectedHash(document, EJSON) !== record.protected_hash) {
    throw new Error(`template release protected hash changed: ${record.template_id}`)
  }
}

function desiredInsertDocument(record, expected) {
  const at = new Date(record.created_at)
  const document = {
    _id: ObjectId.createFromHexString(record.mongo_id),
    domain_id: Long.fromString(record.domain_id),
    template_id: expected.template_id,
    template_version: templateVersion,
    builder_identity: expected.builder_identity,
    status: "published",
    created_at: at,
    updated_at: at,
    published_at: at,
    published_by: "system:template-manifest-governance",
    report_type: reportType,
    manifest: expected.manifest,
    manifest_fingerprint: expected.manifest_fingerprint
  }
  if (expected.adapter_key !== "") {
    document.adapter_key = expected.adapter_key
  }
  return document
}

function apply(collection, governance) {
  let updated = 0
  let inserted = 0
  let alreadyApplied = 0
  for (const record of governance.records) {
    const expected = expectedByID(record.template_id)
    if (record.action === "insert") {
      const desired = desiredInsertDocument(record, expected)
      const current = currentByRecord(collection, record)
      if (current != null) {
        if (!valuesEqual(current, desired, EJSON)) {
          throw new Error(`inserted template release conflicts: ${record.template_id}`)
        }
        alreadyApplied += 1
        continue
      }
      const result = collection.insertOne(desired)
      if (result.insertedId.toHexString() !== record.mongo_id) {
        throw new Error(`template release insert identity mismatch: ${record.template_id}`)
      }
      inserted += 1
      continue
    }
    const document = currentByRecord(collection, record)
    assertProtected(document, record)
    const currentAction = classifyRelease(document, expected, EJSON)
    if (currentAction === "noop") {
      alreadyApplied += 1
      continue
    }
    if (record.action !== "update" || currentAction !== "update") {
      throw new Error(`template release is no longer apply-safe: ${record.template_id}`)
    }
    const result = collection.updateOne({
      _id: document._id, domain_id: document.domain_id,
      template_id: expected.template_id, template_version: templateVersion,
      status: "published", report_type: {$exists: false}, manifest: {$exists: false}, manifest_fingerprint: {$exists: false}
    }, {$set: {report_type: reportType, manifest: expected.manifest, manifest_fingerprint: expected.manifest_fingerprint}})
    if (result.matchedCount !== 1 || result.modifiedCount !== 1) {
      throw new Error(`template release lost apply CAS: ${record.template_id}`)
    }
    updated += 1
  }
  printSummary({operation: "apply", selected: governance.records.length, updated, inserted, already_applied: alreadyApplied})
}

function verify(collection, governance) {
  let verified = 0
  for (const record of governance.records) {
    const expected = expectedByID(record.template_id)
    const document = currentByRecord(collection, record)
    if (document == null || classifyRelease(document, expected, EJSON) !== "noop") {
      throw new Error(`template release verification failed: ${record.template_id}`)
    }
    if (record.action !== "insert") {
      assertProtected(document, record)
    } else if (!valuesEqual(document, desiredInsertDocument(record, expected), EJSON)) {
      throw new Error(`inserted template release changed: ${record.template_id}`)
    }
    verified += 1
  }
  const total = collection.countDocuments({template_version: templateVersion})
  if (total !== expectedReleases.length) {
    throw new Error(`template release set is incomplete: ${total}/${expectedReleases.length}`)
  }
  printSummary({operation: "verify", verified, expected: expectedReleases.length, complete: true})
}

function rollback(collection, governance) {
  let reverted = 0
  let removed = 0
  let alreadyRolledBack = 0
  for (const record of [...governance.records].reverse()) {
    if (record.action === "noop") {
      continue
    }
    const expected = expectedByID(record.template_id)
    const document = currentByRecord(collection, record)
    if (record.action === "insert") {
      if (document == null) {
        alreadyRolledBack += 1
        continue
      }
      if (!valuesEqual(document, desiredInsertDocument(record, expected), EJSON)) {
        throw new Error(`inserted template release changed before rollback: ${record.template_id}`)
      }
      const result = collection.deleteOne({_id: document._id, domain_id: document.domain_id, manifest_fingerprint: expected.manifest_fingerprint})
      if (result.deletedCount !== 1) {
        throw new Error(`template release lost delete CAS: ${record.template_id}`)
      }
      removed += 1
      continue
    }
    if (document == null) {
      throw new Error(`template release is missing before rollback: ${record.template_id}`)
    }
    if (classifyRelease(document, expected, EJSON) === "update") {
      assertProtected(document, record)
      alreadyRolledBack += 1
      continue
    }
    assertProtected(document, record)
    if (classifyRelease(document, expected, EJSON) !== "noop") {
      throw new Error(`template release changed before rollback: ${record.template_id}`)
    }
    const result = collection.updateOne({
      _id: document._id, domain_id: document.domain_id,
      report_type: reportType, manifest: expected.manifest, manifest_fingerprint: expected.manifest_fingerprint
    }, {$unset: {report_type: "", manifest: "", manifest_fingerprint: ""}})
    if (result.matchedCount !== 1 || result.modifiedCount !== 1) {
      throw new Error(`template release lost rollback CAS: ${record.template_id}`)
    }
    const restored = currentByRecord(collection, record)
    if (sha256JSON(restored, EJSON) !== record.protected_hash) {
      throw new Error(`template release rollback hash mismatch: ${record.template_id}`)
    }
    reverted += 1
  }
  printSummary({operation: "rollback", reverted, removed, already_rolled_back: alreadyRolledBack})
}

function loadManifest(fs, path) {
  return validateGovernanceManifest(JSON.parse(fs.readFileSync(path, "utf8")))
}

function writeManifest(fs, path, manifest) {
  const parent = require("path").dirname(path)
  fs.mkdirSync(parent, {recursive: true})
  const temporary = `${path}.partial`
  fs.writeFileSync(temporary, `${JSON.stringify(manifest, null, 2)}\n`, {mode: 0o600})
  fs.renameSync(temporary, path)
}

function printSummary(summary) {
  print("INTERPRETATION_TEMPLATE_RELEASE_GOVERNANCE_RESULT")
  print(EJSON.stringify(summary, null, 2, {relaxed: false}))
}

function main() {
  const fs = require("fs")
  const config = readConfig(process.env)
  const collection = db.getCollection("interpretation_report_templates")
  if (config.operation === "audit") {
    audit(collection, config, fs)
    return
  }
  const governance = loadManifest(fs, config.manifestPath)
  if (governance.database !== db.getName() || governance.collection !== collection.getName()) {
    throw new Error("template release governance target mismatch")
  }
  if (config.operation === "apply") {
    apply(collection, governance)
  } else if (config.operation === "verify") {
    verify(collection, governance)
  } else {
    rollback(collection, governance)
  }
}

if (typeof module !== "undefined") {
  module.exports = {
    governanceSchemaVersion,
    releaseManifestSchemaVersion,
    legacyTemplateVersion,
    currentTemplateVersion,
    templateVersion,
    applyConfirmation,
    rollbackConfirmation,
    expectedReleases,
    canonicalJSONString,
    sha256JSON,
    releaseManifestFingerprint,
    releaseProtectedHash,
    readConfig,
    validateExpectedReleases,
    validateGovernanceManifest,
    classifyRelease
  }
}

if (typeof db !== "undefined" && typeof EJSON !== "undefined" && typeof Long !== "undefined") {
  main()
}
