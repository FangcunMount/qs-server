// Manifest-driven governance for active medical-scale catalogue metadata.
// Run with mongosh. The tool never mutates an immutable published snapshot in place.
"use strict"

const crypto = require("crypto")

const schemaVersion = "scale-catalog-metadata-governance/v1"
const releaseVersionSuffix = "-catalog-202608-v1"
const applyConfirmation = "activate-scale-catalog-metadata-2026-08-v1"
const rollbackConfirmation = "rollback-scale-catalog-metadata-2026-08-v1"
const canonicalCategories = Object.freeze(["adhd", "td", "asd", "pressure", "sii", "efn", "emt", "slp"])
const canonicalCategorySet = new Set(canonicalCategories)
const activeScaleFilter = {
  record_role: "published_snapshot",
  release_status: "active",
  status: "published",
  deleted_at: null,
  kind: "scale"
}

// These two heads contain independently verified MBTI metadata contamination.
// Keep the override intentionally narrow; every other desired value comes from its head.
const metadataOverrides = Object.freeze({
  fthN56: Object.freeze({category: "emt", tags: Object.freeze(["情绪", "症状筛查"])}),
  zOO4eG: Object.freeze({category: "adhd", tags: Object.freeze(["日常生活困难"])}),
})

function isPlainObject(value) {
  if (value == null || typeof value !== "object" || Array.isArray(value)) return false
  const prototype = Object.getPrototypeOf(value)
  return prototype === Object.prototype || prototype === null
}

function isObjectRecord(value) {
  return value != null && typeof value === "object" && !Array.isArray(value) && !(value instanceof Date)
}

function cloneValue(value) {
  if (Array.isArray(value)) return value.map(cloneValue)
  if (value instanceof Date) return new Date(value.getTime())
  if (!isPlainObject(value)) return value
  const result = {}
  for (const [key, item] of Object.entries(value)) result[key] = cloneValue(item)
  return result
}

function deepClone(value, ejson) {
  if (ejson != null) return ejson.deserialize(ejson.serialize(value, {relaxed: false}))
  return cloneValue(value)
}

function sortCanonical(value) {
  if (Array.isArray(value)) return value.map(sortCanonical)
  if (!isPlainObject(value)) return value
  const result = {}
  for (const key of Object.keys(value).sort()) result[key] = sortCanonical(value[key])
  return result
}

function canonicalJSONString(value, ejson) {
  const serialized = ejson == null ? value : ejson.serialize(value, {relaxed: false})
  return JSON.stringify(sortCanonical(serialized))
}

function sha256JSON(value, ejson) {
  return crypto.createHash("sha256").update(canonicalJSONString(value, ejson)).digest("hex")
}

function valuesEqual(left, right, ejson) {
  return canonicalJSONString(left, ejson) === canonicalJSONString(right, ejson)
}

function normalizeTags(value) {
  if (value == null) return []
  if (!Array.isArray(value)) throw new Error("scale tags must be an array")
  const result = []
  const seen = new Set()
  for (const raw of value) {
    if (typeof raw !== "string") throw new Error("scale tag must be a string")
    const tag = raw.trim()
    if (tag === "" || seen.has(tag)) continue
    seen.add(tag)
    result.push(tag)
  }
  return result
}

function desiredMetadata(code, head) {
  if (head == null || head.record_role !== "head" || head.deleted_at != null) {
    throw new Error(`active scale head is unavailable: ${code}`)
  }
  const override = metadataOverrides[code]
  const category = String(override?.category ?? head.category ?? "").trim()
  const tags = normalizeTags(override?.tags ?? head.tags)
  if (!canonicalCategorySet.has(category)) {
    throw new Error(`active scale head has non-canonical category: ${code} category=${category || "<missing>"}`)
  }
  return {category, tags, updateHead: override != null && (
    String(head.category || "").trim() !== category || !valuesEqual(normalizeTags(head.tags), tags)
  )}
}

