import { Counter, Rate, Trend } from 'k6/metrics';
import {
  LEGACY_SUBMIT_RPS,
  MEDICAL_SUBMIT_RPS,
  PERSONALITY_SUBMIT_RPS,
  LEGACY_REPORT_RPS,
  MEDICAL_REPORT_RPS,
  BEHAVIOR_REPORT_RPS,
  PERSONALITY_REPORT_RPS,
  CHAIN_PROBE_MEDICAL_RPS,
  CHAIN_PROBE_PERSONALITY_RPS,
  LEGACY_QUERY_RPS,
  MEDICAL_QUERY_RPS,
  PERSONALITY_QUERY_RPS,
  QUESTIONNAIRE_DETAIL_RPS,
  PERSONALITY_QUESTIONNAIRE_DETAIL_RPS,
  PERSONALITY_SESSION_RPS,
  STATS_RPS,
  THRESHOLD_TIER,
  DURATION,
  REPORT_MODE,
  REPORT_TIMEOUT,
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
export const medicalAnswerSubmitDuration = new Trend('medical_answer_submit_duration', true);
export const personalityAnswerSubmitDuration = new Trend('personality_answer_submit_duration', true);
export const reportStatusDuration = new Trend('report_status_duration', true);
export const medicalReportStatusDuration = new Trend('medical_report_status_duration', true);
export const behaviorReportStatusDuration = new Trend('behavior_report_status_duration', true);
export const personalityReportStatusDuration = new Trend('personality_report_status_duration', true);
export const statisticsDuration = new Trend('statistics_duration', true);
export const statisticsOverviewDuration = new Trend('statistics_overview_duration', true);
export const statisticsContentBatchDuration = new Trend('statistics_content_batch_duration', true);
export const reportWsConnectDuration = new Trend('report_ws_connect_duration', true);
export const reportWsFirstMessageLatency = new Trend('report_ws_first_message_latency', true);
export const reportWsSessionDuration = new Trend('report_ws_session_duration', true);
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
export const chainProbeStarted = new Counter('chain_probe_started');
export const chainProbeAccepted = new Counter('chain_probe_accepted');
export const chainProbeCompleted = new Counter('chain_probe_completed');
export const chainProbeFinalFailed = new Counter('chain_probe_final_failed');
export const chainProbeTimeout = new Counter('chain_probe_timeout');
export const chainProbePollRequests = new Counter('chain_probe_poll_requests');
export const questionnaireQueryFailed = new Counter('questionnaire_query_failed');
export const personalityQuestionnaireQueryFailed = new Counter('personality_questionnaire_query_failed');
export const medicalModelQueryFailed = new Counter('medical_model_query_failed');
export const personalityModelQueryFailed = new Counter('personality_model_query_failed');
export const personalitySessionFailed = new Counter('personality_session_failed');
export const answerSubmitFailed = new Counter('answer_submit_failed');
export const reportStatusFailed = new Counter('report_status_failed');
export const reportSampleSkipped = new Counter('report_sample_skipped');
export const medicalReportStatusFailed = new Counter('medical_report_status_failed');
export const behaviorReportStatusFailed = new Counter('behavior_report_status_failed');
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
export const reportWsTimeoutTotal = new Counter('report_ws_timeout_total');

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
  behavior_report_status_query: buildEndpointFailureCounters('behavior_report_status'),
  personality_report_status_query: buildEndpointFailureCounters('personality_report_status'),
  chain_probe_personality_session: buildEndpointFailureCounters('chain_probe_personality_session'),
  chain_probe_behavior_assessment_lookup: buildEndpointFailureCounters('chain_probe_behavior_assessment_lookup'),
  chain_probe_personality_report_status: buildEndpointFailureCounters('chain_probe_personality_report_status'),
  chain_probe_personality_report: buildEndpointFailureCounters('chain_probe_personality_report'),
};

