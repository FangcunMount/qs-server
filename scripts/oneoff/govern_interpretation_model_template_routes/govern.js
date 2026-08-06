// Manifest-driven cutover of active ModelCatalog snapshots to an explicit
// Interpretation TemplateID + TemplateVersion. Run with mongosh.
"use strict"

const crypto = require("crypto")

const governanceSchemaVersion = "interpretation-model-template-route-governance/v1"
const targetTemplateVersion = "2026-08-v1"
const releaseVersionSuffix = "-report-202608-v1"
const applyConfirmation = "activate-model-template-route-2026-08-v1"
const rollbackConfirmation = "rollback-model-template-route-2026-08-v1"
const activeSnapshotFilter = {
  record_role: "published_snapshot",
  release_status: "active",
  status: "published",
  deleted_at: null
}
const typologyTemplateIDs = new Set(["mbti", "sbti", "bigfive", "enneagram"])
const factorModelKinds = new Set(["scale", "behavioral_rating", "cognitive"])
const factorScoreSectionKind = "factor_scores"

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

function snapshotContentHash(document, ejson) {
  const protectedDocument = deepClone(document, ejson)
  delete protectedDocument.release_status
  delete protectedDocument.release_archived_at
  delete protectedDocument.updated_at
  return sha256JSON(protectedDocument, ejson)
}

function factorScorePlan(definition, kind, ejson) {
  if (!factorModelKinds.has(kind)) {
    if (kind !== "typology") throw new Error(`unsupported published snapshot kind: ${kind || "unknown"}`)
    const sections = definition?.report_map?.sections
    if (!Array.isArray(sections) || sections.length === 0) {
      throw new Error("typology definition_v2.report_map.sections are required")
    }
    return {sections: deepClone(sections, ejson), added: false, sourceRefs: []}
  }

  const factors = definition?.measure?.factors
  if (!Array.isArray(factors) || factors.length === 0) {
    throw new Error("factor model definition_v2.measure.factors are required")
  }
  const factorCodes = []
  const knownCodes = new Set()
  for (const factor of factors) {
    const code = typeof factor?.code === "string" ? factor.code : ""
    if (code === "" || code !== code.trim()) {
      throw new Error("factor model measure factor code must be non-blank and normalized")
    }
    if (knownCodes.has(code)) throw new Error(`factor model measure factor code is duplicated: ${code}`)
    knownCodes.add(code)
    factorCodes.push(code)
  }

  const existing = definition?.report_map?.sections
  const sections = Array.isArray(existing) ? deepClone(existing, ejson) : []
  const factorSections = sections.filter(section => section?.kind === factorScoreSectionKind)
  if (factorSections.length > 1) {
    throw new Error("factor model may contain only one factor_scores report section")
  }
  if (factorSections.length === 1) {
    const refs = factorSections[0].source_refs
    const sourceRefs = Array.isArray(refs) ? [...refs] : []
    const seen = new Set()
    for (const ref of sourceRefs) {
      if (typeof ref !== "string" || ref === "" || ref !== ref.trim()) {
        throw new Error("factor_scores source_ref must be non-blank and normalized")
      }
      if (seen.has(ref)) throw new Error(`factor_scores source_ref is duplicated: ${ref}`)
      if (!knownCodes.has(ref)) throw new Error(`factor_scores source_ref is not a measure factor: ${ref}`)
      seen.add(ref)
    }
    return {sections, added: false, sourceRefs}
  }

  sections.push({
    code: factorScoreSectionKind,
    kind: factorScoreSectionKind,
    source_refs: factorCodes
  })
  return {sections, added: true, sourceRefs: factorCodes}
}

function resolveTemplateID(document) {
  if (document == null || typeof document.kind !== "string") {
    throw new Error("published snapshot kind is required")
  }
  if (factorModelKinds.has(document.kind)) {
    factorScorePlan(document.definition_v2, document.kind)
    return "standard"
  }
  if (document.kind !== "typology") {
    throw new Error(`unsupported published snapshot kind: ${document.kind}`)
  }
  const sections = factorScorePlan(document.definition_v2, document.kind).sections
  const ids = [...new Set(sections.map(section => (section.template_id || "").trim()).filter(Boolean))]
  if (ids.length !== 1 || !typologyTemplateIDs.has(ids[0])) {
    throw new Error(`typology snapshot must resolve one registered template_id: ${document.code || "unknown"}`)
  }
  return ids[0]
}

