// Reset every Testee-owned QS MongoDB document while preserving collection
// definitions, indexes, questionnaires, models, norms, and report templates.
// Run with mongosh from the qs-server repository root. Required environment
// variables are documented in README.md.

(function resetAllTesteeMongoFacts() {
  "use strict";

  const expectedConfirmation = "DELETE_ALL_TESTEE_DATA";
  const confirmation = process.env.QS_RESET_CONFIRM || "";
  const expectedDatabase = process.env.QS_RESET_EXPECTED_DATABASE || "";
  const expectedAnswerSheetCountRaw = process.env.QS_RESET_EXPECTED_ANSWERSHEET_COUNT;
  const batchSizeRaw = process.env.QS_RESET_BATCH_SIZE || "1000";
  const resumeRaw = process.env.QS_RESET_RESUME || "0";
  const batchSize = Number.parseInt(batchSizeRaw, 10);
  const resume = resumeRaw === "1";

  if (confirmation !== expectedConfirmation) {
    throw new Error("refusing Mongo reset: QS_RESET_CONFIRM is not DELETE_ALL_TESTEE_DATA");
  }
  if (!expectedDatabase || db.getName() !== expectedDatabase) {
    throw new Error(`refusing Mongo reset: connected database ${db.getName()} does not match QS_RESET_EXPECTED_DATABASE`);
  }
  if (expectedAnswerSheetCountRaw === undefined || expectedAnswerSheetCountRaw === "") {
    throw new Error("refusing Mongo reset: QS_RESET_EXPECTED_ANSWERSHEET_COUNT is required");
  }
  const expectedAnswerSheetCount = Number.parseInt(expectedAnswerSheetCountRaw, 10);
  if (!Number.isSafeInteger(expectedAnswerSheetCount) || expectedAnswerSheetCount < 0) {
    throw new Error("refusing Mongo reset: invalid QS_RESET_EXPECTED_ANSWERSHEET_COUNT");
  }
  if (!Number.isSafeInteger(batchSize) || batchSize < 100 || batchSize > 10000) {
    throw new Error("refusing Mongo reset: QS_RESET_BATCH_SIZE must be between 100 and 10000");
  }
  if (resumeRaw !== "0" && resumeRaw !== "1") {
    throw new Error("refusing Mongo reset: QS_RESET_RESUME must be 0 or 1");
  }

  const existingCollections = new Set(db.getCollectionNames());
  const hasCollection = (name) => existingCollections.has(name);

  const actualAnswerSheetCount = hasCollection("answersheets")
    ? db.getCollection("answersheets").countDocuments({})
    : 0;
  if (!resume && actualAnswerSheetCount !== expectedAnswerSheetCount) {
    throw new Error(
      `refusing Mongo reset: answersheet count changed after preflight; expected=${expectedAnswerSheetCount} actual=${actualAnswerSheetCount}`,
    );
  }
  if (resume && actualAnswerSheetCount > expectedAnswerSheetCount) {
    throw new Error(
      `refusing Mongo reset: resume answersheet count exceeds original scope; expected<=${expectedAnswerSheetCount} actual=${actualAnswerSheetCount}`,
    );
  }

  const protectedCollections = [
    "questionnaires",
    "scales",
    "assessment_models",
    "published_assessment_models",
    "assessment_norms",
    "evaluation_rule_sets",
    "interpretation_report_templates",
    "schema_migrations",
  ];
  const protectedBaseline = Object.fromEntries(
    protectedCollections.map((name) => [
      name,
      hasCollection(name) ? db.getCollection(name).countDocuments({}) : null,
    ]),
  );

  const testeeEventTypes = [
    "answersheet.submitted",
    "evaluation.requested",
    "evaluation.retry.requested",
    "evaluation.outcome.committed",
    "evaluation.failed",
    "interpretation.report.generated",
    "interpretation.report.failed",
    "interpretation.retry.requested",
    "task.opened",
    "task.completed",
    "task.expired",
    "task.canceled",
  ];

  const fullResetCollections = [
    "answersheets",
    "answersheet_submit_idempotency",
    "report_generations",
    "interpretation_runs",
    "interpret_report_artifacts",
    "report_query_catalog",
    "archived_reports",
    "interpretation_admission_failures",
    "interpretation_attention_projections",
    "interpretation_catalog_repair_plans",
    "interpret_reports",
  ];

  const unfinishedOutboxCount = hasCollection("domain_event_outbox")
    ? db.getCollection("domain_event_outbox").countDocuments({
      event_type: { $in: testeeEventTypes },
      status: { $ne: "published" },
    })
    : 0;
  if (unfinishedOutboxCount !== 0) {
    throw new Error(`refusing Mongo reset: Testee Outbox has ${unfinishedOutboxCount} unfinished rows`);
  }

  function deleteCollectionInBatches(name, filter = {}) {
    if (!hasCollection(name)) {
      return { collection: name, existed: false, deleted: 0 };
    }
    const collection = db.getCollection(name);
    let total = 0;
    while (true) {
      const ids = collection.find(filter, { _id: 1 }).limit(batchSize).toArray().map((doc) => doc._id);
      if (ids.length === 0) {
        break;
      }
      const result = collection.deleteMany({ _id: { $in: ids } });
      if (!result.acknowledged || result.deletedCount === 0) {
        throw new Error(`Mongo reset made no progress while deleting ${name}`);
      }
      total += result.deletedCount;
    }
    return { collection: name, existed: true, deleted: total };
  }

  print(EJSON.stringify({
    phase: "preflight",
    database: db.getName(),
    answersheets: actualAnswerSheetCount,
    protected: protectedBaseline,
    batchSize,
    resume,
    unfinishedOutboxCount,
  }, null, 2));

  const results = fullResetCollections.map((name) => deleteCollectionInBatches(name));
  results.push(deleteCollectionInBatches("domain_event_outbox", { event_type: { $in: testeeEventTypes } }));

  const remaining = {};
  for (const name of fullResetCollections) {
    remaining[name] = hasCollection(name) ? db.getCollection(name).countDocuments({}) : 0;
  }
  remaining.domain_event_outbox = hasCollection("domain_event_outbox")
    ? db.getCollection("domain_event_outbox").countDocuments({ event_type: { $in: testeeEventTypes } })
    : 0;

  const nonZero = Object.entries(remaining).filter(([, count]) => count !== 0);
  if (nonZero.length > 0) {
    throw new Error(`Mongo reset postcheck failed: Testee documents remain: ${EJSON.stringify(Object.fromEntries(nonZero))}`);
  }

  for (const [name, before] of Object.entries(protectedBaseline)) {
    const after = hasCollection(name) ? db.getCollection(name).countDocuments({}) : null;
    if (after !== before) {
      throw new Error(`Mongo reset postcheck failed: protected collection ${name} changed from ${before} to ${after}`);
    }
  }

  print(EJSON.stringify({
    phase: "completed",
    database: db.getName(),
    deleted: results,
    remaining,
    protected: protectedBaseline,
  }, null, 2));
})();
