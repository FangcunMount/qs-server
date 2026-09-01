const runID = String(process.env.REVIEW_RUN_ID || '').trim()
const databaseName = String(process.env.MONGO_DB || '').trim()

if (!/^[1-9][0-9]{0,19}$/.test(runID)) {
  throw new Error('REVIEW_RUN_ID must be an unsigned decimal domain id')
}
if (!databaseName) {
  throw new Error('MONGO_DB is required')
}

const targetDB = db.getSiblingDB(databaseName)
const evidence = targetDB.ai_explanation_prompt_evaluations.findOne(
  { domain_id: Long.fromString(runID), evidence_version: 'v2' },
  {
    _id: 0,
    domain_id: 1,
    status: 1,
    version: 1,
    slots: 1,
    generation_executions: 1,
    semantic_executions: 1,
    human_reviews: 1,
  },
)

if (!evidence) {
  throw new Error(`v2 Prompt evaluation Run ${runID} was not found`)
}

print(EJSON.stringify(evidence, null, 2, { relaxed: false }))