function materializeDefinition(definition, templateID, kind, ejson) {
  if (!isObjectRecord(definition)) throw new Error("definition_v2 is required")
  const result = deepClone(definition, ejson)
  const plan = factorScorePlan(result, kind, ejson)
  const governedSections = plan.sections.map(section => ({
    ...section,
    template_id: templateID,
    template_version: targetTemplateVersion
  }))
  if (!isObjectRecord(result.report_map)) result.report_map = {}
  result.report_map.sections = governedSections
  if (!isObjectRecord(result.interpretation_assets)) result.interpretation_assets = {}
  if (!isObjectRecord(result.interpretation_assets.report_spec)) result.interpretation_assets.report_spec = {}
  result.interpretation_assets.report_spec.sections = deepClone(governedSections, ejson)
  return result
}

function hasTargetRoute(document, ejson) {
  try {
    const templateID = resolveTemplateID(document)
    const expected = materializeDefinition(document.definition_v2, templateID, document.kind, ejson)
    return valuesEqual(document.definition_v2, expected, ejson)
  } catch (_) {
    return false
  }
}

function targetReleaseVersion(sourceVersion) {
  const normalized = String(sourceVersion || "").trim()
  if (normalized === "") throw new Error("published snapshot release_version is required")
  if (normalized.endsWith(releaseVersionSuffix)) return normalized
  return normalized + releaseVersionSuffix
}

function validateTemplateRelease(collection, templateID, decisionKind) {
  const release = collection.findOne({
    template_id: templateID,
    template_version: targetTemplateVersion,
    report_type: "standard",
    status: "published"
  })
  if (release == null || release.manifest?.template_id !== templateID ||
      release.manifest?.template_version !== targetTemplateVersion ||
      release.manifest_fingerprint == null) {
    throw new Error(`published report template release is unavailable: ${templateID}@${targetTemplateVersion}`)
  }
  if (!Array.isArray(release.manifest.routes) ||
      !release.manifest.routes.some(route => route.decision_kind === decisionKind)) {
    throw new Error(`report template release does not support decision_kind ${decisionKind}: ${templateID}`)
  }
}

function validateSourceSnapshot(document) {
  for (const field of ["kind", "code", "release_version", "decision_kind", "questionnaire_code", "questionnaire_version"]) {
    if (typeof document?.[field] !== "string" || document[field].trim() === "") {
      throw new Error(`published snapshot ${field} is required: ${document?.code || "unknown"}`)
    }
  }
  if (document.record_role !== "published_snapshot" || document.release_status !== "active" ||
      document.status !== "published" || document.deleted_at != null) {
    throw new Error(`published snapshot is not active: ${document.code}`)
  }
  if (!(document.created_at instanceof Date) || !(document.updated_at instanceof Date) ||
      !(document.published_at instanceof Date)) {
    throw new Error(`published snapshot lifecycle timestamps are invalid: ${document.code}`)
  }
}

function desiredClone(source, record, status, ejson) {
  const clone = deepClone(source, ejson)
  clone._id = ObjectId.createFromHexString(record.clone_id)
  clone.release_version = record.target_release_version
  clone.release_status = status
  clone.definition_v2 = materializeDefinition(source.definition_v2, record.template_id, source.kind, ejson)
  const governedAt = new Date(record.governed_at)
  clone.created_at = governedAt
  clone.updated_at = governedAt
  clone.published_at = governedAt
  if (status === "active") delete clone.release_archived_at
  else clone.release_archived_at = governedAt
  clone.source = isObjectRecord(clone.source) ? clone.source : {}
  clone.source.interpretation_template_route_governance = {
    schema_version: governanceSchemaVersion,
    template_version: targetTemplateVersion,
    source_release_version: record.source_release_version,
    source_definition_hash: record.source_definition_hash,
    target_definition_hash: record.target_definition_hash,
    governed_at: record.governed_at
  }
  clone.source.definition_content_hash = record.target_definition_hash
  clone.source.definition_hash_schema = "definition-v2/v1"
  if (ejson != null) return ejson.deserialize(ejson.serialize(clone, {relaxed: false}))
  return clone
}

