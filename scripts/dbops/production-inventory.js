// Metadata only. Credentials come from environment, never URI/argv/output.
try {
  const connection = new Mongo(`mongodb://${process.env.MONGODB_HOST}:${process.env.MONGODB_PORT}/?connectTimeoutMS=5000&serverSelectionTimeoutMS=5000`);
  if (!connection.getDB("admin").auth(process.env.MONGODB_USERNAME, process.env.MONGODB_PASSWORD)) {
    throw new Error("authentication_failed");
  }
  const target = connection.getDB(process.env.MONGODB_DBNAME);
  const started = Date.now();
  const maxTimeMS = 5000;
  const number = value => value === undefined || value === null ? null : String(value);
  function metadataCursor(command, limit) {
    let result = target.runCommand({...command, maxTimeMS});
    if (result.ok !== 1 || !result.cursor) throw new Error("metadata_command_failed");
    const rows = [...(result.cursor.firstBatch || [])];
    while (String(result.cursor.id) !== "0") {
      if (rows.length >= limit || Date.now() - started > 70000) throw new Error("metadata_limit_exceeded");
      result = target.runCommand({getMore: result.cursor.id, collection: result.cursor.ns.substring(process.env.MONGODB_DBNAME.length + 1), batchSize: 200});
      if (result.ok !== 1 || !result.cursor) throw new Error("metadata_cursor_failed");
      rows.push(...(result.cursor.nextBatch || []));
    }
    if (rows.length > limit) throw new Error("metadata_limit_exceeded");
    return rows;
  }
  const namespaces = metadataCursor({listCollections: 1, nameOnly: false, authorizedCollections: false, cursor: {batchSize: 200}}, 1000);
  const report = {database: process.env.MONGODB_DBNAME, namespaces: [], migration_state: []};
  let totalIndexes = 0;
  let totalFields = 0;
  for (const namespace of namespaces.sort((a, b) => a.name.localeCompare(b.name))) {
    if (Date.now() - started > 70000) throw new Error("metadata_deadline_exceeded");
    const properties = namespace.options?.validator?.$jsonSchema?.properties || {};
    const item = {name: namespace.name, type: namespace.type, validator_fields: Object.keys(properties).sort(), storage: null, indexes: []};
    totalFields += item.validator_fields.length;
    if (totalFields > 10000) throw new Error("metadata_limit_exceeded");
    if (item.type === "collection" || item.type === "timeseries") {
      const stats = target.runCommand({collStats: item.name, scale: 1, maxTimeMS});
      if (stats.ok !== 1) throw new Error("storage_metadata_failed");
      item.storage = {estimated_documents: number(stats.count), data_bytes: number(stats.size), storage_bytes: number(stats.storageSize), index_bytes: number(stats.totalIndexSize)};
      item.indexes = metadataCursor({listIndexes: item.name, cursor: {batchSize: 200}}, 1000).map(index => ({
        name: index.name, keys: index.key, unique: index.unique === true || index.name === "_id_", sparse: index.sparse === true,
        hidden: index.hidden === true, partial: index.partialFilterExpression !== undefined,
        expire_after_seconds: number(index.expireAfterSeconds)
      }));
    }
    totalIndexes += item.indexes.length;
    if (totalIndexes > 10000) throw new Error("metadata_limit_exceeded");
    report.namespaces.push(item);
  }
  if (!namespaces.some(item => item.name === "schema_migrations" && item.type === "collection")) throw new Error("migration_metadata_missing");
  report.migration_state = target.getCollection("schema_migrations").find({}, {_id: 0, version: 1, dirty: 1}).limit(2).maxTimeMS(maxTimeMS).toArray().map(head => ({version: number(head.version), dirty: head.dirty}));
  print(JSON.stringify(report));
} catch (_) {
  // Raw server errors can contain connection strings; emit only a fixed category.
  print(JSON.stringify({error: "mongo_metadata_inventory_failed"}));
  quit(1);
}
