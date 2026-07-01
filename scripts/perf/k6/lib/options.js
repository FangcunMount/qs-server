import {
  MEDICAL_QUERY_RPS,
  PERSONALITY_QUERY_RPS,
  QUESTIONNAIRE_DETAIL_RPS,
  PERSONALITY_QUESTIONNAIRE_DETAIL_RPS,
  LEGACY_QUERY_RPS,
  PERSONALITY_SESSION_RPS,
  LEGACY_SUBMIT_RPS,
  MEDICAL_SUBMIT_RPS,
  PERSONALITY_SUBMIT_RPS,
  LEGACY_REPORT_RPS,
  MEDICAL_REPORT_RPS,
  PERSONALITY_REPORT_RPS,
  STATS_RPS,
  CHAIN_PROBE_MEDICAL_RPS,
  CHAIN_PROBE_PERSONALITY_RPS,
  intEnv,
  REPORT_MODE,
  REPORT_VUSER_DEFAULTS,
} from './config.js';
import { addScenario, lowRateArrivalScenario, scenarios } from './metrics.js';

const reportPreAllocatedVUs = intEnv('REPORT_VUS', REPORT_VUSER_DEFAULTS.preAllocated);
const reportMaxVUs = intEnv('REPORT_MAX_VUS', REPORT_VUSER_DEFAULTS.max);

addScenario('medical_model_query', 'medicalModelQuery', MEDICAL_QUERY_RPS, intEnv('MEDICAL_QUERY_VUS', intEnv('QUERY_VUS', 80)), intEnv('MEDICAL_QUERY_MAX_VUS', intEnv('QUERY_MAX_VUS', 400)));
addScenario('personality_model_query', 'personalityModelQuery', PERSONALITY_QUERY_RPS, intEnv('PERSONALITY_QUERY_VUS', intEnv('QUERY_VUS', 80)), intEnv('PERSONALITY_QUERY_MAX_VUS', intEnv('QUERY_MAX_VUS', 400)));
addScenario('questionnaire_query', 'questionnaireDetailQuery', QUESTIONNAIRE_DETAIL_RPS || LEGACY_QUERY_RPS, intEnv('QUESTIONNAIRE_DETAIL_VUS', intEnv('QUERY_VUS', 80)), intEnv('QUESTIONNAIRE_DETAIL_MAX_VUS', intEnv('QUERY_MAX_VUS', 400)));
addScenario('personality_questionnaire_query', 'personalityQuestionnaireDetailQuery', PERSONALITY_QUESTIONNAIRE_DETAIL_RPS, intEnv('PERSONALITY_QUESTIONNAIRE_QUERY_VUS', intEnv('QUERY_VUS', 80)), intEnv('PERSONALITY_QUESTIONNAIRE_QUERY_MAX_VUS', intEnv('QUERY_MAX_VUS', 400)));
addScenario('personality_session', 'personalitySession', PERSONALITY_SESSION_RPS, intEnv('PERSONALITY_SESSION_VUS', 40), intEnv('PERSONALITY_SESSION_MAX_VUS', 200));
addScenario('answersheet_submit', 'answerSubmit', LEGACY_SUBMIT_RPS, intEnv('SUBMIT_VUS', 120), intEnv('SUBMIT_MAX_VUS', 800));
addScenario('medical_submit', 'medicalAnswerSubmit', MEDICAL_SUBMIT_RPS, intEnv('MEDICAL_SUBMIT_VUS', intEnv('SUBMIT_VUS', 120)), intEnv('MEDICAL_SUBMIT_MAX_VUS', intEnv('SUBMIT_MAX_VUS', 800)));
addScenario('personality_submit', 'personalityAnswerSubmit', PERSONALITY_SUBMIT_RPS, intEnv('PERSONALITY_SUBMIT_VUS', intEnv('SUBMIT_VUS', 120)), intEnv('PERSONALITY_SUBMIT_MAX_VUS', intEnv('SUBMIT_MAX_VUS', 800)));

if (REPORT_MODE === 'websocket') {
  addScenario('report_ws_query', 'reportWsQuery', LEGACY_REPORT_RPS, reportPreAllocatedVUs, reportMaxVUs);
  addScenario('medical_report_ws_query', 'medicalReportWsQuery', MEDICAL_REPORT_RPS, intEnv('MEDICAL_REPORT_VUS', reportPreAllocatedVUs), intEnv('MEDICAL_REPORT_MAX_VUS', reportMaxVUs));
  addScenario('personality_report_ws_query', 'personalityReportWsQuery', PERSONALITY_REPORT_RPS, intEnv('PERSONALITY_REPORT_VUS', reportPreAllocatedVUs), intEnv('PERSONALITY_REPORT_MAX_VUS', reportMaxVUs));
} else {
  addScenario('report_status_query', 'reportStatusQuery', LEGACY_REPORT_RPS, reportPreAllocatedVUs, reportMaxVUs);
  addScenario('medical_report_status_query', 'medicalReportStatusQuery', MEDICAL_REPORT_RPS, intEnv('MEDICAL_REPORT_VUS', reportPreAllocatedVUs), intEnv('MEDICAL_REPORT_MAX_VUS', reportMaxVUs));
  addScenario('personality_report_status_query', 'personalityReportStatusQuery', PERSONALITY_REPORT_RPS, intEnv('PERSONALITY_REPORT_VUS', reportPreAllocatedVUs), intEnv('PERSONALITY_REPORT_MAX_VUS', reportMaxVUs));
}

addScenario('statistics_query', 'statisticsQuery', STATS_RPS, intEnv('STATS_VUS', 60), intEnv('STATS_MAX_VUS', 300));

if (CHAIN_PROBE_MEDICAL_RPS > 0) {
  scenarios.async_chain_probe_medical = lowRateArrivalScenario(
    'asyncChainProbeMedical',
    CHAIN_PROBE_MEDICAL_RPS,
    intEnv('CHAIN_PROBE_VUS', 20),
    intEnv('CHAIN_PROBE_MAX_VUS', 200)
  );
}
if (CHAIN_PROBE_PERSONALITY_RPS > 0) {
  scenarios.async_chain_probe_personality = lowRateArrivalScenario(
    'asyncChainProbePersonality',
    CHAIN_PROBE_PERSONALITY_RPS,
    intEnv('CHAIN_PROBE_VUS', 20),
    intEnv('CHAIN_PROBE_MAX_VUS', 200)
  );
}

export { scenarios };