function targetReleaseVersion(sourceVersion) {
  const normalized = String(sourceVersion || "").trim()
  if (normalized === "") throw new Error("published scale release_version is required")
  return normalized.endsWith(releaseVersionSuffix) ? normalized : normalized + releaseVersionSuffix
}

function protectedSnapshotDocument(document, ejson) {
  const protectedDocument = deepClone(document, ejson)
  for (const field of [
    "_id", "release_version", "release_status", "release_archived_at",
    "created_at", "updated_at", "published_at", "category", "tags"
  ]) delete protectedDocument[field]
  if (isObjectRecord(protectedDocument.source)) {
    delete protectedDocument.source.scale_catalog_metadata_governance
    if (Object.keys(protectedDocument.source).length === 0) delete protectedDocument.source
  }
  return protectedDocument
}

function protectedSnapshotHash(document, ejson) {
  return sha256JSON(protectedSnapshotDocument(document, ejson), ejson)
}

function protectedSnapshotDriftFields(source, clone, ejson) {
  const left = protectedSnapshotDocument(source, ejson)
  const right = protectedSnapshotDocument(clone, ejson)
  return [...new Set([...Object.keys(left), ...Object.keys(right)])]
    .filter(field => !valuesEqual(left[field], right[field], ejson))
    .sort()
}

function readConfig(env) {
  const operation = env.SCALE_CATALOG_OPERATION || "audit"
  if (!["audit", "apply", "verify", "rollback"].includes(operation)) {
    throw new Error("SCALE_CATALOG_OPERATION must be audit, apply, verify or rollback")
  }
  const manifestPath = env.SCALE_CATALOG_MANIFEST_PATH || ""
  if (manifestPath === "") throw new Error("SCALE_CATALOG_MANIFEST_PATH is required")
  const requireComplete = env.SCALE_CATALOG_REQUIRE_COMPLETE === "true"
  if (operation === "apply" && env.SCALE_CATALOG_CONFIRM !== applyConfirmation) {
    throw new Error(`apply requires SCALE_CATALOG_CONFIRM=${applyConfirmation}`)
  }
  if (operation === "rollback" && env.SCALE_CATALOG_CONFIRM !== rollbackConfirmation) {
    throw new Error(`rollback requires SCALE_CATALOG_CONFIRM=${rollbackConfirmation}`)
  }
  return {operation, manifestPath, requireComplete}
}

function recordFingerprint(records) {
  return sha256JSON(records)
}

function validateManifest(manifest) {
  if (manifest?.schema_version !== schemaVersion || manifest?.collection !== "assessment_models") {
    throw new Error("unsupported scale catalogue metadata governance manifest")
  }
  if (!Array.isArray(manifest.canonical_categories) || !valuesEqual(manifest.canonical_categories, canonicalCategories)) {
    throw new Error("scale catalogue metadata canonical category set drifted")
  }
  if (!Array.isArray(manifest.records) || manifest.records_fingerprint !== recordFingerprint(manifest.records)) {
    throw new Error("scale catalogue metadata governance fingerprint mismatch")
  }
  if (manifest.target_count !== manifest.records.length || manifest.records.length === 0) {
    throw new Error("scale catalogue metadata governance manifest must contain its complete non-empty target set")
  }
  const sourceIDs = new Set()
  const cloneIDs = new Set()
  const codes = new Set()
  for (const record of manifest.records) {
    if (!/^[a-f0-9]{24}$/.test(record.source_id || "") ||
        !/^[a-f0-9]{24}$/.test(record.clone_id || "") ||
        sourceIDs.has(record.source_id) || cloneIDs.has(record.clone_id) || codes.has(record.code) ||
        !/^[a-f0-9]{64}$/.test(record.protected_snapshot_hash || "") ||
        !/^[0-9]+$/.test(record.head_revision || "") ||
        record.target_release_version !== targetReleaseVersion(record.source_release_version) ||
        !canonicalCategorySet.has(record.desired_category) ||
        !Array.isArray(record.desired_tags) || !Array.isArray(record.source_tags) ||
        !Array.isArray(record.head_source_tags) || typeof record.update_head !== "boolean" ||
        typeof record.governed_at !== "string") {
      throw new Error(`invalid scale catalogue metadata governance record: ${record.code || "unknown"}`)
    }
    sourceIDs.add(record.source_id)
    cloneIDs.add(record.clone_id)
    codes.add(record.code)
  }
  return manifest
}