function readConfig(env) {
  const operation = env.MODEL_TEMPLATE_ROUTE_OPERATION || "audit"
  if (!["audit", "apply", "verify", "rollback"].includes(operation)) {
    throw new Error("MODEL_TEMPLATE_ROUTE_OPERATION must be audit, apply, verify or rollback")
  }
  const manifestPath = env.MODEL_TEMPLATE_ROUTE_MANIFEST_PATH || ""
  if (manifestPath === "") throw new Error("MODEL_TEMPLATE_ROUTE_MANIFEST_PATH is required")
  const afterID = env.MODEL_TEMPLATE_ROUTE_AFTER_ID || ""
  if (afterID !== "" && !/^[a-f0-9]{24}$/.test(afterID)) {
    throw new Error("MODEL_TEMPLATE_ROUTE_AFTER_ID must be a lowercase ObjectId hex string")
  }
  const maxRecords = Number.parseInt(env.MODEL_TEMPLATE_ROUTE_MAX_RECORDS || "0", 10)
  if (!Number.isSafeInteger(maxRecords) || maxRecords < 0) {
    throw new Error("MODEL_TEMPLATE_ROUTE_MAX_RECORDS must be a non-negative integer")
  }
  const requireComplete = env.MODEL_TEMPLATE_ROUTE_REQUIRE_COMPLETE === "true"
  if (operation === "apply" && env.MODEL_TEMPLATE_ROUTE_CONFIRM !== applyConfirmation) {
    throw new Error(`apply requires MODEL_TEMPLATE_ROUTE_CONFIRM=${applyConfirmation}`)
  }
  if (operation === "rollback" && env.MODEL_TEMPLATE_ROUTE_CONFIRM !== rollbackConfirmation) {
    throw new Error(`rollback requires MODEL_TEMPLATE_ROUTE_CONFIRM=${rollbackConfirmation}`)
  }
  return {operation, manifestPath, afterID, maxRecords, requireComplete}
}

function validateGovernanceManifest(manifest) {
  if (manifest?.schema_version !== governanceSchemaVersion || manifest?.target_template_version !== targetTemplateVersion) {
    throw new Error("unsupported model template route governance manifest")
  }
  if (!Array.isArray(manifest.records) || manifest.records_fingerprint !== sha256JSON(manifest.records)) {
    throw new Error("model template route governance fingerprint mismatch")
  }
  const sourceIDs = new Set()
  const cloneIDs = new Set()
  for (const record of manifest.records) {
    if (!/^[a-f0-9]{24}$/.test(record.source_id || "") || !/^[a-f0-9]{24}$/.test(record.clone_id || "") ||
        sourceIDs.has(record.source_id) || cloneIDs.has(record.clone_id) ||
        !/^[a-f0-9]{64}$/.test(record.source_content_hash || "") ||
        !/^[a-f0-9]{64}$/.test(record.source_definition_hash || "") ||
        !/^[a-f0-9]{64}$/.test(record.target_definition_hash || "") ||
        typeof record.template_id !== "string" || typeof record.source_release_version !== "string" ||
        typeof record.factor_score_section_added !== "boolean" ||
        !Array.isArray(record.factor_source_refs) || record.factor_source_refs.some(ref => typeof ref !== "string") ||
        record.target_release_version !== targetReleaseVersion(record.source_release_version) || !record.governed_at) {
      throw new Error(`invalid model template route governance record: ${record.code || "unknown"}`)
    }
    sourceIDs.add(record.source_id)
    cloneIDs.add(record.clone_id)
  }
  return manifest
}

function selectedRecords(governance, config) {
  let records = governance.records
  if (config.afterID !== "") records = records.filter(record => record.source_id > config.afterID)
  if (config.maxRecords > 0) records = records.slice(0, config.maxRecords)
  return records
}

function writeManifest(fs, path, manifest) {
  const parent = require("path").dirname(path)
  fs.mkdirSync(parent, {recursive: true})
  const temporary = `${path}.partial`
  fs.writeFileSync(temporary, `${JSON.stringify(manifest, null, 2)}\n`, {mode: 0o600})
  fs.renameSync(temporary, path)
}

function loadManifest(fs, path) {
  return validateGovernanceManifest(JSON.parse(fs.readFileSync(path, "utf8")))
}

