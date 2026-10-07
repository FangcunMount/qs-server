// Execute the entire mongosh entry offline. Any business read/write command fails.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const path = require("node:path");
const script = fs.readFileSync(path.join(__dirname, "production-inventory.js"), "utf8");
const secret = "FIXTURE_SECRET_AND_PATIENT_TEXT_MUST_NEVER_BE_OUTPUT";
function execute(options = {}) {
  const output = [];
  const commands = [];
  let uri;
  let exitStatus = 0;
  function failure(stage) {
    if (options.throwStage === stage) throw Object.assign(new Error(secret), {code: options.code});
    if (options.rejectStage === stage) return {ok: 0, code: options.code, errmsg: secret, namespace: secret, username: secret};
    return null;
  }
  const target = {
    runCommand(command) {
      commands.push(command);
      if (options.fail) throw new Error(secret);
      const stage = command.listCollections || command.getMore ? "listCollections" : command.collStats ? "collStats" : command.listIndexes ? "listIndexes" : "unexpected";
      const rejected = failure(stage);
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
        return {limit(limit) {assert.equal(limit, 2); return this;}, maxTimeMS(ms) {assert.equal(ms, 5000); return this;}, toArray() {return options.invalidHead ? [] : [{version: 36, dirty: false, body: secret}];}};
      }};
    }
  };
  const sandbox = {
    process: {env: {MONGODB_HOST: "mongo.fixture", MONGODB_PORT: "27017", MONGODB_USERNAME: "fixture", MONGODB_PASSWORD: secret, MONGODB_DBNAME: "qs"}},
    Mongo: function (connectionURI) {uri = connectionURI; failure("connect"); return {getDB(name) {return name === "admin" ? {auth(user, password) {assert.equal(password, secret); failure("auth"); return options.authFalse ? 0 : 1;}} : target;}};},
    print(value) {output.push(value);}, quit(code) {exitStatus = code; throw Object.assign(new Error("exit"), {code});}
  };
  try {vm.runInNewContext(script, sandbox);} catch (error) {if (error.code !== 1) throw error;}
  assert.equal(exitStatus, options.expectFailure ? 1 : 0, "failed inventory must exit nonzero");
  assert.ok(!uri.includes(secret), "credentials must not appear in Mongo URI");
  assert.ok(!output.join("\n").includes(secret), "raw body, validator values and errors must not be printed");
  for (const command of commands) {
    if (!command.getMore) assert.equal(command.maxTimeMS, 5000);
    assert.ok(!command.$out && !command.$merge && !command.update && !command.insert && !command.delete);
  }
  return JSON.parse(output[0]);
}
const result = execute();
assert.equal(result.namespaces.length, 3);
assert.equal(result.namespaces.find(item => item.type === "view").storage, null);
assert.deepEqual(result.namespaces.find(item => item.name === "reports").validator_fields, ["domain_id"]);
assert.equal(result.namespaces[0].indexes.length, 0); // sorted historical_view first
assert.equal(result.namespaces.find(item => item.name === "reports").indexes[0].expire_after_seconds, "0");
assert.equal(result.namespaces.find(item => item.name === "reports").indexes[1].unique, true);
assert.deepEqual(result.migration_state, [{version: "36", dirty: false}]);
assert.deepEqual(execute({fail: true, expectFailure: true}), {error: "mongo_inventory_listCollections_code_none"});
for (const stage of ["connect", "auth", "listCollections", "collStats", "listIndexes", "migration"]) {
  assert.deepEqual(execute({throwStage: stage, code: 18, expectFailure: true}), {error: `mongo_inventory_${stage}_code_18`});
}
for (const stage of ["listCollections", "collStats", "listIndexes"]) {
  assert.deepEqual(execute({rejectStage: stage, code: 13, expectFailure: true}), {error: `mongo_inventory_${stage}_code_13`});
}
assert.deepEqual(execute({authFalse: true, expectFailure: true}), {error: "mongo_inventory_auth_code_none"});
assert.deepEqual(execute({noMigration: true, expectFailure: true}), {error: "mongo_inventory_migration_code_none"});
assert.deepEqual(execute({invalidHead: true, expectFailure: true}), {error: "mongo_inventory_head_code_none"});
for (const code of [-1, 2147483648, 1.5, "18", secret, {toString() {throw new Error(secret);}}, undefined]) {
  assert.deepEqual(execute({throwStage: "auth", code, expectFailure: true}), {error: "mongo_inventory_auth_code_none"});
}
console.log("Mongo metadata-only and output safety fixture OK");