function writeManifest(fs, path, manifest) {
  const parent = require("path").dirname(path)
  fs.mkdirSync(parent, {recursive: true})
  const temporary = `${path}.partial`
  fs.writeFileSync(temporary, `${JSON.stringify(manifest, null, 2)}\n`, {mode: 0o600})
  fs.renameSync(temporary, path)
}

function loadManifest(fs, path) {
  return validateManifest(JSON.parse(fs.readFileSync(path, "utf8")))
}

function findHead(collection, code) {
  const heads = collection.find({record_role: "head", code, deleted_at: null}).toArray()
  if (heads.length !== 1) throw new Error(`expected exactly one active scale head: ${code} count=${heads.length}`)
  return heads[0]
}

function validateSourceSnapshot(source) {
  for (const field of ["code", "release_version", "title", "kind", "algorithm", "questionnaire_code", "questionnaire_version"]) {
    if (typeof source?.[field] !== "string" || source[field].trim() === "") {
      throw new Error(`active scale snapshot ${field} is required: ${source?.code || "unknown"}`)
    }
  }
  if (source.record_role !== "published_snapshot" || source.release_status !== "active" ||
      source.status !== "published" || source.deleted_at != null || source.kind !== "scale") {
    throw new Error(`scale snapshot is not active: ${source.code}`)
  }
  if (!(source.created_at instanceof Date) || !(source.updated_at instanceof Date) || !(source.published_at instanceof Date)) {
    throw new Error(`active scale snapshot timestamps are invalid: ${source.code}`)
  }
}

function audit(collection, config, fs) {
  const governedAt = new Date().toISOString()
  const active = collection.find(activeScaleFilter).sort({code: 1}).toArray()
  const records = []
  for (const source of active) {
    validateSourceSnapshot(source)
    const head = findHead(collection, source.code)
    const desired = desiredMetadata(source.code, head)
    const sourceCategory = String(source.category || "").trim()
    const sourceTags = normalizeTags(source.tags)
    if (sourceCategory === desired.category && valuesEqual(sourceTags, desired.tags, EJSON)) continue
    const headRevision = String(head.revision)
    if (!/^[0-9]+$/.test(headRevision)) throw new Error(`scale head revision is invalid: ${source.code}`)
    records.push({
      code: source.code,
      title: source.title,
      source_id: source._id.toHexString(),
      clone_id: new ObjectId().toHexString(),
      source_release_version: source.release_version,
      target_release_version: targetReleaseVersion(source.release_version),
      protected_snapshot_hash: protectedSnapshotHash(source, EJSON),
      source_category: sourceCategory,
      source_tags: sourceTags,
      desired_category: desired.category,
      desired_tags: desired.tags,
      head_id: head._id.toHexString(),
      head_revision: headRevision,
      head_status: head.status,
      head_source_category: String(head.category || "").trim(),
      head_source_tags: normalizeTags(head.tags),
      update_head: desired.updateHead,
      governed_at: governedAt,
    })
  }
  if (records.length === 0) throw new Error("scale catalogue metadata already has no governance targets")
  const manifest = {
    schema_version: schemaVersion,
    database: db.getName(),
    collection: collection.getName(),
    canonical_categories: canonicalCategories,
    active_scale_count: active.length,
    target_count: records.length,
    generated_at: governedAt,
    records,
    records_fingerprint: recordFingerprint(records),
  }
  validateManifest(manifest)
  writeManifest(fs, config.manifestPath, manifest)
  printSummary({
    operation: "audit",
    active_scales: active.length,
    targets: records.length,
    target_codes: records.map(record => record.code),
    manifest: config.manifestPath,
    records_fingerprint: manifest.records_fingerprint,
  })
}

