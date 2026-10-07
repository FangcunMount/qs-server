// Execute the entire mongosh entry offline. Any business read/write command fails.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const path = require("node:path");
const script = fs.readFileSync(path.join(__dirname, "production-inventory.js"), "utf8");
const secret = "FIXTURE_SECRET_AND_PATIENT_TEXT_MUST_NEVER_BE_OUTPUT";
async function execute(options = {}) {
  const output = [];
  const commands = [];
  let uri;
  let exitStatus = 0;
  let headRead = false;
  function failure(stage, namespace) {
    const selected = !options.denyNamespace || options.denyNamespace === namespace;
    if (selected && options.throwStage === stage) throw Object.assign(new Error(secret), {code: options.code});
    if (selected && options.denyBoth && ["collStats", "listIndexes"].includes(stage)) return {ok: 0, code: 13, errmsg: secret};
    if (selected && options.rejectStage === stage) return {ok: 0, code: options.code, errmsg: secret, namespace: secret, username: secret};
    return null;
  }
  const target = {
    async runCommand(command) {
      await Promise.resolve();
      commands.push(command);
      if (command.collStats || command.listIndexes) assert.equal(headRead, true, "head must be read before namespace details");
      if (options.fail) throw new Error(secret);
      const stage = command.listCollections || command.getMore ? "listCollections" : command.collStats ? "collStats" : command.listIndexes ? "listIndexes" : "unexpected";
      const rejected = failure(stage, command.collStats || command.listIndexes);
      if (rejected) return rejected;
      if (command.listCollections) return {ok: 1, cursor: {id: "1", ns: "qs.$cmd.listCollections", firstBatch: [{name: options.noMigration ? "other_metadata" : "schema_migrations", type: "collection", options: {}}, {name: "reports", type: "collection", options: {validator: {$jsonSchema: {description: secret, properties: {domain_id: {description: secret}}}}}}]}};
      if (command.getMore) return {ok: 1, cursor: {id: "0", ns: "qs.$cmd.listCollections", nextBatch: [{name: "historical_view", type: "view", options: {pipeline: [{body: secret}]}}]}};
      if (command.collStats) return {ok: 1, count: 4, size: 60, storageSize: 16384, totalIndexSize: 16384, body: secret};
      if (command.listIndexes) return {ok: 1, cursor: {id: "0", ns: "qs." + command.listIndexes, firstBatch: [{name: "expire_at", key: {expires_at: 1}, expireAfterSeconds: 0, partialFilterExpression: {body: secret}}, {name: "_id_", key: {_id: 1}}]}};
      throw new Error("unexpected command: " + Object.keys(command).join(","));
    },
    getCollection(name) {
      assert.equal(name, "schema_migrations", "only migration documents can be read");
      return {find(filter, projection) {
        failure("migration");
        assert.deepEqual(JSON.parse(JSON.stringify(filter)), {});
        assert.deepEqual(JSON.parse(JSON.stringify(projection)), {_id: 0, version: 1, dirty: 1});
        return {limit(limit) {assert.equal(limit, 2); return this;}, maxTimeMS(ms) {assert.equal(ms, 5000); return this;}, async toArray() {await Promise.resolve(); headRead = true; if (options.rejectToArray) throw Object.assign(new Error(secret), {code: 13}); return options.invalidHead ? [] : [{version: 36, dirty: false, body: secret}];}};
      }};
    }
  };
  const sandbox = {
    process: {env: {MONGODB_HOST: "mongo.fixture", MONGODB_PORT: "27017", MONGODB_USERNAME: "fixture", MONGODB_PASSWORD: secret, MONGODB_DBNAME: "qs"}},
    Mongo: function (connectionURI) {
      uri = connectionURI;
      const connect = async () => {
        await Promise.resolve();
        failure("connect");
        return {async getDB(name) {
          await Promise.resolve();
          return name === "admin" ? {async auth(user, password) {
            await Promise.resolve();
            assert.equal(password, secret); failure("auth");
            return options.authFalse ? 0 : Object.prototype.hasOwnProperty.call(options, "authResult") ? options.authResult : {ok: 1};
          }} : target;
        }};
      };
      return connect();
    },
    print(value) {output.push(value);}, quit(code) {exitStatus = code; throw Object.assign(new Error("exit"), {code});}
  };
  try {await vm.runInNewContext(script, sandbox);} catch (error) {if (error.code !== 1) throw error;}
  assert.equal(exitStatus, options.expectFailure ? 1 : 0, "failed inventory must exit nonzero");
  assert.ok(!uri.includes(secret), "credentials must not appear in Mongo URI");
  assert.ok(!output.join("\n").includes(secret), "raw body, validator values and errors must not be printed");
  for (const command of commands) {
    if (!command.getMore) assert.equal(command.maxTimeMS, 5000);
    assert.ok(!command.$out && !command.$merge && !command.update && !command.insert && !command.delete);
  }
  return JSON.parse(output[0]);
}
(async function verifyAsyncEntry() {
const result = await execute();
assert.equal(result.namespaces.length, 3);
assert.equal(result.namespaces.find(item => item.type === "view").storage, null);
assert.deepEqual(result.namespaces.find(item => item.name === "reports").validator_fields, ["domain_id"]);
assert.equal(result.namespaces[0].indexes.length, 0); // sorted historical_view first
assert.equal(result.namespaces.find(item => item.name === "reports").indexes[0].expire_after_seconds, "0");
assert.equal(result.namespaces.find(item => item.name === "reports").indexes[1].unique, true);
assert.deepEqual(result.migration_state, [{version: "36", dirty: false}]);
assert.equal(result.metadata_complete, true);
assert.ok(result.namespaces.every(item => item.metadata_error.length === 0));
assert.deepEqual(await execute({fail: true, expectFailure: true}), {error: "mongo_inventory_listCollections_code_none"});
for (const stage of ["connect", "auth", "listCollections", "collStats", "listIndexes", "migration"]) {
  assert.deepEqual(await execute({throwStage: stage, code: 18, expectFailure: true}), {error: `mongo_inventory_${stage}_code_18`});
}
assert.deepEqual(await execute({rejectStage: "listCollections", code: 13, expectFailure: true}), {error: "mongo_inventory_listCollections_code_13"});
for (const stage of ["collStats", "listIndexes"]) {
  for (const mode of ["rejectStage", "throwStage"]) {
    const partial = await execute({[mode]: stage, code: 13, denyNamespace: "reports", expectFailure: true});
    assert.equal(partial.metadata_complete, false);
    assert.equal(partial.namespaces.length, 3, "complete catalog must be retained");
    assert.deepEqual(partial.migration_state, [{version: "36", dirty: false}]);
    const unknown = partial.namespaces.find(item => item.name === "reports");
    assert.deepEqual(unknown.validator_fields, ["domain_id"]);
    assert.deepEqual(unknown.metadata_error, [stage + "_unauthorized_code_13"]);
    assert.equal(unknown[stage === "collStats" ? "storage" : "indexes"], null);
    assert.notEqual(unknown[stage === "collStats" ? "indexes" : "storage"], null);
    const allowed = partial.namespaces.find(item => item.name === "schema_migrations");
    assert.notEqual(allowed.storage, null); assert.notEqual(allowed.indexes, null);
    assert.deepEqual(allowed.metadata_error, []);
  }
  for (const code of [12, 18, "13", undefined]) {
    const suffix = typeof code === "number" ? String(code) : "none";
    assert.deepEqual(await execute({rejectStage: stage, code, expectFailure: true}), {error: `mongo_inventory_${stage}_code_${suffix}`});
  }
}
for (const stage of ["connect", "auth", "migration"]) {
  assert.deepEqual(await execute({throwStage: stage, code: 13, expectFailure: true}), {error: `mongo_inventory_${stage}_code_13`});
}
const bothDenied = await execute({denyBoth: true, denyNamespace: "reports", expectFailure: true});
const bothUnknown = bothDenied.namespaces.find(item => item.name === "reports");
assert.equal(bothUnknown.storage, null); assert.equal(bothUnknown.indexes, null);
assert.deepEqual(bothUnknown.metadata_error, ["collStats_unauthorized_code_13", "listIndexes_unauthorized_code_13"]);
assert.equal(bothDenied.metadata_complete, false);
assert.deepEqual(await execute({authFalse: true, expectFailure: true}), {error: "mongo_inventory_auth_code_none"});
for (const authResult of [1, {ok: 1}]) {
  assert.equal((await execute({authResult})).namespaces.length, 3);
}
for (const authResult of [0, {ok: 0}, null, undefined, true, "1", {}, {ok: "1"}, []]) {
  assert.deepEqual(await execute({authResult, expectFailure: true}), {error: "mongo_inventory_auth_code_none"});
}
assert.deepEqual(await execute({noMigration: true, expectFailure: true}), {error: "mongo_inventory_migration_code_none"});
assert.deepEqual(await execute({invalidHead: true, expectFailure: true}), {error: "mongo_inventory_head_code_none"});
assert.deepEqual(await execute({rejectToArray: true, expectFailure: true}), {error: "mongo_inventory_migration_code_13"});
for (const code of [-1, 2147483648, 1.5, "18", secret, {toString() {throw new Error(secret);}}, undefined]) {
  assert.deepEqual(await execute({throwStage: "auth", code, expectFailure: true}), {error: "mongo_inventory_auth_code_none"});
}
console.log("Mongo Promise metadata-only and output safety fixture OK");
})().catch(() => { console.error("Mongo Promise metadata inventory fixture failed"); process.exitCode = 1; });
