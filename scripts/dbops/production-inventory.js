// Metadata only. Credentials come from environment, never URI/argv/output.
(async function metadataInventory() {
let stage = "connect";
try {
  const connection = await new Mongo(`mongodb://${process.env.MONGODB_HOST}:${process.env.MONGODB_PORT}/?connectTimeoutMS=5000&serverSelectionTimeoutMS=5000`);
  stage = "auth";
  const admin = await connection.getDB("admin");
  const authenticated = await admin.auth(process.env.MONGODB_USERNAME, process.env.MONGODB_PASSWORD);
  if (authenticated !== 1 && !(authenticated !== null && typeof authenticated === "object" && authenticated.ok === 1)) {
    throw new Error("authentication_failed");
  }
  const target = await connection.getDB(process.env.MONGODB_DBNAME);
  const started = Date.now();
  const maxTimeMS = 5000;
  const number = value => value === undefined || value === null ? null : String(value);
  async function metadataCursor(command, limit, operation) {
    stage = operation;
    let result = await target.runCommand({...command, maxTimeMS});
    if (result.ok !== 1 || !result.cursor) throw Object.assign(new Error("metadata_command_failed"), {code: result.code});
    const rows = [...(result.cursor.firstBatch || [])];
    while (String(result.cursor.id) !== "0") {
      if (rows.length >= limit || Date.now() - started > 70000) throw new Error("metadata_limit_exceeded");
      result = await target.runCommand({getMore: result.cursor.id, collection: result.cursor.ns.substring(process.env.MONGODB_DBNAME.length + 1), batchSize: 200});
      if (result.ok !== 1 || !result.cursor) throw Object.assign(new Error("metadata_cursor_failed"), {code: result.code});
      rows.push(...(result.cursor.nextBatch || []));
    }
    if (rows.length > limit) throw new Error("metadata_limit_exceeded");
    return rows;
  }
  const namespaces = await metadataCursor({listCollections: 1, nameOnly: false, authorizedCollections: false, cursor: {batchSize: 200}}, 1000, "listCollections");
  // Obtain and validate migration identity before any optional detail query.
  stage = "migration";
  if (!namespaces.some(item => item.name === "schema_migrations" && item.type === "collection")) throw new Error("migration_metadata_missing");
  const migration = await target.getCollection("schema_migrations");
  const migrationState = (await migration.find({}, {_id: 0, version: 1, dirty: 1}).limit(2).maxTimeMS(maxTimeMS).toArray()).map(head => ({version: number(head.version), dirty: head.dirty}));
  stage = "head";
  if (migrationState.length !== 1 || !/^[0-9]{1,20}$/.test(migrationState[0].version) || typeof migrationState[0].dirty !== "boolean") {
    throw new Error("migration_metadata_incomplete");
  }
  const report = {database: process.env.MONGODB_DBNAME, namespaces: [], migration_state: migrationState, metadata_complete: true};
  let totalIndexes = 0;
  let totalFields = 0;
  for (const namespace of namespaces.sort((a, b) => a.name.localeCompare(b.name))) {
    if (Date.now() - started > 70000) throw new Error("metadata_deadline_exceeded");
    const properties = namespace.options?.validator?.$jsonSchema?.properties || {};
    const item = {name: namespace.name, type: namespace.type, validator_fields: Object.keys(properties).sort(), storage: null, indexes: [], metadata_error: []};
    totalFields += item.validator_fields.length;
    if (totalFields > 10000) throw new Error("metadata_limit_exceeded");
    if (item.type === "collection" || item.type === "timeseries") {
      stage = "collStats";
      try {
        const stats = await target.runCommand({collStats: item.name, scale: 1, maxTimeMS});
        if (stats.ok !== 1) throw Object.assign(new Error("storage_metadata_failed"), {code: stats.code});
        item.storage = {estimated_documents: number(stats.count), data_bytes: number(stats.size), storage_bytes: number(stats.storageSize), index_bytes: number(stats.totalIndexSize)};
      } catch (error) {
        if (error?.code !== 13) throw error;
        item.metadata_error.push("collStats_unauthorized_code_13");
        report.metadata_complete = false;
      }
      try {
        item.indexes = (await metadataCursor({listIndexes: item.name, cursor: {batchSize: 200}}, 1000, "listIndexes")).map(index => ({
          name: index.name, keys: index.key, unique: index.unique === true || index.name === "_id_", sparse: index.sparse === true,
          hidden: index.hidden === true, partial: index.partialFilterExpression !== undefined,
          expire_after_seconds: number(index.expireAfterSeconds)
        }));
      } catch (error) {
        if (error?.code !== 13) throw error;
        item.indexes = null;
        item.metadata_error.push("listIndexes_unauthorized_code_13");
        report.metadata_complete = false;
      }
    }
    totalIndexes += item.indexes === null ? 0 : item.indexes.length;
    if (totalIndexes > 10000) throw new Error("metadata_limit_exceeded");
    report.namespaces.push(item);
  }
  print(JSON.stringify(report));
  return report.metadata_complete;
} catch (error) {
  // Do not emit message, namespace, connection URI, username or raw error.
  const code = typeof error?.code === "number" && Number.isInteger(error.code) && error.code >= 0 && error.code <= 2147483647 ? String(error.code) : "none";
  print(JSON.stringify({error: `mongo_inventory_${stage}_code_${code}`}));
  return false;
}
})().then(complete => { if (!complete) quit(1); });