function desiredClone(source, record, status, ejson) {
  const clone = deepClone(source, ejson)
  clone._id = ObjectId.createFromHexString(record.clone_id)
  clone.release_version = record.target_release_version
  clone.release_status = status
  clone.category = record.desired_category
  clone.tags = deepClone(record.desired_tags, ejson)
  const governedAt = new Date(record.governed_at)
  clone.created_at = governedAt
  clone.updated_at = governedAt
  clone.published_at = governedAt
  if (status === "active") delete clone.release_archived_at
  else clone.release_archived_at = governedAt
  clone.source = isObjectRecord(clone.source) ? clone.source : {}
  clone.source.scale_catalog_metadata_governance = {
    schema_version: schemaVersion,
    source_release_version: record.source_release_version,
    governed_at: record.governed_at,
  }
  return ejson == null ? clone : ejson.deserialize(ejson.serialize(clone, {relaxed: false}))
}

function findByID(collection, id) {
  return collection.findOne({_id: ObjectId.createFromHexString(id)})
}

function assertSource(source, record) {
  if (source == null || source.code !== record.code || source.release_version !== record.source_release_version ||
      protectedSnapshotHash(source, EJSON) !== record.protected_snapshot_hash ||
      String(source.category || "").trim() !== record.source_category ||
      !valuesEqual(normalizeTags(source.tags), record.source_tags, EJSON)) {
    throw new Error(`source scale snapshot drifted: ${record.code}`)
  }
}

function assertClone(clone, source, record) {
  const driftFields = clone == null ? ["missing"] : protectedSnapshotDriftFields(source, clone, EJSON)
  if (clone == null || clone.code !== record.code || clone.release_version !== record.target_release_version ||
      protectedSnapshotHash(clone, EJSON) !== record.protected_snapshot_hash || driftFields.length !== 0 ||
      clone.category !== record.desired_category || !valuesEqual(normalizeTags(clone.tags), record.desired_tags, EJSON) ||
      clone.source?.scale_catalog_metadata_governance?.source_release_version !== record.source_release_version) {
    throw new Error(`governed scale snapshot drifted: ${record.code} fields=${driftFields.join(",") || "metadata"}`)
  }
}

function assertCloneIdentity(clone, record) {
  if (clone == null || clone._id.toHexString() !== record.clone_id || clone.code !== record.code ||
      clone.release_version !== record.target_release_version || clone.record_role !== "published_snapshot" ||
      clone.source?.scale_catalog_metadata_governance?.source_release_version !== record.source_release_version) {
    throw new Error(`governed scale snapshot identity drifted: ${record.code}`)
  }
}

function assertHeadIdentity(head, record) {
  if (head == null || head._id.toHexString() !== record.head_id || head.code !== record.code ||
      head.record_role !== "head" || head.deleted_at != null) {
    throw new Error(`scale head identity drifted: ${record.code}`)
  }
}

function headMatches(head, category, tags) {
  return String(head.category || "").trim() === category && valuesEqual(normalizeTags(head.tags), tags, EJSON)
}

function nextRevision(value, increment) {
  return String(BigInt(value) + BigInt(increment))
}

async function updateHeadForApply(collection, record) {
  if (!record.update_head) return "unchanged"
  const head = await findByID(collection, record.head_id)
  assertHeadIdentity(head, record)
  const revision = String(head.revision)
  if (revision === nextRevision(record.head_revision, 1) && headMatches(head, record.desired_category, record.desired_tags)) {
    return "already_applied"
  }
  if (revision !== record.head_revision || !headMatches(head, record.head_source_category, record.head_source_tags) ||
      head.status !== record.head_status) {
    throw new Error(`scale head lost apply CAS: ${record.code}`)
  }
  const result = await collection.updateOne({_id: head._id, revision: head.revision}, {
    $set: {category: record.desired_category, tags: record.desired_tags, updated_at: new Date(record.governed_at)},
    $inc: {revision: 1},
  })
  if (result.matchedCount !== 1 || result.modifiedCount !== 1) throw new Error(`scale head update failed: ${record.code}`)
  return "updated"
}

