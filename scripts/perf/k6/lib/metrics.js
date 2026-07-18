import { Counter, Rate, Trend } from 'k6/metrics';
import {
  LEGACY_SUBMIT_RPS,
  MEDICAL_SUBMIT_RPS,
  PERSONALITY_SUBMIT_RPS,
  LEGACY_REPORT_RPS,
  MEDICAL_REPORT_RPS,
  PERSONALITY_REPORT_RPS,
  CHAIN_PROBE_MEDICAL_RPS,
  CHAIN_PROBE_PERSONALITY_RPS,
  LEGACY_QUERY_RPS,
  MEDICAL_QUERY_RPS,
  PERSONALITY_QUERY_RPS,
  QUESTIONNAIRE_DETAIL_RPS,
  PERSONALITY_QUESTIONNAIRE_DETAIL_RPS,
  STATS_RPS,
  STRICT_THRESHOLDS,
  DURATION,
  REPORT_MODE,
  REPORT_TIMEOUT,
  REPORT_POLL_INTERVAL_MS,
  REPORT_WS_HOLD_SECONDS,
} from './config.js';

export function buildEndpointFailureCounters(prefix) {
  return {
    status4xx: new Counter(`${prefix}_4xx`),
    status5xx: new Counter(`${prefix}_5xx`),
    transportError: new Counter(`${prefix}_transport_error`),
    timeout: new Counter(`${prefix}_timeout`),
  };
}

export const questionnaireQueryDuration = new Trend('questionnaire_query_duration', true);
export const personalityQuestionnaireQueryDuration = new Trend('personality_questionnaire_query_duration', true);
export const medicalModelQueryDuration = new Trend('medical_model_query_duration', true);
export const personalityModelQueryDuration = new Trend('personality_model_query_duration', true);
export const personalitySessionDuration = new Trend('personality_session_duration', true);
export const answerSubmitDuration = new Trend('answer_submit_duration', true);
export const reportStatusDuration = new Trend('report_status_duration', true);
export const medicalReportStatusDuration = new Trend('medical_report_status_duration', true);
export const personalityReportStatusDuration = new Trend('personality_report_status_duration', true);
export const statisticsDuration = new Trend('statistics_duration', true);
export const reportGeneratedLatency = new Trend('report_generated_latency', true);
export const medicalReportGeneratedLatency = new Trend('medical_report_generated_latency', true);
export const personalityReportGeneratedLatency = new Trend('personality_report_generated_latency', true);
export const submitToAssessmentLatency = new Trend('submit_to_assessment_latency', true);
export const assessmentToReportLatency = new Trend('assessment_to_report_latency', true);
export const personalityReportFetchDuration = new Trend('personality_report_fetch_duration', true);

export const answerSubmitAccepted = new Counter('answer_submit_accepted');
export const reportStatusPending = new Counter('report_status_pending');
export const reportStatusTerminal = new Counter('report_status_terminal');
export const chainProbeTerminal = new Counter('chain_probe_terminal');
export const chainProbeFailed = new Counter('chain_probe_failed');
export const questionnaireQueryFailed = new Counter('questionnaire_query_failed');
export const personalityQuestionnaireQueryFailed = new Counter('personality_questionnaire_query_failed');
export const medicalModelQueryFailed = new Counter('medical_model_query_failed');
export const personalityModelQueryFailed = new Counter('personality_model_query_failed');
export const personalitySessionFailed = new Counter('personality_session_failed');
export const answerSubmitFailed = new Counter('answer_submit_failed');
export const reportStatusFailed = new Counter('report_status_failed');
export const medicalReportStatusFailed = new Counter('medical_report_status_failed');
export const personalityReportStatusFailed = new Counter('personality_report_status_failed');
export const statisticsFailed = new Counter('statistics_failed');
export const setupDiscoveryFailed = new Counter('setup_discovery_failed');
export const http429Total = new Counter('http_429_total');
export const http401Total = new Counter('http_401_total');
export const http403Total = new Counter('http_403_total');
export const http4xxTotal = new Counter('http_4xx_total');
export const http5xxTotal = new Counter('http_5xx_total');
export const httpTransportErrorTotal = new Counter('http_transport_error_total');
export const httpTimeoutTotal = new Counter('http_timeout_total');

export const endpointFailureCounters = {
  questionnaire_query: buildEndpointFailureCounters('questionnaire_query'),
  personality_questionnaire_query: buildEndpointFailureCounters('personality_questionnaire_query'),
  answersheet_submit: buildEndpointFailureCounters('answer_submit'),
  report_status_query: buildEndpointFailureCounters('report_status'),
  statistics_query: buildEndpointFailureCounters('statistics'),
  statistics_overview: buildEndpointFailureCounters('statistics_overview'),
  statistics_content_batch: buildEndpointFailureCounters('statistics_content_batch'),
  chain_probe_submit: buildEndpointFailureCounters('chain_probe_submit'),
  chain_probe_assessment_readiness: buildEndpointFailureCounters('chain_probe_assessment_readiness'),
  chain_probe_report_status: buildEndpointFailureCounters('chain_probe_report_status'),
  discover_scale: buildEndpointFailureCounters('discover_scale'),
  discover_questionnaire: buildEndpointFailureCounters('discover_questionnaire'),
  discover_testees: buildEndpointFailureCounters('discover_testees'),
  discover_testees_fallback: buildEndpointFailureCounters('discover_testees_fallback'),
  discover_testees_no_source: buildEndpointFailureCounters('discover_testees_no_source'),
  discover_assessments: buildEndpointFailureCounters('discover_assessments'),
  discover_personality_models: buildEndpointFailureCounters('discover_personality_models'),
  discover_personality_model: buildEndpointFailureCounters('discover_personality_model'),
  discover_personality_session: buildEndpointFailureCounters('discover_personality_session'),
  discover_personality_assessments: buildEndpointFailureCounters('discover_personality_assessments'),
  discover_behavior_assessments: buildEndpointFailureCounters('discover_behavior_assessments'),
  personality_session: buildEndpointFailureCounters('personality_session'),
  medical_model_query: buildEndpointFailureCounters('medical_model_query'),
  personality_model_query: buildEndpointFailureCounters('personality_model_query'),
  medical_report_status_query: buildEndpointFailureCounters('medical_report_status'),
  personality_report_status_query: buildEndpointFailureCounters('personality_report_status'),
  chain_probe_personality_session: buildEndpointFailureCounters('chain_probe_personality_session'),
  chain_probe_behavior_assessment_lookup: buildEndpointFailureCounters('chain_probe_behavior_assessment_lookup'),
  chain_probe_personality_report_status: buildEndpointFailureCounters('chain_probe_personality_report_status'),
  chain_probe_personality_report: buildEndpointFailureCounters('chain_probe_personality_report'),
};

