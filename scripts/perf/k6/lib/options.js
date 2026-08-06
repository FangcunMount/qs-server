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
  BEHAVIOR_REPORT_RPS,
  PERSONALITY_REPORT_RPS,
  STATS_RPS,
  CHAIN_PROBE_MEDICAL_RPS,
  CHAIN_PROBE_PERSONALITY_RPS,
  intEnv,
  REPORT_MODE,
  resolveArrivalVuserDefaults,
  resolveReportVuserDefaults,
  CHAIN_PROBE_TIMEOUT_SECONDS,
} from './config.js';
import { addScenario, lowRateArrivalScenario, scenarios } from './metrics.js';

function arrivalVUs(rate, expectedLatencySeconds, options = {}) {
  return resolveArrivalVuserDefaults(rate, { expectedLatencySeconds, ...options });
}

function addArrival(name, exec, rate, envPrefix, expectedLatencySeconds) {
  const defaults = arrivalVUs(rate, expectedLatencySeconds);
  addScenario(
    name,
    exec,
    rate,
    intEnv(`${envPrefix}_VUS`, defaults.preAllocated),
    intEnv(`${envPrefix}_MAX_VUS`, defaults.max)
  );
}

function addReport(name, exec, rate, envPrefix) {
  const defaults = resolveReportVuserDefaults(rate);
  addScenario(
    name,
    exec,
    rate,
    intEnv(`${envPrefix}_VUS`, intEnv('REPORT_VUS', defaults.preAllocated)),
    intEnv(`${envPrefix}_MAX_VUS`, intEnv('REPORT_MAX_VUS', defaults.max))
  );
}

addArrival('medical_model_query', 'medicalModelQuery', MEDICAL_QUERY_RPS, 'MEDICAL_QUERY', 0.5);
addArrival('personality_model_query', 'personalityModelQuery', PERSONALITY_QUERY_RPS, 'PERSONALITY_QUERY', 0.5);
addArrival('questionnaire_query', 'questionnaireDetailQuery', QUESTIONNAIRE_DETAIL_RPS || LEGACY_QUERY_RPS, 'QUESTIONNAIRE_DETAIL', 0.5);
addArrival('personality_questionnaire_query', 'personalityQuestionnaireDetailQuery', PERSONALITY_QUESTIONNAIRE_DETAIL_RPS, 'PERSONALITY_QUESTIONNAIRE_QUERY', 0.5);
addArrival('personality_session', 'personalitySession', PERSONALITY_SESSION_RPS, 'PERSONALITY_SESSION', 0.5);
addArrival('answersheet_submit', 'answerSubmit', LEGACY_SUBMIT_RPS, 'SUBMIT', 0.8);
addArrival('medical_submit', 'medicalAnswerSubmit', MEDICAL_SUBMIT_RPS, 'MEDICAL_SUBMIT', 0.8);
addArrival('personality_submit', 'personalityAnswerSubmit', PERSONALITY_SUBMIT_RPS, 'PERSONALITY_SUBMIT', 0.8);

if (REPORT_MODE === 'websocket') {
  addReport('report_ws_query', 'reportWsQuery', LEGACY_REPORT_RPS, 'REPORT');
  addReport('medical_report_ws_query', 'medicalReportWsQuery', MEDICAL_REPORT_RPS, 'MEDICAL_REPORT');
  addReport('behavior_report_ws_query', 'behaviorReportWsQuery', BEHAVIOR_REPORT_RPS, 'BEHAVIOR_REPORT');
  addReport('personality_report_ws_query', 'personalityReportWsQuery', PERSONALITY_REPORT_RPS, 'PERSONALITY_REPORT');
} else {
  addReport('report_status_query', 'reportStatusQuery', LEGACY_REPORT_RPS, 'REPORT');
  addReport('medical_report_status_query', 'medicalReportStatusQuery', MEDICAL_REPORT_RPS, 'MEDICAL_REPORT');
  addReport('behavior_report_status_query', 'behaviorReportStatusQuery', BEHAVIOR_REPORT_RPS, 'BEHAVIOR_REPORT');
  addReport('personality_report_status_query', 'personalityReportStatusQuery', PERSONALITY_REPORT_RPS, 'PERSONALITY_REPORT');
}

addArrival('statistics_query', 'statisticsQuery', STATS_RPS, 'STATS', 1);

if (CHAIN_PROBE_MEDICAL_RPS > 0) {
  scenarios.async_chain_probe_medical = lowRateArrivalScenario(
    'asyncChainProbeMedical',
    CHAIN_PROBE_MEDICAL_RPS,
    intEnv('CHAIN_PROBE_VUS', arrivalVUs(CHAIN_PROBE_MEDICAL_RPS, 5, { timeoutSeconds: CHAIN_PROBE_TIMEOUT_SECONDS }).preAllocated),
    intEnv('CHAIN_PROBE_MAX_VUS', arrivalVUs(CHAIN_PROBE_MEDICAL_RPS, 5, { timeoutSeconds: CHAIN_PROBE_TIMEOUT_SECONDS }).max)
  );
}
if (CHAIN_PROBE_PERSONALITY_RPS > 0) {
  scenarios.async_chain_probe_personality = lowRateArrivalScenario(
    'asyncChainProbePersonality',
    CHAIN_PROBE_PERSONALITY_RPS,
    intEnv('CHAIN_PROBE_VUS', arrivalVUs(CHAIN_PROBE_PERSONALITY_RPS, 5, { timeoutSeconds: CHAIN_PROBE_TIMEOUT_SECONDS }).preAllocated),
    intEnv('CHAIN_PROBE_MAX_VUS', arrivalVUs(CHAIN_PROBE_PERSONALITY_RPS, 5, { timeoutSeconds: CHAIN_PROBE_TIMEOUT_SECONDS }).max)
  );
}

export { scenarios };