async function updateHeadForRollback(collection, record) {
  if (!record.update_head) return "unchanged"
  const head = await findByID(collection, record.head_id)
  assertHeadIdentity(head, record)
  const revision = String(head.revision)
  if (revision === nextRevision(record.head_revision, 2) && headMatches(head, record.head_source_category, record.head_source_tags)) {
    return "already_rolled_back"
  }
  if (revision !== nextRevision(record.head_revision, 1) || !headMatches(head, record.desired_category, record.desired_tags) ||
      head.status !== record.head_status) {
    throw new Error(`scale head lost rollback CAS: ${record.code}`)
  }
  const result = await collection.updateOne({_id: head._id, revision: head.revision}, {
    $set: {category: record.head_source_category, tags: record.head_source_tags, updated_at: new Date()},
    $inc: {revision: 1},
  })
  if (result.matchedCount !== 1 || result.modifiedCount !== 1) throw new Error(`scale head rollback failed: ${record.code}`)
  return "restored"
}

async function withGovernanceTransaction(callback) {
  const session = db.getMongo().startSession()
  try {
    return await session.withTransaction(async () => callback(session.getDatabase(db.getName())))
  } finally {
    await session.endSession()
  }
}

async function apply(collection, manifest) {
  let activated = 0
  let alreadyApplied = 0
  let updatedHeads = 0
  await withGovernanceTransaction(async sessionDB => {
    const sessionCollection = sessionDB.getCollection(collection.getName())
    for (const record of manifest.records) {
      const source = await findByID(sessionCollection, record.source_id)
      const clone = await findByID(sessionCollection, record.clone_id)
      assertSource(source, record)
      if (source.release_status === "archived" && clone?.release_status === "active") {
        assertClone(clone, source, record)
        if (await updateHeadForApply(sessionCollection, record) === "updated") updatedHeads += 1
        alreadyApplied += 1
        continue
      }
      if (source.release_status !== "active" || clone != null) {
        throw new Error(`scale catalogue metadata release is not apply-safe: ${record.code}`)
      }
      const archive = await sessionCollection.updateOne({_id: source._id, release_status: "active"}, {$set: {
        release_status: "archived",
        release_archived_at: new Date(record.governed_at),
        updated_at: new Date(record.governed_at),
      }})
      if (archive.matchedCount !== 1 || archive.modifiedCount !== 1) throw new Error(`source activation CAS failed: ${record.code}`)
      await sessionCollection.insertOne(desiredClone(source, record, "active", EJSON))
      if (await updateHeadForApply(sessionCollection, record) === "updated") updatedHeads += 1
      activated += 1
    }
  })
  printSummary({operation: "apply", selected: manifest.records.length, activated, already_applied: alreadyApplied, updated_heads: updatedHeads})
}

function completeness(collection) {
  const active = collection.find(activeScaleFilter).toArray()
  return {
    active: active.length,
    missing_or_invalid_category: active.filter(item => !canonicalCategorySet.has(String(item.category || "").trim())).length,
    forbidden_mbti_tags: active.filter(item => normalizeTags(item.tags).some(tag => tag === "MBTI风格" || tag === "人格" || tag === "偏好" || tag === "沟通风格")).length,
  }
}

function verify(collection, manifest, config) {
  for (const record of manifest.records) {
    const source = findByID(collection, record.source_id)
    const clone = findByID(collection, record.clone_id)
    assertSource(source, record)
    assertClone(clone, source, record)
    if (source.release_status !== "archived" || clone.release_status !== "active") {
      throw new Error(`scale catalogue metadata verification failed: ${record.code}`)
    }
    const head = findByID(collection, record.head_id)
    assertHeadIdentity(head, record)
    if (!headMatches(head, record.desired_category, record.desired_tags)) {
      throw new Error(`scale head metadata verification failed: ${record.code}`)
    }
  }
  const result = completeness(collection)
  if (config.requireComplete && (result.active !== manifest.active_scale_count ||
      result.missing_or_invalid_category !== 0 || result.forbidden_mbti_tags !== 0)) {
    throw new Error(`active scale catalogue metadata remains incomplete: ${JSON.stringify(result)}`)
  }
  printSummary({operation: "verify", verified: manifest.records.length, ...result, require_complete: config.requireComplete})
}