function audit(collection, templates, config, fs) {
  const records = []
  let alreadyCurrent = 0
  let factorScoreSectionsAdded = 0
  const governedAt = new Date().toISOString()
  const cursor = collection.find(activeSnapshotFilter).sort({_id: 1})
  while (cursor.hasNext()) {
    const document = cursor.next()
    validateSourceSnapshot(document)
    if (hasTargetRoute(document, EJSON)) {
      alreadyCurrent += 1
      continue
    }
    const templateID = resolveTemplateID(document)
    const routePlan = factorScorePlan(document.definition_v2, document.kind, EJSON)
    if (routePlan.added) factorScoreSectionsAdded += 1
    validateTemplateRelease(templates, templateID, document.decision_kind)
    const targetVersion = targetReleaseVersion(document.release_version)
    if (collection.countDocuments({
      record_role: "published_snapshot",
      kind: document.kind,
      algorithm: document.algorithm,
      code: document.code,
      release_version: targetVersion,
      deleted_at: null
    }) !== 0) {
      throw new Error(`target published snapshot already exists: ${document.code}@${targetVersion}`)
    }
    records.push({
      source_id: document._id.toHexString(),
      clone_id: new ObjectId().toHexString(),
      kind: document.kind,
      algorithm: document.algorithm || "",
      code: document.code,
      decision_kind: document.decision_kind,
      template_id: templateID,
      factor_score_section_added: routePlan.added,
      factor_source_refs: routePlan.sourceRefs,
      source_release_version: document.release_version,
      target_release_version: targetVersion,
      source_updated_at: document.updated_at.toISOString(),
      governed_at: governedAt,
      source_content_hash: snapshotContentHash(document, EJSON)
    })
  }
  const manifest = {
    schema_version: governanceSchemaVersion,
    database: db.getName(),
    collection: collection.getName(),
    generated_at: governedAt,
    target_template_version: targetTemplateVersion,
    records,
    records_fingerprint: sha256JSON(records)
  }
  writeManifest(fs, config.manifestPath, manifest)
  printSummary({
    operation: "audit",
    records: records.length,
    already_current: alreadyCurrent,
    factor_score_sections_added: factorScoreSectionsAdded,
    manifest_path: config.manifestPath,
    records_fingerprint: manifest.records_fingerprint
  })
}

function findByID(collection, id) {
  return collection.findOne({_id: ObjectId.createFromHexString(id)})
}

function assertSourceContent(document, record) {
  if (document == null || document.code !== record.code || document.release_version !== record.source_release_version ||
      snapshotContentHash(document, EJSON) !== record.source_content_hash) {
    throw new Error(`source published snapshot changed: ${record.code}`)
  }
}

function assertCloneContent(document, source, record) {
  if (document == null || document.code !== record.code || document.release_version !== record.target_release_version ||
      snapshotContentHash(document, EJSON) !== snapshotContentHash(desiredClone(source, record, document.release_status, EJSON), EJSON)) {
    throw new Error(`governed published snapshot changed: ${record.code}`)
  }
}

async function withGovernanceTransaction(callback) {
  const session = db.getMongo().startSession()
  try {
    return await session.withTransaction(async () => callback(session.getDatabase(db.getName())))
  } finally {
    await session.endSession()
  }
}

async function apply(collection, governance, config) {
  const records = selectedRecords(governance, config)
  let activated = 0
  let reactivated = 0
  let alreadyApplied = 0
  await withGovernanceTransaction(async sessionDB => {
    const sessionCollection = sessionDB.getCollection(collection.getName())
    for (const record of records) {
      const source = await findByID(sessionCollection, record.source_id)
      const clone = await findByID(sessionCollection, record.clone_id)
      assertSourceContent(source, record)
      if (source.release_status === "archived" && clone?.release_status === "active") {
        assertCloneContent(clone, source, record)
        alreadyApplied += 1
        continue
      }
      if (source.release_status !== "active") {
        throw new Error(`source snapshot is not active: ${record.code} status=${String(source.release_status)}`)
      }
      const archiveResult = await sessionCollection.updateOne({_id: source._id, release_status: "active"}, {$set: {
        release_status: "archived", release_archived_at: new Date(record.governed_at), updated_at: new Date(record.governed_at)
      }})
      if (archiveResult.matchedCount !== 1 || archiveResult.modifiedCount !== 1) {
        throw new Error(`source snapshot lost activation CAS: ${record.code}`)
      }
      if (clone == null) {
        await sessionCollection.insertOne(desiredClone(source, record, "active", EJSON))
        activated += 1
      } else {
        assertCloneContent(clone, source, record)
        if (clone.release_status !== "archived") throw new Error(`governed snapshot is not rollback-safe: ${record.code}`)
        const result = await sessionCollection.updateOne({_id: clone._id, release_status: "archived"}, {
          $set: {release_status: "active", updated_at: new Date(record.governed_at)},
          $unset: {release_archived_at: ""}
        })
        if (result.matchedCount !== 1 || result.modifiedCount !== 1) {
          throw new Error(`governed snapshot lost reactivation CAS: ${record.code}`)
        }
        reactivated += 1
      }
    }
  })
  printSummary({operation: "apply", selected: records.length, activated, reactivated, already_applied: alreadyApplied})
}