export const answerSubmitSuccessRate = new Rate('answer_submit_success_rate');
export const medicalAnswerSubmitSuccessRate = new Rate('medical_answer_submit_success_rate');
export const personalityAnswerSubmitSuccessRate = new Rate('personality_answer_submit_success_rate');
export const reportStatusSuccessRate = new Rate('report_status_success_rate');
export const medicalReportStatusSuccessRate = new Rate('medical_report_status_success_rate');
export const behaviorReportStatusSuccessRate = new Rate('behavior_report_status_success_rate');
export const personalityReportStatusSuccessRate = new Rate('personality_report_status_success_rate');
export const reportWsConnectSuccessRate = new Rate('report_ws_connect_success_rate');
export const reportWsMessageSuccessRate = new Rate('report_ws_message_success_rate');
export const statisticsOverviewSuccessRate = new Rate('statistics_overview_success_rate');
export const statisticsContentBatchSuccessRate = new Rate('statistics_content_batch_success_rate');
export const personalityReportFetchSuccessRate = new Rate('personality_report_fetch_success_rate');
export const httpTimeoutRate = new Rate('http_timeout_rate');

export const scenarios = {};

const THRESHOLD_LIMITS = {
  experience: {
    query: [200, 500],
    submit: [300, 800],
    reportStatus: [300, 800],
    statistics: [700, 1500],
    wsConnect: [300, 800],
    wsFirstMessage: [300, 800],
    reportFetch: [500, 1000],
  },
  protection: {
    query: [500, 1200],
    submit: [500, 1000],
    reportStatus: [500, 1200],
    statistics: [1000, 2000],
    wsConnect: [500, 1200],
    wsFirstMessage: [500, 1200],
    reportFetch: [1000, 2000],
  },
};

function latencyThresholds(limits) {
  return [`p(95)<${limits[0]}`, `p(99)<${limits[1]}`];
}

function addLatencyThreshold(thresholds, metric, active, limits) {
  if (active) {
    thresholds[metric] = latencyThresholds(limits);
  }
}

function addEndpointErrorRate(thresholds, endpoint, active, extraTags) {
  if (!active) {
    return;
  }
  const tags = [`endpoint:${endpoint}`].concat(extraTags || []);
  thresholds[`http_req_failed{${tags.join(',')}}`] = ['rate<0.01'];
}

function reportDurationThresholds(limits) {
  if (REPORT_MODE === 'long_poll') {
    const p95 = Math.max(1500, Math.floor(REPORT_TIMEOUT * 1000 * 0.95));
    return [`p(95)<${p95}`, `p(99)<${p95 * 2}`];
  }
  return latencyThresholds(limits.reportStatus);
}