async function rollback(collection, manifest) {
  let rolledBack = 0
  let alreadyRolledBack = 0
  let restoredHeads = 0
  await withGovernanceTransaction(async sessionDB => {
    const sessionCollection = sessionDB.getCollection(collection.getName())
    for (const record of [...manifest.records].reverse()) {
      const source = await findByID(sessionCollection, record.source_id)
      const clone = await findByID(sessionCollection, record.clone_id)
      assertSource(source, record)
      if (source.release_status === "active" && clone?.release_status === "archived") {
        if (await updateHeadForRollback(sessionCollection, record) === "restored") restoredHeads += 1
        alreadyRolledBack += 1
        continue
      }
      assertCloneIdentity(clone, record)
      if (source.release_status !== "archived" || clone.release_status !== "active") {
        throw new Error(`scale catalogue metadata release is not rollback-safe: ${record.code}`)
      }
      const now = new Date()
      const archiveClone = await sessionCollection.updateOne({_id: clone._id, release_status: "active"}, {$set: {
        release_status: "archived", release_archived_at: now, updated_at: now,
      }})
      if (archiveClone.matchedCount !== 1 || archiveClone.modifiedCount !== 1) throw new Error(`governed snapshot rollback CAS failed: ${record.code}`)
      const restoreSource = await sessionCollection.updateOne({_id: source._id, release_status: "archived"}, {
        $set: {release_status: "active", updated_at: new Date(record.governed_at)},
        $unset: {release_archived_at: ""},
      })
      if (restoreSource.matchedCount !== 1 || restoreSource.modifiedCount !== 1) throw new Error(`source snapshot restore CAS failed: ${record.code}`)
      if (await updateHeadForRollback(sessionCollection, record) === "restored") restoredHeads += 1
      rolledBack += 1
    }
  })
  printSummary({operation: "rollback", selected: manifest.records.length, rolled_back: rolledBack, already_rolled_back: alreadyRolledBack, restored_heads: restoredHeads})
}

function printSummary(summary) {
  print("SCALE_CATALOG_METADATA_GOVERNANCE_RESULT")
  print(EJSON.stringify(summary, null, 2, {relaxed: false}))
}

async function main() {
  const fs = require("fs")
  const config = readConfig(process.env)
  const collection = db.getCollection("assessment_models")
  if (config.operation === "audit") {
    audit(collection, config, fs)
    return
  }
  const manifest = loadManifest(fs, config.manifestPath)
  if (manifest.database !== db.getName() || manifest.collection !== collection.getName()) {
    throw new Error("scale catalogue metadata governance target mismatch")
  }
  if (config.operation === "apply") await apply(collection, manifest)
  else if (config.operation === "verify") verify(collection, manifest, config)
  else await rollback(collection, manifest)
}

if (typeof module !== "undefined") {
  module.exports = {
    schemaVersion,
    releaseVersionSuffix,
    applyConfirmation,
    rollbackConfirmation,
    canonicalCategories,
    metadataOverrides,
    isObjectRecord,
    normalizeTags,
    desiredMetadata,
    targetReleaseVersion,
    protectedSnapshotHash,
    protectedSnapshotDriftFields,
    readConfig,
    recordFingerprint,
    validateManifest,
    desiredClone,
    nextRevision,
  }
}

if (typeof db !== "undefined" && typeof EJSON !== "undefined" && typeof ObjectId !== "undefined") {
  main().catch(error => {
    print(error.stack || error.message || error)
    process.exit(1)
  })
}