function activeCompleteness(collection) {
  let active = 0
  let complete = 0
  const cursor = collection.find(activeSnapshotFilter)
  while (cursor.hasNext()) {
    active += 1
    if (hasTargetRoute(cursor.next(), EJSON)) complete += 1
  }
  return {active, complete, missing: active - complete}
}

function verify(collection, governance, config) {
  const records = selectedRecords(governance, config)
  for (const record of records) {
    const source = findByID(collection, record.source_id)
    const clone = findByID(collection, record.clone_id)
    assertSourceContent(source, record)
    assertCloneContent(clone, source, record)
    if (source.release_status !== "archived" || clone.release_status !== "active" || !hasTargetRoute(clone, EJSON)) {
      throw new Error(`model template route verification failed: ${record.code}`)
    }
  }
  const completeness = activeCompleteness(collection)
  if (config.requireComplete && completeness.missing !== 0) {
    throw new Error(`active model template route set is incomplete: ${completeness.complete}/${completeness.active}`)
  }
  printSummary({operation: "verify", verified: records.length, ...completeness, require_complete: config.requireComplete})
}

async function rollback(collection, governance, config) {
  const records = selectedRecords(governance, config)
  let rolledBack = 0
  let alreadyRolledBack = 0
  await withGovernanceTransaction(async sessionDB => {
    const sessionCollection = sessionDB.getCollection(collection.getName())
    for (const record of [...records].reverse()) {
      const source = await findByID(sessionCollection, record.source_id)
      const clone = await findByID(sessionCollection, record.clone_id)
      assertSourceContent(source, record)
      if (clone == null) {
        if (source.release_status !== "active") throw new Error(`source snapshot cannot be restored: ${record.code}`)
        alreadyRolledBack += 1
        continue
      }
      assertCloneContent(clone, source, record)
      if (source.release_status === "active" && clone.release_status === "archived") {
        alreadyRolledBack += 1
        continue
      }
      if (source.release_status !== "archived" || clone.release_status !== "active") {
        throw new Error(`model template route is not rollback-safe: ${record.code}`)
      }
      const archiveClone = await sessionCollection.updateOne({_id: clone._id, release_status: "active"}, {$set: {
        release_status: "archived", release_archived_at: new Date(), updated_at: new Date()
      }})
      if (archiveClone.matchedCount !== 1 || archiveClone.modifiedCount !== 1) {
        throw new Error(`governed snapshot lost rollback CAS: ${record.code}`)
      }
      const restoreSource = await sessionCollection.updateOne({_id: source._id, release_status: "archived"}, {
        $set: {release_status: "active", updated_at: new Date(record.source_updated_at)},
        $unset: {release_archived_at: ""}
      })
      if (restoreSource.matchedCount !== 1 || restoreSource.modifiedCount !== 1) {
        throw new Error(`source snapshot lost rollback CAS: ${record.code}`)
      }
      rolledBack += 1
    }
  })
  printSummary({operation: "rollback", selected: records.length, rolled_back: rolledBack, already_rolled_back: alreadyRolledBack})
}

function printSummary(summary) {
  print("INTERPRETATION_MODEL_TEMPLATE_ROUTE_GOVERNANCE_RESULT")
  print(EJSON.stringify(summary, null, 2, {relaxed: false}))
}

async function main() {
  const fs = require("fs")
  const config = readConfig(process.env)
  const collection = db.getCollection("assessment_models")
  if (config.operation === "audit") {
    audit(collection, db.getCollection("interpretation_report_templates"), config, fs)
    return
  }
  const governance = loadManifest(fs, config.manifestPath)
  if (governance.database !== db.getName() || governance.collection !== collection.getName()) {
    throw new Error("model template route governance target mismatch")
  }
  if (config.operation === "apply") await apply(collection, governance, config)
  else if (config.operation === "verify") verify(collection, governance, config)
  else await rollback(collection, governance, config)
}

if (typeof module !== "undefined") {
  module.exports = {
    governanceSchemaVersion,
    targetTemplateVersion,
    releaseVersionSuffix,
    applyConfirmation,
    rollbackConfirmation,
    cloneValue,
    deepClone,
    canonicalJSONString,
    sha256JSON,
    snapshotContentHash,
    resolveTemplateID,
    factorScorePlan,
    materializeDefinition,
    hasTargetRoute,
    targetReleaseVersion,
    readConfig,
    validateGovernanceManifest,
    selectedRecords,
    desiredClone
  }
}

if (typeof db !== "undefined" && typeof EJSON !== "undefined" && typeof ObjectId !== "undefined") {
  main().catch(error => {
    print(error.stack || error.message || error)
    process.exit(1)
  })
}