export function buildThresholds() {
  const submitRps = LEGACY_SUBMIT_RPS + MEDICAL_SUBMIT_RPS + PERSONALITY_SUBMIT_RPS;
  const reportRps = LEGACY_REPORT_RPS + MEDICAL_REPORT_RPS + BEHAVIOR_REPORT_RPS + PERSONALITY_REPORT_RPS;
  const chainProbeRps = CHAIN_PROBE_MEDICAL_RPS + CHAIN_PROBE_PERSONALITY_RPS;
  const thresholds = {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
    chain_probe_failed: ['count<3'],
  };

  addEndpointErrorRate(thresholds, 'questionnaire_query', LEGACY_QUERY_RPS > 0 || QUESTIONNAIRE_DETAIL_RPS > 0);
  addEndpointErrorRate(thresholds, 'personality_questionnaire_query', PERSONALITY_QUESTIONNAIRE_DETAIL_RPS > 0);
  addEndpointErrorRate(thresholds, 'medical_model_query', MEDICAL_QUERY_RPS > 0);
  addEndpointErrorRate(thresholds, 'personality_model_query', PERSONALITY_QUERY_RPS > 0);
  addEndpointErrorRate(thresholds, 'personality_session', PERSONALITY_SESSION_RPS > 0);
  addEndpointErrorRate(thresholds, 'answersheet_submit', LEGACY_SUBMIT_RPS > 0);
  addEndpointErrorRate(thresholds, 'answersheet_submit', MEDICAL_SUBMIT_RPS > 0, ['model_type:medical']);
  addEndpointErrorRate(thresholds, 'answersheet_submit', PERSONALITY_SUBMIT_RPS > 0, ['model_type:personality']);
  addEndpointErrorRate(thresholds, 'report_status_query', REPORT_MODE !== 'websocket' && LEGACY_REPORT_RPS > 0);
  addEndpointErrorRate(thresholds, 'medical_report_status_query', REPORT_MODE !== 'websocket' && MEDICAL_REPORT_RPS > 0);
  addEndpointErrorRate(thresholds, 'behavior_report_status_query', REPORT_MODE !== 'websocket' && BEHAVIOR_REPORT_RPS > 0);
  addEndpointErrorRate(thresholds, 'personality_report_status_query', REPORT_MODE !== 'websocket' && PERSONALITY_REPORT_RPS > 0);
  addEndpointErrorRate(thresholds, 'statistics_overview', STATS_RPS > 0);
  addEndpointErrorRate(thresholds, 'statistics_content_batch', STATS_RPS > 0);

  if (submitRps > 0) {
    thresholds.answer_submit_success_rate = ['rate>0.99'];
    if (MEDICAL_SUBMIT_RPS > 0) {
      thresholds.medical_answer_submit_success_rate = ['rate>0.99'];
    }
    if (PERSONALITY_SUBMIT_RPS > 0) {
      thresholds.personality_answer_submit_success_rate = ['rate>0.99'];
    }
  }
  if (reportRps > 0) {
    thresholds.report_sample_skipped = ['count==0'];
  }
  if (REPORT_MODE === 'websocket' && reportRps > 0) {
    thresholds.report_ws_connect_success_rate = ['rate>0.99'];
    thresholds.report_ws_message_success_rate = ['rate>0.99'];
    if (MEDICAL_REPORT_RPS > 0) {
      thresholds['report_ws_connect_success_rate{model_type:medical}'] = ['rate>0.99'];
      thresholds['report_ws_message_success_rate{model_type:medical}'] = ['rate>0.99'];
    }
    if (BEHAVIOR_REPORT_RPS > 0) {
      thresholds['report_ws_connect_success_rate{model_type:behavior}'] = ['rate>0.99'];
      thresholds['report_ws_message_success_rate{model_type:behavior}'] = ['rate>0.99'];
    }
    if (PERSONALITY_REPORT_RPS > 0) {
      thresholds['report_ws_connect_success_rate{model_type:personality}'] = ['rate>0.99'];
      thresholds['report_ws_message_success_rate{model_type:personality}'] = ['rate>0.99'];
    }
  }
  if (REPORT_MODE !== 'websocket' && reportRps > 0) {
    thresholds.report_status_success_rate = ['rate>0.99'];
    if (MEDICAL_REPORT_RPS > 0) {
      thresholds.medical_report_status_success_rate = ['rate>0.99'];
    }
    if (BEHAVIOR_REPORT_RPS > 0) {
      thresholds.behavior_report_status_success_rate = ['rate>0.99'];
    }
    if (PERSONALITY_REPORT_RPS > 0) {
      thresholds.personality_report_status_success_rate = ['rate>0.99'];
    }
  }
  if (STATS_RPS > 0) {
    thresholds.statistics_overview_success_rate = ['rate>0.99'];
    thresholds.statistics_content_batch_success_rate = ['rate>0.99'];
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
  if (THRESHOLD_TIER === 'none') {
    return thresholds;
  }
  thresholds.dropped_iterations = ['count==0'];
  thresholds.http_timeout_rate = ['rate<0.001'];
  if (submitRps > 0) {
    thresholds.answer_submit_timeout = ['count==0'];
    if (MEDICAL_SUBMIT_RPS > 0) {
      thresholds['answer_submit_timeout{model_type:medical}'] = ['count==0'];
    }
    if (PERSONALITY_SUBMIT_RPS > 0) {
      thresholds['answer_submit_timeout{model_type:personality}'] = ['count==0'];
    }
  }
  if (chainProbeRps > 0) {
    thresholds.chain_probe_timeout = ['count==0'];
  }
  if (REPORT_MODE === 'websocket' && reportRps > 0) {
    thresholds.report_ws_timeout_total = ['count==0'];
    if (MEDICAL_REPORT_RPS > 0) {
      thresholds['report_ws_timeout_total{model_type:medical}'] = ['count==0'];
    }
    if (BEHAVIOR_REPORT_RPS > 0) {
      thresholds['report_ws_timeout_total{model_type:behavior}'] = ['count==0'];
    }
    if (PERSONALITY_REPORT_RPS > 0) {
      thresholds['report_ws_timeout_total{model_type:personality}'] = ['count==0'];
    }
  }
  const limits = THRESHOLD_LIMITS[THRESHOLD_TIER];
  addLatencyThreshold(thresholds, 'questionnaire_query_duration', LEGACY_QUERY_RPS > 0 || QUESTIONNAIRE_DETAIL_RPS > 0, limits.query);
  addLatencyThreshold(thresholds, 'personality_questionnaire_query_duration', PERSONALITY_QUESTIONNAIRE_DETAIL_RPS > 0, limits.query);
  addLatencyThreshold(thresholds, 'medical_model_query_duration', MEDICAL_QUERY_RPS > 0, limits.query);
  addLatencyThreshold(thresholds, 'personality_model_query_duration', PERSONALITY_QUERY_RPS > 0, limits.query);
  addLatencyThreshold(thresholds, 'personality_session_duration', PERSONALITY_SESSION_RPS > 0, limits.submit);
  if (submitRps > 0) {
    thresholds.answer_submit_duration = latencyThresholds(limits.submit);
    addLatencyThreshold(thresholds, 'medical_answer_submit_duration', MEDICAL_SUBMIT_RPS > 0, limits.submit);
    addLatencyThreshold(thresholds, 'personality_answer_submit_duration', PERSONALITY_SUBMIT_RPS > 0, limits.submit);
  }
  if (REPORT_MODE === 'websocket' && reportRps > 0) {
    thresholds.report_ws_connect_duration = latencyThresholds(limits.wsConnect);
    thresholds.report_ws_first_message_latency = latencyThresholds(limits.wsFirstMessage);
    addLatencyThreshold(thresholds, 'report_ws_connect_duration{model_type:medical}', MEDICAL_REPORT_RPS > 0, limits.wsConnect);
    addLatencyThreshold(thresholds, 'report_ws_first_message_latency{model_type:medical}', MEDICAL_REPORT_RPS > 0, limits.wsFirstMessage);
    addLatencyThreshold(thresholds, 'report_ws_connect_duration{model_type:behavior}', BEHAVIOR_REPORT_RPS > 0, limits.wsConnect);
    addLatencyThreshold(thresholds, 'report_ws_first_message_latency{model_type:behavior}', BEHAVIOR_REPORT_RPS > 0, limits.wsFirstMessage);
    addLatencyThreshold(thresholds, 'report_ws_connect_duration{model_type:personality}', PERSONALITY_REPORT_RPS > 0, limits.wsConnect);
    addLatencyThreshold(thresholds, 'report_ws_first_message_latency{model_type:personality}', PERSONALITY_REPORT_RPS > 0, limits.wsFirstMessage);
  }
  if (REPORT_MODE !== 'websocket' && reportRps > 0) {
    const reportThresholds = reportDurationThresholds(limits);
    thresholds.report_status_duration = reportThresholds;
    if (MEDICAL_REPORT_RPS > 0) {
      thresholds.medical_report_status_duration = reportThresholds;
    }
    if (BEHAVIOR_REPORT_RPS > 0) {
      thresholds.behavior_report_status_duration = reportThresholds;
    }
    if (PERSONALITY_REPORT_RPS > 0) {
      thresholds.personality_report_status_duration = reportThresholds;
    }
  }
  if (STATS_RPS > 0) {
    thresholds.statistics_duration = latencyThresholds(limits.statistics);
    thresholds.statistics_overview_duration = latencyThresholds(limits.statistics);
    thresholds.statistics_content_batch_duration = latencyThresholds(limits.statistics);
  }
  if (CHAIN_PROBE_PERSONALITY_RPS > 0) {
    thresholds.personality_report_fetch_duration = latencyThresholds(limits.reportFetch);
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