export const answerSubmitSuccessRate = new Rate('answer_submit_success_rate');
export const reportStatusSuccessRate = new Rate('report_status_success_rate');
export const personalityReportFetchSuccessRate = new Rate('personality_report_fetch_success_rate');

export const scenarios = {};

function reportDurationThresholds() {
  if (REPORT_MODE === 'long_poll') {
    const p95 = Math.max(1500, Math.floor(REPORT_TIMEOUT * 1000 * 0.95));
    return [`p(95)<${p95}`, `p(99)<${p95 * 2}`];
  }
  if (REPORT_MODE === 'short_poll') {
    const cycleMs = Math.max(500, REPORT_POLL_INTERVAL_MS) + 500;
    const p95 = Math.max(1500, cycleMs * 2);
    return [`p(95)<${p95}`, `p(99)<${p95 * 2}`];
  }
  const wsMs = Math.max(1000, Math.floor(REPORT_WS_HOLD_SECONDS * 1000));
  return [`p(95)<${wsMs + 500}`, `p(99)<${wsMs + 2000}`];
}

export function buildThresholds() {
  const submitRps = LEGACY_SUBMIT_RPS + MEDICAL_SUBMIT_RPS + PERSONALITY_SUBMIT_RPS;
  const reportRps = LEGACY_REPORT_RPS + MEDICAL_REPORT_RPS + PERSONALITY_REPORT_RPS;
  const chainProbeRps = CHAIN_PROBE_MEDICAL_RPS + CHAIN_PROBE_PERSONALITY_RPS;
  const queryRps = LEGACY_QUERY_RPS + MEDICAL_QUERY_RPS + PERSONALITY_QUERY_RPS + QUESTIONNAIRE_DETAIL_RPS + PERSONALITY_QUESTIONNAIRE_DETAIL_RPS;
  const thresholds = {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
    chain_probe_failed: ['count<3'],
  };
  if (submitRps > 0 || chainProbeRps > 0) {
    thresholds.answer_submit_success_rate = ['rate>0.99'];
  }
  if (reportRps > 0 || chainProbeRps > 0) {
    thresholds.report_status_success_rate = ['rate>0.99'];
  }
  if (chainProbeRps > 0) {
    thresholds.medical_report_generated_latency = ['p(95)<60000'];
    thresholds.personality_report_generated_latency = ['p(95)<90000'];
    thresholds.submit_to_assessment_latency = ['p(95)<15000'];
    thresholds.assessment_to_report_latency = ['p(95)<60000'];
    if (CHAIN_PROBE_PERSONALITY_RPS > 0) {
      thresholds.personality_report_fetch_success_rate = ['rate>0.99'];
    }
  }
  if (!STRICT_THRESHOLDS) {
    return thresholds;
  }
  if (queryRps > 0) {
    thresholds.questionnaire_query_duration = ['p(95)<500', 'p(99)<1200'];
  }
  if (submitRps > 0 || chainProbeRps > 0) {
    thresholds.answer_submit_duration = ['p(95)<500', 'p(99)<1000'];
  }
  if (reportRps > 0 || chainProbeRps > 0) {
    thresholds.report_status_duration = reportDurationThresholds();
  }
  if (STATS_RPS > 0) {
    thresholds.statistics_duration = ['p(95)<1000', 'p(99)<2000'];
  }
  return thresholds;
}

export function addScenario(name, exec, rate, preAllocatedVUs, maxVUs) {
  if (rate <= 0) {
    return;
  }
  scenarios[name] = arrivalScenario(exec, rate, preAllocatedVUs, maxVUs);
}

export function arrivalScenario(exec, rate, preAllocatedVUs, maxVUs) {
  return {
    executor: 'constant-arrival-rate',
    exec,
    rate: Math.max(1, Math.floor(rate)),
    timeUnit: '1s',
    duration: DURATION,
    preAllocatedVUs,
    maxVUs,
  };
}

export function lowRateArrivalScenario(exec, perSecondRate, preAllocatedVUs, maxVUs) {
  if (perSecondRate >= 1) {
    return arrivalScenario(exec, perSecondRate, preAllocatedVUs, maxVUs);
  }
  const secondsPerRequest = Math.max(1, Math.round(1 / perSecondRate));
  return {
    executor: 'constant-arrival-rate',
    exec,
    rate: 1,
    timeUnit: `${secondsPerRequest}s`,
    duration: DURATION,
    preAllocatedVUs,
    maxVUs,
  };
}
