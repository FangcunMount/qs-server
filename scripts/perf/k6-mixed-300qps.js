import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const PERF_CONFIG_PATH = __ENV.PERF_CONFIG_FILE || __ENV.K6_CONFIG_FILE || '';
let PERF_CONFIG_DIR = '';
const PERF_CONFIG = loadPerfConfig();
const TOKEN_FILE_READ_ISSUES = [];
const TOKEN_FILE_LOADS = [];
const QPS_PROFILE = resolveQpsProfileName();
const QPS_PROFILE_CONFIG = resolveQpsProfileConfig(QPS_PROFILE);
const DEBUG_SETUP = boolEnv('DEBUG_SETUP', false);

const COLLECTION_BASE_URL = normalizeBaseURL(
  __ENV.COLLECTION_BASE_URL ||
    __ENV.BASE_URL ||
    configStringValue(['collectionBaseUrl', 'collection_base_url', 'collection.baseUrl', 'collection.base_url'], 'http://127.0.0.1:18083')
);
const APISERVER_BASE_URL = normalizeBaseURL(
  __ENV.APISERVER_BASE_URL ||
    configStringValue(['apiserverBaseUrl', 'apiserver_base_url', 'apiserver.baseUrl', 'apiserver.base_url'], 'http://127.0.0.1:18082')
);
const SEEDDATA_SCALE_CODES = '3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35';
const SEEDDATA_PLAN_IDS = '614333603412718126,614187067651404334';

const TOKEN = envOrConfigString('TOKEN', ['token'], '');
const COLLECTION_TOKEN = envOrConfigString('COLLECTION_TOKEN', ['collectionToken', 'collection_token', 'collection.token'], TOKEN);
const APISERVER_TOKEN = envOrConfigString('APISERVER_TOKEN', ['apiserverToken', 'apiserver_token', 'apiserver.token'], TOKEN);
const TOKENS_FILE = envOrConfigString('TOKENS_FILE', ['tokensFile', 'tokens_file'], '');
const COLLECTION_TOKENS_FILE = envOrConfigString('COLLECTION_TOKENS_FILE', ['collectionTokensFile', 'collection_tokens_file', 'collection.tokensFile', 'collection.tokens_file'], '');
const APISERVER_TOKENS_FILE = envOrConfigString('APISERVER_TOKENS_FILE', ['apiserverTokensFile', 'apiserver_tokens_file', 'apiserver.tokensFile', 'apiserver.tokens_file'], '');
const COMMON_TOKENS = uniqueList(
  envOrConfigList('TOKENS', ['tokens'], TOKEN).concat(listTokenFilePath('tokensFile', TOKENS_FILE))
);
const COLLECTION_SPECIFIC_TOKENS = uniqueList(
  envOrConfigList('COLLECTION_TOKENS', ['collectionTokens', 'collection_tokens', 'collection.tokens'], COLLECTION_TOKEN).concat(
    listTokenFilePath('collectionTokensFile', COLLECTION_TOKENS_FILE)
  )
);
const APISERVER_SPECIFIC_TOKENS = uniqueList(
  envOrConfigList('APISERVER_TOKENS', ['apiserverTokens', 'apiserver_tokens', 'apiserver.tokens'], APISERVER_TOKEN).concat(
    listTokenFilePath('apiserverTokensFile', APISERVER_TOKENS_FILE)
  )
);
const COLLECTION_TOKENS = COLLECTION_SPECIFIC_TOKENS.length > 0 ? COLLECTION_SPECIFIC_TOKENS : COMMON_TOKENS;
const APISERVER_TOKENS = APISERVER_SPECIFIC_TOKENS.length > 0 ? APISERVER_SPECIFIC_TOKENS : COMMON_TOKENS;

const DURATION = envOrConfigString('DURATION', ['duration'], '10m');
const QUERY_RPS = intEnv('QUERY_RPS', 120);
const SUBMIT_RPS = intEnv('SUBMIT_RPS', 60);
const REPORT_RPS = intEnv('REPORT_RPS', 90);
const STATS_RPS = intEnv('STATS_RPS', 30);
const CHAIN_PROBE_RPS = numberEnv('CHAIN_PROBE_RPS', 0);

const QUERY_PATHS = envOrConfigList(
  'QUESTIONNAIRE_QUERY_PATHS',
  ['questionnaireQueryPaths', 'questionnaire_query_paths', 'paths.questionnaireQuery', 'paths.questionnaire_query'],
  '/api/v1/scales?page=1&page_size=20&status=published,/api/v1/scales/categories,/api/v1/scales/hot?limit=5,/api/v1/scales/{scale_code},/api/v1/questionnaires/{questionnaire_code}'
);
const STATS_PATHS = envOrConfigList(
  'STATISTICS_PATHS',
  ['statisticsPaths', 'statistics_paths', 'paths.statistics'],
  '/api/v1/statistics/overview?preset=7d,/api/v1/statistics/system,/api/v1/statistics/questionnaires/{questionnaire_code}?preset=7d'
);

const SUBMIT_PATH = envOrConfigString('SUBMIT_PATH', ['submitPath', 'submit_path', 'paths.submit'], '/api/v1/answersheets');
const REPORT_STATUS_PATH = envOrConfigString(
  'REPORT_STATUS_PATH',
  ['reportStatusPath', 'report_status_path', 'paths.reportStatus', 'paths.report_status'],
  '/api/v1/assessments/{assessment_id}/wait-report?testee_id={testee_id}&timeout={report_timeout}'
);
const SUBMIT_STATUS_PATH = envOrConfigString(
  'SUBMIT_STATUS_PATH',
  ['submitStatusPath', 'submit_status_path', 'paths.submitStatus', 'paths.submit_status'],
  '/api/v1/answersheets/submit-status?request_id={request_id}'
);
const ANSWERSHEET_ASSESSMENT_PATH = envOrConfigString(
  'ANSWERSHEET_ASSESSMENT_PATH',
  ['answersheetAssessmentPath', 'answersheet_assessment_path', 'paths.answersheetAssessment', 'paths.answersheet_assessment'],
  '/api/v1/answersheets/{answersheet_id}/assessment'
);

const TESTEE_IDS = envOrConfigList('TESTEE_IDS', ['testeeIds', 'testee_ids'], __ENV.TESTEE_ID || '');
const ASSESSMENT_IDS = envOrConfigList('ASSESSMENT_IDS', ['assessmentIds', 'assessment_ids'], __ENV.ASSESSMENT_ID || '');
const QUESTIONNAIRE_CODES = envOrConfigList('QUESTIONNAIRE_CODES', ['questionnaireCodes', 'questionnaire_codes'], __ENV.QUESTIONNAIRE_CODE || __ENV.Q_CODE || '');
const QUESTIONNAIRE_VERSION = __ENV.QUESTIONNAIRE_VERSION || __ENV.Q_VER || configStringValue(['questionnaireVersion', 'questionnaire_version'], '');
const SCALE_CODES = envOrConfigList('SCALE_CODES', ['scaleCodes', 'scale_codes'], __ENV.SCALE_CODE || SEEDDATA_SCALE_CODES);
const PLAN_IDS = envOrConfigList('PLAN_IDS', ['planIds', 'plan_ids'], __ENV.PLAN_ID || SEEDDATA_PLAN_IDS);
const ENTRY_IDS = envOrConfigList('ENTRY_IDS', ['entryIds', 'entry_ids'], __ENV.ENTRY_ID || '');
const ORG_ID = envOrConfigString('ORG_ID', ['orgId', 'org_id'], '1');
const TESTEE_SOURCE = envOrConfigString('TESTEE_SOURCE', ['testeeSource', 'testee_source'], 'daily_simulation');
const DISCOVER_ANSWERS = boolEnv('DISCOVER_ANSWERS', true);
const AUTO_DISCOVER_SEEDDATA = boolEnv('AUTO_DISCOVER_SEEDDATA', false);
const DISCOVER_TESTEE_LOOKBACK_DAYS = intEnv('DISCOVER_TESTEE_LOOKBACK_DAYS', 7);
const DISCOVER_TESTEE_LIMIT = intEnv('DISCOVER_TESTEE_LIMIT', 100);
const DISCOVER_ASSESSMENT_LIMIT = intEnv('DISCOVER_ASSESSMENT_LIMIT', 100);
const REPORT_TIMEOUT = intEnv('REPORT_TIMEOUT', 5);
const STATIC_REPORT_SAMPLES = loadReportSamples();
const STATIC_ANSWER_TEMPLATES = loadAnswerTemplates();

const RUN_ID = envOrConfigString('RUN_ID', ['runId', 'run_id'], `${Date.now()}`);
const IDEMPOTENCY_PREFIX = envOrConfigString('IDEMPOTENCY_PREFIX', ['idempotencyPrefix', 'idempotency_prefix'], `k6-300qps-${RUN_ID}`);
const STRICT_THRESHOLDS = boolEnv('STRICT_THRESHOLDS', false);
const CHAIN_PROBE_TIMEOUT_SECONDS = intEnv('CHAIN_PROBE_TIMEOUT_SECONDS', 120);
const CHAIN_PROBE_POLL_SECONDS = numberEnv('CHAIN_PROBE_POLL_SECONDS', 1);
const HTTP_TIMEOUT = envOrConfigString('HTTP_TIMEOUT', ['httpTimeout', 'http_timeout'], '30s');
const USER_AGENT = envOrConfigString('USER_AGENT', ['userAgent', 'user_agent'], 'qs-server-k6-300qps/1.0');

const questionnaireQueryDuration = new Trend('questionnaire_query_duration', true);
const answerSubmitDuration = new Trend('answer_submit_duration', true);
const reportStatusDuration = new Trend('report_status_duration', true);
const statisticsDuration = new Trend('statistics_duration', true);
const reportGeneratedLatency = new Trend('report_generated_latency', true);

const answerSubmitAccepted = new Counter('answer_submit_accepted');
const reportStatusPending = new Counter('report_status_pending');
const reportStatusTerminal = new Counter('report_status_terminal');
const chainProbeTerminal = new Counter('chain_probe_terminal');
const chainProbeFailed = new Counter('chain_probe_failed');
const questionnaireQueryFailed = new Counter('questionnaire_query_failed');
const answerSubmitFailed = new Counter('answer_submit_failed');
const reportStatusFailed = new Counter('report_status_failed');
const statisticsFailed = new Counter('statistics_failed');
const setupDiscoveryFailed = new Counter('setup_discovery_failed');
const http429Total = new Counter('http_429_total');
const http401Total = new Counter('http_401_total');
const http403Total = new Counter('http_403_total');
const http5xxTotal = new Counter('http_5xx_total');

const answerSubmitSuccessRate = new Rate('answer_submit_success_rate');
const reportStatusSuccessRate = new Rate('report_status_success_rate');

const scenarios = {};
addScenario('questionnaire_query', 'questionnaireQuery', QUERY_RPS, intEnv('QUERY_VUS', 80), intEnv('QUERY_MAX_VUS', 400));
addScenario('answersheet_submit', 'answerSubmit', SUBMIT_RPS, intEnv('SUBMIT_VUS', 120), intEnv('SUBMIT_MAX_VUS', 800));
addScenario('report_status_query', 'reportStatusQuery', REPORT_RPS, intEnv('REPORT_VUS', 500), intEnv('REPORT_MAX_VUS', 1500));
addScenario('statistics_query', 'statisticsQuery', STATS_RPS, intEnv('STATS_VUS', 60), intEnv('STATS_MAX_VUS', 300));

if (CHAIN_PROBE_RPS > 0) {
  scenarios.async_chain_probe = lowRateArrivalScenario(
    'asyncChainProbe',
    CHAIN_PROBE_RPS,
    intEnv('CHAIN_PROBE_VUS', 20),
    intEnv('CHAIN_PROBE_MAX_VUS', 200)
  );
}

export const options = {
  scenarios,
  thresholds: buildThresholds(),
  noConnectionReuse: boolEnv('NO_CONNECTION_REUSE', false),
  userAgent: USER_AGENT,
};

export function setup() {
  debugSetupState();
  const testeeIDs = discoverTesteeIDs();
  const questionnaireBundle = discoverQuestionnairesAndAnswers(testeeIDs);
  const reportSamples = discoverReportSamples(testeeIDs);
  const data = {
    testeeIDs,
    questionnaireCodes: uniqueList(QUESTIONNAIRE_CODES.concat(questionnaireBundle.questionnaireCodes)),
    scaleCodes: SCALE_CODES,
    answerTemplates: questionnaireBundle.answerTemplates,
    reportSamples,
  };
  validateScenarioData(data);
  return data;
}

export function questionnaireQuery(data) {
  const ctx = scenarioData(data);
  const path = renderPath(pick(QUERY_PATHS), null, ctx);
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
    endpoint: 'questionnaire_query',
    service: 'collection-server',
  });

  questionnaireQueryDuration.add(res.timings.duration, res.tags);
  recordHTTPStatus(res, questionnaireQueryFailed);
  check(res, {
    'questionnaire query status is 2xx': (r) => is2xx(r.status),
  });
}

export function answerSubmit(data) {
  const ctx = scenarioData(data);
  const payload = buildAnswerPayload(ctx);
  const requestID = payload.idempotency_key || `${IDEMPOTENCY_PREFIX}-req-${__VU}-${__ITER}-${Date.now()}`;
  const headers = jsonHeaders(collectionToken(), requestID);
  const res = timedRequest('POST', COLLECTION_BASE_URL, SUBMIT_PATH, JSON.stringify(payload), headers, {
    endpoint: 'answersheet_submit',
    service: 'collection-server',
  });

  answerSubmitDuration.add(res.timings.duration, res.tags);
  const accepted = res.status === 202;
  if (accepted) {
    answerSubmitAccepted.add(1, res.tags);
  }
  recordHTTPStatus(res, answerSubmitFailed);
  answerSubmitSuccessRate.add(accepted, res.tags);
  check(res, {
    'answersheet submit status is 202': (r) => r.status === 202,
  });
}

export function reportStatusQuery(data) {
  const ctx = scenarioData(data);
  const sample = pick(ctx.reportSamples);
  const path = renderPath(REPORT_STATUS_PATH, {
    assessment_id: sample.assessment_id,
    testee_id: sample.testee_id,
    report_timeout: String(REPORT_TIMEOUT),
  }, ctx);
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
    endpoint: 'report_status_query',
    service: 'collection-server',
  });

  reportStatusDuration.add(res.timings.duration, res.tags);
  const ok = res.status === 200;
  recordHTTPStatus(res, reportStatusFailed);
  reportStatusSuccessRate.add(ok, res.tags);
  if (ok) {
    const status = responseData(res).status || '';
    if (status === 'interpreted' || status === 'failed') {
      reportStatusTerminal.add(1, Object.assign({}, res.tags, { assessment_status: status }));
    } else {
      reportStatusPending.add(1, Object.assign({}, res.tags, { assessment_status: status || 'unknown' }));
    }
  }

  check(res, {
    'report status query status is 200': (r) => r.status === 200,
  });
}

export function statisticsQuery(data) {
  const ctx = scenarioData(data);
  const path = renderPath(pick(STATS_PATHS), null, ctx);
  const res = timedRequest('GET', APISERVER_BASE_URL, path, null, authHeaders(apiserverToken()), {
    endpoint: 'statistics_query',
    service: 'qs-apiserver',
  });

  statisticsDuration.add(res.timings.duration, res.tags);
  recordHTTPStatus(res, statisticsFailed);
  check(res, {
    'statistics query status is 2xx': (r) => is2xx(r.status),
  });
}

export function asyncChainProbe(data) {
  const ctx = scenarioData(data);
  const start = Date.now();
  const payload = buildAnswerPayload(ctx);
  const requestID = payload.idempotency_key || `${IDEMPOTENCY_PREFIX}-chain-${__VU}-${__ITER}-${start}`;
  const submitRes = timedRequest('POST', COLLECTION_BASE_URL, SUBMIT_PATH, JSON.stringify(payload), jsonHeaders(collectionToken(), requestID), {
    endpoint: 'chain_probe_submit',
    service: 'collection-server',
  });

  if (submitRes.status !== 202) {
    chainProbeFailed.add(1, { reason: 'submit_not_accepted' });
    return;
  }

  const answerSheetID = waitSubmitDone(requestID);
  if (!answerSheetID) {
    chainProbeFailed.add(1, { reason: 'submit_status_timeout' });
    return;
  }

  const assessmentID = lookupAssessmentID(answerSheetID);
  if (!assessmentID) {
    chainProbeFailed.add(1, { reason: 'assessment_lookup_failed' });
    return;
  }

  const terminalStatus = waitReportTerminal(assessmentID, payload.testee_id, ctx);
  if (!terminalStatus) {
    chainProbeFailed.add(1, { reason: 'report_timeout' });
    return;
  }

  reportGeneratedLatency.add(Date.now() - start, {
    endpoint: 'async_chain_probe',
    service: 'collection-server',
    assessment_status: terminalStatus,
  });
  chainProbeTerminal.add(1, { assessment_status: terminalStatus });
}

function waitSubmitDone(requestID) {
  const deadline = Date.now() + CHAIN_PROBE_TIMEOUT_SECONDS * 1000;
  while (Date.now() < deadline) {
    const path = renderPath(SUBMIT_STATUS_PATH, { request_id: encodeURIComponent(requestID) });
    const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
      endpoint: 'chain_probe_submit_status',
      service: 'collection-server',
    });
    if (res.status === 200) {
      const data = responseData(res);
      if (data.status === 'done' && data.answersheet_id) {
        return data.answersheet_id;
      }
      if (data.status === 'failed') {
        return '';
      }
    }
    sleep(CHAIN_PROBE_POLL_SECONDS);
  }
  return '';
}

function lookupAssessmentID(answerSheetID) {
  const path = renderPath(ANSWERSHEET_ASSESSMENT_PATH, { answersheet_id: answerSheetID });
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
    endpoint: 'chain_probe_assessment_lookup',
    service: 'collection-server',
  });
  if (res.status !== 200) {
    return '';
  }
  const data = responseData(res);
  return data.id || data.assessment_id || '';
}

function waitReportTerminal(assessmentID, testeeID, data) {
  const deadline = Date.now() + CHAIN_PROBE_TIMEOUT_SECONDS * 1000;
  while (Date.now() < deadline) {
    const path = renderPath(REPORT_STATUS_PATH, {
      assessment_id: assessmentID,
      testee_id: testeeID,
      report_timeout: String(REPORT_TIMEOUT),
    }, data);
    const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
      endpoint: 'chain_probe_report_status',
      service: 'collection-server',
    });
    if (res.status === 200) {
      const status = responseData(res).status || '';
      if (status === 'interpreted' || status === 'failed') {
        return status;
      }
    }
    sleep(CHAIN_PROBE_POLL_SECONDS);
  }
  return '';
}

function scenarioData(data) {
  const fallbackTesteeIDs = TESTEE_IDS;
  const fallbackQuestionnaireCodes = QUESTIONNAIRE_CODES;
  const fallbackReportSamples = STATIC_REPORT_SAMPLES;
  const fallbackAnswerTemplates = STATIC_ANSWER_TEMPLATES;
  return {
    testeeIDs: nonEmptyList(data && data.testeeIDs, fallbackTesteeIDs),
    questionnaireCodes: nonEmptyList(data && data.questionnaireCodes, fallbackQuestionnaireCodes),
    scaleCodes: nonEmptyList(data && data.scaleCodes, SCALE_CODES),
    reportSamples: nonEmptyList(data && data.reportSamples, fallbackReportSamples),
    answerTemplates: nonEmptyList(data && data.answerTemplates, fallbackAnswerTemplates),
  };
}

function validateScenarioData(data) {
  if ((SUBMIT_RPS > 0 || CHAIN_PROBE_RPS > 0) && COLLECTION_TOKENS.length === 0) {
    throw new Error(`TOKEN, TOKENS, TOKENS_FILE, COLLECTION_TOKEN, COLLECTION_TOKENS or a valid collectionTokensFile is required for answersheet submit.${tokenFileIssueMessage()}`);
  }
  if (STATS_RPS > 0 && APISERVER_TOKENS.length === 0) {
    throw new Error(`TOKEN, TOKENS, TOKENS_FILE, APISERVER_TOKEN, APISERVER_TOKENS or a valid apiserverTokensFile is required for statistics query.${tokenFileIssueMessage()}`);
  }
  if ((REPORT_RPS > 0 || CHAIN_PROBE_RPS > 0) && COLLECTION_TOKENS.length === 0) {
    throw new Error(`TOKEN, TOKENS, TOKENS_FILE, COLLECTION_TOKEN, COLLECTION_TOKENS or a valid collectionTokensFile is required for report status query.${tokenFileIssueMessage()}`);
  }
  if ((SUBMIT_RPS > 0 || CHAIN_PROBE_RPS > 0) && data.answerTemplates.length === 0) {
    throw new Error('No answer templates found. Set ANSWERS_JSON/ANSWERS_FILE, or provide valid collection tokens and SCALE_CODES for auto discovery. Check setup_discovery_failed plus http_401_total/http_403_total/http_5xx_total in the k6 summary.');
  }
  if ((SUBMIT_RPS > 0 || CHAIN_PROBE_RPS > 0) && data.testeeIDs.length === 0) {
    throw new Error(
      'No testee IDs found. Set TESTEE_IDS, or ensure AUTO_DISCOVER_SEEDDATA=true with apiserverTokensFile. '
      + 'If apiserver testees returns 200 in preflight but setup is empty, try TESTEE_SOURCE= or increase discover.testeeLookbackDays in qs-perf.config.json. '
      + 'Run with DEBUG_SETUP=true to see discover HTTP statuses.'
    );
  }
  if (REPORT_RPS > 0 && data.reportSamples.length === 0) {
    throw new Error('No report samples found. Set ASSESSMENT_IDS/REPORT_SAMPLES_FILE or run with AUTO_DISCOVER_SEEDDATA=true.');
  }
}

function discoverQuestionnairesAndAnswers(testeeIDs) {
  const fromStatic = STATIC_ANSWER_TEMPLATES.length > 0 ? STATIC_ANSWER_TEMPLATES : [];
  const questionnaireCodes = uniqueList(QUESTIONNAIRE_CODES.concat(fromStatic.map((item) => String(item.questionnaire_code || item.questionnaireCode || ''))));
  if (!DISCOVER_ANSWERS) {
    return { questionnaireCodes, answerTemplates: fromStatic };
  }
  if (COLLECTION_TOKENS.length === 0 && fromStatic.length > 0) {
    return { questionnaireCodes, answerTemplates: fromStatic };
  }
  if (COLLECTION_TOKENS.length === 0) {
    return { questionnaireCodes, answerTemplates: fromStatic };
  }

  const discovered = [];
  const scaleQuestionnaireCodes = [];
  SCALE_CODES.forEach((scaleCode) => {
    const scale = getCollectionData(`/api/v1/scales/${encodeURIComponent(scaleCode)}`, 'discover_scale');
    if (!scale) {
      return;
    }
    const qCode = String(scale.questionnaire_code || scale.questionnaireCode || '');
    if (qCode) {
      scaleQuestionnaireCodes.push(qCode);
    }
  });

  uniqueList(questionnaireCodes.concat(scaleQuestionnaireCodes)).forEach((qCode) => {
    const detail = getCollectionData(`/api/v1/questionnaires/${encodeURIComponent(qCode)}`, 'discover_questionnaire');
    if (!detail || !Array.isArray(detail.questions)) {
      return;
    }
    const answers = buildAnswersFromQuestionnaire(detail);
    if (answers.length === 0) {
      return;
    }
    discovered.push({
      questionnaire_code: detail.code || qCode,
      questionnaire_version: detail.version || QUESTIONNAIRE_VERSION || '',
      title: detail.title || envOrConfigString('ANSWERSHEET_TITLE', ['answersheetTitle', 'answersheet_title'], 'k6 300qps mixed scenario'),
      testee_id: pick(testeeIDs),
      answers,
    });
  });

  return {
    questionnaireCodes: uniqueList(questionnaireCodes.concat(scaleQuestionnaireCodes).concat(discovered.map((item) => item.questionnaire_code))),
    answerTemplates: fromStatic.concat(discovered),
  };
}

function appendTesteesFromResponse(data, out, requireSourceMatch) {
  responseItems(data).forEach((item) => {
    const source = String(item.source || '');
    const id = String(item.id || item.testee_id || item.testeeId || '');
    if (!id) {
      return;
    }
    if (requireSourceMatch && TESTEE_SOURCE && source !== TESTEE_SOURCE) {
      return;
    }
    out.push(id);
  });
}

function discoverTesteeIDs() {
  if (TESTEE_IDS.length > 0 || !AUTO_DISCOVER_SEEDDATA || APISERVER_TOKENS.length === 0) {
    return TESTEE_IDS;
  }
  const out = [];

  for (let offset = 0; offset < DISCOVER_TESTEE_LOOKBACK_DAYS && out.length < DISCOVER_TESTEE_LIMIT; offset += 1) {
    const date = dateStringDaysAgo(offset);
    const path = `/api/v1/testees?org_id=${encodeURIComponent(ORG_ID)}&page=1&page_size=100&created_start_date=${date}&created_end_date=${date}`;
    appendTesteesFromResponse(getApiserverData(path, 'discover_testees'), out, true);
  }

  // preflight 用无日期过滤的列表；seed 数据可能不在近 N 天窗口内，或 source 字段与 TESTEE_SOURCE 不一致
  if (out.length === 0) {
    for (let page = 1; page <= 5 && out.length < DISCOVER_TESTEE_LIMIT; page += 1) {
      const path = `/api/v1/testees?org_id=${encodeURIComponent(ORG_ID)}&page=${page}&page_size=100`;
      const data = getApiserverData(path, 'discover_testees_fallback');
      if (!data) {
        break;
      }
      appendTesteesFromResponse(data, out, true);
      if (responseItems(data).length < 100) {
        break;
      }
    }
  }

  if (out.length === 0) {
    for (let page = 1; page <= 5 && out.length < DISCOVER_TESTEE_LIMIT; page += 1) {
      const path = `/api/v1/testees?org_id=${encodeURIComponent(ORG_ID)}&page=${page}&page_size=100`;
      const data = getApiserverData(path, 'discover_testees_no_source');
      if (!data) {
        break;
      }
      appendTesteesFromResponse(data, out, false);
      if (responseItems(data).length < 100) {
        break;
      }
    }
  }

  return uniqueList(out).slice(0, DISCOVER_TESTEE_LIMIT);
}

function discoverReportSamples(testeeIDs) {
  if (STATIC_REPORT_SAMPLES.length > 0 || !AUTO_DISCOVER_SEEDDATA || APISERVER_TOKENS.length === 0) {
    return STATIC_REPORT_SAMPLES;
  }
  const out = [];
  testeeIDs.slice(0, Math.min(testeeIDs.length, DISCOVER_TESTEE_LIMIT)).forEach((testeeID) => {
    if (out.length >= DISCOVER_ASSESSMENT_LIMIT) {
      return;
    }
    const data = getApiserverData(`/api/v1/evaluations/assessments?testee_id=${encodeURIComponent(testeeID)}&page=1&page_size=20`, 'discover_assessments');
    responseItems(data).forEach((item) => {
      const assessmentID = String(item.id || item.assessment_id || item.assessmentId || '');
      const sampleTesteeID = String(item.testee_id || item.testeeId || testeeID);
      if (assessmentID && sampleTesteeID) {
        out.push({ assessment_id: assessmentID, testee_id: sampleTesteeID });
      }
    });
  });
  return uniqueReportSamples(out).slice(0, DISCOVER_ASSESSMENT_LIMIT);
}

function getCollectionData(path, endpoint) {
  const token = collectionToken();
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(token), {
    endpoint,
    service: 'collection-server',
  });
  debugSetupRequest('collection-server', endpoint, path, res.status, token);
  if (!is2xx(res.status)) {
    recordHTTPStatus(res, setupDiscoveryFailed);
    return null;
  }
  return responseData(res);
}

function getApiserverData(path, endpoint) {
  const token = apiserverToken();
  const res = timedRequest('GET', APISERVER_BASE_URL, path, null, authHeaders(token), {
    endpoint,
    service: 'qs-apiserver',
  });
  debugSetupRequest('qs-apiserver', endpoint, path, res.status, token);
  if (!is2xx(res.status)) {
    recordHTTPStatus(res, setupDiscoveryFailed);
    return null;
  }
  return responseData(res);
}

function timedRequest(method, baseURL, path, body, headers, tags) {
  return http.request(method, `${baseURL}${path}`, body, {
    headers,
    tags,
    timeout: HTTP_TIMEOUT,
  });
}

function buildAnswerPayload(data) {
  const template = clone(pick(data.answerTemplates));
  const testeeID = String(template.testee_id || template.testeeId || pick(data.testeeIDs));
  const questionnaireCode = template.questionnaire_code || template.questionnaireCode || pick(data.questionnaireCodes);
  const questionnaireVersion = template.questionnaire_version || template.questionnaireVersion || QUESTIONNAIRE_VERSION || '1.0';

  const payload = {
    questionnaire_code: questionnaireCode,
    questionnaire_version: questionnaireVersion,
    title: template.title || envOrConfigString('ANSWERSHEET_TITLE', ['answersheetTitle', 'answersheet_title'], 'k6 300qps mixed scenario'),
    testee_id: testeeID,
    task_id: template.task_id || template.taskId || __ENV.TASK_ID || undefined,
    idempotency_key: `${IDEMPOTENCY_PREFIX}-${__VU}-${__ITER}-${Date.now()}`,
    answers: normalizeAnswers(template.answers),
  };

  if (!boolEnv('USE_IDEMPOTENCY_KEY', true)) {
    delete payload.idempotency_key;
  }
  if (!payload.task_id) {
    delete payload.task_id;
  }
  return payload;
}

function normalizeAnswers(answers) {
  const source = Array.isArray(answers) && answers.length > 0 ? answers : defaultAnswers();
  return source.map((answer) => ({
    question_code: answer.question_code || answer.questionCode || 'Q1',
    question_type: answer.question_type || answer.questionType || 'Radio',
    value: stringifyAnswerValue(answer.value === undefined ? 'A' : answer.value),
    score: Number(answer.score || 0),
  }));
}

function buildAnswersFromQuestionnaire(detail) {
  const questions = Array.isArray(detail.questions) ? detail.questions : [];
  const answers = [];
  questions.forEach((question, index) => {
    const answer = buildAnswerForQuestion(question, index);
    if (answer) {
      answers.push(answer);
    }
  });
  return answers;
}

function buildAnswerForQuestion(question, index) {
  const questionType = resolveQuestionType(question);
  const normalizedType = normalizeQuestionType(questionType);
  const options = Array.isArray(question.options) ? question.options : [];

  if (normalizedType === 'radio') {
    if (options.length === 0) {
      return null;
    }
    const option = options[index % options.length];
    const value = option.code || option.content || '';
    if (!value) {
      return null;
    }
    return answer(question, 'Radio', value);
  }

  if (normalizedType === 'checkbox') {
    if (options.length === 0) {
      return null;
    }
    let minSelections = Math.max(1, intRuleValue(question, 'min_selections', 1));
    if (hasRequiredRule(question)) {
      minSelections = Math.max(1, minSelections);
    }
    let maxSelections = options.length;
    const maxRule = intRuleValue(question, 'max_selections', 0);
    if (maxRule > 0) {
      maxSelections = Math.min(maxSelections, maxRule);
    }
    minSelections = Math.min(minSelections, options.length);
    maxSelections = Math.max(maxSelections, minSelections);
    const count = minSelections;
    const values = [];
    for (let i = 0; i < options.length && values.length < count; i += 1) {
      const option = options[(index + i) % options.length];
      const value = option.code || option.content || '';
      if (value) {
        values.push(value);
      }
    }
    return values.length > 0 ? answer(question, 'Checkbox', values) : null;
  }

  if (normalizedType === 'text' || normalizedType === 'textarea') {
    return answer(question, questionType, buildTextAnswer(question, index));
  }

  if (normalizedType === 'number') {
    return answer(question, 'Number', buildNumberAnswer(question, index));
  }

  return null;
}

function answer(question, questionType, value) {
  return {
    question_code: question.code || question.question_code || '',
    question_type: questionType,
    score: 0,
    value,
  };
}

function resolveQuestionType(question) {
  const normalized = normalizeQuestionType(question.type || question.question_type || '');
  if (['radio', 'checkbox', 'text', 'textarea', 'number', 'section'].indexOf(normalized) >= 0) {
    return normalized.charAt(0).toUpperCase() + normalized.slice(1);
  }
  const options = Array.isArray(question.options) ? question.options : [];
  return options.length > 0 ? 'Radio' : 'Section';
}

function normalizeQuestionType(raw) {
  return String(raw || '').trim().toLowerCase();
}

function buildTextAnswer(question, index) {
  let minLength = Math.max(2, intRuleValue(question, 'min_length', 2));
  const maxLength = intRuleValue(question, 'max_length', 0);
  if (maxLength > 0 && maxLength < minLength) {
    minLength = maxLength;
  }

  const pattern = stringRuleValue(question, 'pattern', '');
  const candidates = [
    '情况稳定',
    '状态良好',
    '需要关注',
    '测试填写',
    '学习正常',
    '睡眠正常',
    '情绪平稳',
    '测试123',
    '123456',
    '13812345678',
    'test@example.com',
  ];
  for (let i = 0; i < candidates.length; i += 1) {
    const candidate = normalizeTextLength(candidates[(index + i) % candidates.length], minLength, maxLength);
    if (!candidate || (pattern && !matchesPattern(candidate, pattern))) {
      continue;
    }
    return candidate;
  }
  return normalizeTextLength('测'.repeat(Math.max(minLength, 1)), minLength, maxLength);
}

function buildNumberAnswer(question, index) {
  const minValue = floatRuleValue(question, 'min_value', 1);
  const maxValue = Math.max(minValue, floatRuleValue(question, 'max_value', 100));
  const rangeSize = Math.floor(maxValue - minValue) + 1;
  if (rangeSize <= 1) {
    return minValue;
  }
  return minValue + (index % rangeSize);
}

function hasRequiredRule(question) {
  return stringRuleValue(question, 'required', '') === 'true';
}

function intRuleValue(question, ruleType, fallback) {
  const raw = stringRuleValue(question, ruleType, '');
  if (!raw) {
    return fallback;
  }
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? Math.floor(parsed) : fallback;
}

function floatRuleValue(question, ruleType, fallback) {
  const raw = stringRuleValue(question, ruleType, '');
  if (!raw) {
    return fallback;
  }
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function stringRuleValue(question, ruleType, fallback) {
  const rules = Array.isArray(question.validation_rules) ? question.validation_rules : [];
  const matched = rules.find((rule) => String(rule.rule_type || '').trim() === ruleType);
  if (!matched) {
    return fallback;
  }
  return String(matched.target_value || '').trim();
}

function normalizeTextLength(value, minLength, maxLength) {
  let text = String(value || '');
  while ([...text].length < minLength) {
    text += '测';
  }
  if (maxLength > 0 && [...text].length > maxLength) {
    text = [...text].slice(0, maxLength).join('');
  }
  return text;
}

function matchesPattern(value, pattern) {
  try {
    return new RegExp(pattern).test(value);
  } catch (_) {
    return true;
  }
}

function stringifyAnswerValue(value) {
  if (typeof value === 'string') {
    return value;
  }
  return JSON.stringify(value);
}

function defaultAnswers() {
  return [
    { question_code: __ENV.DEFAULT_QUESTION_CODE || 'Q1', question_type: __ENV.DEFAULT_QUESTION_TYPE || 'Radio', value: __ENV.DEFAULT_ANSWER_VALUE || 'A' },
  ];
}

function loadAnswerTemplates() {
  const answersFile = envOrConfigString('ANSWERS_FILE', ['answersFile', 'answers_file'], '');
  if (answersFile) {
    const parsed = JSON.parse(readTextFile(answersFile, configFileBaseDirs()).content);
    if (Array.isArray(parsed)) {
      return parsed;
    }
    if (Array.isArray(parsed.answersheets)) {
      return parsed.answersheets;
    }
    return [parsed];
  }
  const answersJSON = __ENV.ANSWERS_JSON || configFirstValue(['answersJson', 'answers_json']);
  if (answersJSON) {
    const parsed = typeof answersJSON === 'string' ? JSON.parse(answersJSON) : answersJSON;
    return Array.isArray(parsed) ? parsed : [parsed];
  }
  const answerTemplates = configFirstValue(['answerTemplates', 'answer_templates', 'answersheets']);
  if (Array.isArray(answerTemplates)) {
    return answerTemplates;
  }
  return [];
}

function loadReportSamples() {
  const reportSamplesFile = envOrConfigString('REPORT_SAMPLES_FILE', ['reportSamplesFile', 'report_samples_file'], '');
  if (reportSamplesFile) {
    const parsed = JSON.parse(readTextFile(reportSamplesFile, configFileBaseDirs()).content);
    return parsed.map((item) => ({
      assessment_id: String(item.assessment_id || item.assessmentId || item.id),
      testee_id: String(item.testee_id || item.testeeId || pick(TESTEE_IDS)),
    }));
  }
  const reportSamples = configFirstValue(['reportSamples', 'report_samples']);
  if (Array.isArray(reportSamples)) {
    return reportSamples.map((item) => ({
      assessment_id: String(item.assessment_id || item.assessmentId || item.id),
      testee_id: String(item.testee_id || item.testeeId || pick(TESTEE_IDS)),
    }));
  }
  if (ASSESSMENT_IDS.length === 0) {
    return [];
  }
  return ASSESSMENT_IDS.map((assessmentID, index) => ({
    assessment_id: String(assessmentID),
    testee_id: String(TESTEE_IDS[index % TESTEE_IDS.length]),
  })).filter((item) => item.assessment_id && item.testee_id);
}

function buildThresholds() {
  const thresholds = {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  };
  if (SUBMIT_RPS > 0 || CHAIN_PROBE_RPS > 0) {
    thresholds.answer_submit_success_rate = ['rate>0.99'];
  }
  if (REPORT_RPS > 0 || CHAIN_PROBE_RPS > 0) {
    thresholds.report_status_success_rate = ['rate>0.99'];
  }
  if (!STRICT_THRESHOLDS) {
    return thresholds;
  }
  if (QUERY_RPS > 0) {
    thresholds.questionnaire_query_duration = ['p(95)<500', 'p(99)<1200'];
  }
  if (SUBMIT_RPS > 0 || CHAIN_PROBE_RPS > 0) {
    thresholds.answer_submit_duration = ['p(95)<1000', 'p(99)<2000'];
  }
  if (REPORT_RPS > 0 || CHAIN_PROBE_RPS > 0) {
    thresholds.report_status_duration = ['p(95)<1500', 'p(99)<3000'];
  }
  if (STATS_RPS > 0) {
    thresholds.statistics_duration = ['p(95)<1000', 'p(99)<2000'];
  }
  return thresholds;
}

function addScenario(name, exec, rate, preAllocatedVUs, maxVUs) {
  if (rate <= 0) {
    return;
  }
  scenarios[name] = arrivalScenario(exec, rate, preAllocatedVUs, maxVUs);
}

function arrivalScenario(exec, rate, preAllocatedVUs, maxVUs) {
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

function lowRateArrivalScenario(exec, perSecondRate, preAllocatedVUs, maxVUs) {
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

function collectionToken() {
  return pick(COLLECTION_TOKENS);
}

function apiserverToken() {
  return pick(APISERVER_TOKENS);
}

function recordHTTPStatus(res, endpointFailedCounter) {
  if (is2xx(res.status)) {
    return;
  }
  endpointFailedCounter.add(1, res.tags);
  if (res.status === 429) {
    http429Total.add(1, res.tags);
  }
  if (res.status === 401) {
    http401Total.add(1, res.tags);
  }
  if (res.status === 403) {
    http403Total.add(1, res.tags);
  }
  if (res.status >= 500) {
    http5xxTotal.add(1, res.tags);
  }
}

function authHeaders(token) {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function jsonHeaders(token, requestID) {
  const headers = Object.assign({ 'Content-Type': 'application/json' }, authHeaders(token));
  if (requestID) {
    headers['X-Request-ID'] = requestID;
  }
  return headers;
}

function responseData(res) {
  try {
    const parsed = res.json();
    if (parsed && parsed.data !== undefined) {
      return parsed.data || {};
    }
    return parsed || {};
  } catch (_) {
    return {};
  }
}

function renderPath(path, values, data) {
  const ctx = scenarioData(data);
  const vars = Object.assign(
    {
      testee_id: pick(ctx.testeeIDs),
      assessment_id: pick(ASSESSMENT_IDS),
      questionnaire_code: pick(ctx.questionnaireCodes),
      scale_code: pick(ctx.scaleCodes),
      plan_id: pick(PLAN_IDS),
      entry_id: pick(ENTRY_IDS),
      report_timeout: String(REPORT_TIMEOUT),
    },
    values || {}
  );
  let rendered = path;
  Object.keys(vars).forEach((key) => {
    rendered = rendered.replace(new RegExp(`\\{${key}\\}`, 'g'), String(vars[key]));
  });
  return rendered;
}

function pick(items) {
  if (!items || items.length === 0) {
    return '';
  }
  const vu = typeof __VU === 'undefined' ? 0 : __VU;
  const iter = typeof __ITER === 'undefined' ? 0 : __ITER;
  return items[(iter + vu) % items.length];
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

function loadPerfConfig() {
  if (!PERF_CONFIG_PATH) {
    return {};
  }
  const loaded = readTextFile(PERF_CONFIG_PATH, perfConfigBaseDirs());
  PERF_CONFIG_DIR = dirnamePath(loaded.path);
  const raw = loaded.content.trim();
  if (!raw) {
    return {};
  }
  return JSON.parse(raw);
}

function envOrConfigString(envName, keys, fallback) {
  const envValue = __ENV[envName];
  if (envValue !== undefined && envValue !== '') {
    return String(envValue);
  }
  return configStringValue(keys, fallback);
}

function envOrConfigList(envName, keys, fallback) {
  const envValue = __ENV[envName];
  if (envValue !== undefined && envValue !== '') {
    return listValue(envValue);
  }
  const configValue = configFirstValue(keys);
  if (configValue !== undefined && configValue !== null && configValue !== '') {
    return listValue(configValue);
  }
  return listValue(fallback);
}

function configStringValue(keys, fallback) {
  const value = configFirstValue(keys);
  if (value === undefined || value === null || value === '') {
    return fallback;
  }
  return String(value);
}

function configFirstValue(keys) {
  const profileValue = configFirstValueFrom(QPS_PROFILE_CONFIG, keys);
  if (profileValue !== undefined && profileValue !== null && profileValue !== '') {
    return profileValue;
  }
  return configFirstValueFrom(PERF_CONFIG, keys);
}

function configFirstValueFrom(config, keys) {
  for (let i = 0; i < keys.length; i += 1) {
    const value = configPathValue(config, keys[i]);
    if (value !== undefined && value !== null && value !== '') {
      return value;
    }
  }
  return undefined;
}

function resolveQpsProfileName() {
  const envValue = __ENV.QPS_PROFILE || __ENV.PERF_PROFILE || '';
  if (envValue) {
    return String(envValue);
  }
  const configValue = configFirstValueFrom(PERF_CONFIG, ['qpsProfile', 'qps_profile', 'profile', 'defaultQpsProfile', 'default_qps_profile']);
  return configValue ? String(configValue) : '';
}

function resolveQpsProfileConfig(profileName) {
  if (!profileName) {
    return {};
  }
  const profile = configPathValue(PERF_CONFIG, `qpsProfiles.${profileName}`) || configPathValue(PERF_CONFIG, `qps_profiles.${profileName}`);
  if (!profile) {
    throw new Error(`QPS_PROFILE=${profileName} was not found in qpsProfiles.`);
  }
  return profile;
}

function configPathValue(obj, path) {
  if (!obj || !path) {
    return undefined;
  }
  const parts = String(path).split('.');
  let current = obj;
  for (let i = 0; i < parts.length; i += 1) {
    if (current === undefined || current === null) {
      return undefined;
    }
    current = current[parts[i]];
  }
  return current;
}

function configAliasesForEnv(name) {
  const aliases = {
    QUERY_RPS: ['qps.query', 'queryRps', 'query_rps'],
    SUBMIT_RPS: ['qps.submit', 'submitRps', 'submit_rps'],
    REPORT_RPS: ['qps.report', 'reportRps', 'report_rps'],
    STATS_RPS: ['qps.stats', 'statsRps', 'stats_rps'],
    CHAIN_PROBE_RPS: ['qps.chainProbe', 'qps.chain_probe', 'chainProbeRps', 'chain_probe_rps'],
    QUERY_VUS: ['vusers.query.preAllocated', 'vusers.query.pre_allocated', 'queryVus', 'query_vus'],
    QUERY_MAX_VUS: ['vusers.query.max', 'queryMaxVus', 'query_max_vus'],
    SUBMIT_VUS: ['vusers.submit.preAllocated', 'vusers.submit.pre_allocated', 'submitVus', 'submit_vus'],
    SUBMIT_MAX_VUS: ['vusers.submit.max', 'submitMaxVus', 'submit_max_vus'],
    REPORT_VUS: ['vusers.report.preAllocated', 'vusers.report.pre_allocated', 'reportVus', 'report_vus'],
    REPORT_MAX_VUS: ['vusers.report.max', 'reportMaxVus', 'report_max_vus'],
    STATS_VUS: ['vusers.stats.preAllocated', 'vusers.stats.pre_allocated', 'statsVus', 'stats_vus'],
    STATS_MAX_VUS: ['vusers.stats.max', 'statsMaxVus', 'stats_max_vus'],
    CHAIN_PROBE_VUS: ['vusers.chainProbe.preAllocated', 'vusers.chain_probe.pre_allocated', 'chainProbeVus', 'chain_probe_vus'],
    CHAIN_PROBE_MAX_VUS: ['vusers.chainProbe.max', 'vusers.chain_probe.max', 'chainProbeMaxVus', 'chain_probe_max_vus'],
    DISCOVER_ANSWERS: ['discoverAnswers', 'discover_answers'],
    AUTO_DISCOVER_SEEDDATA: ['autoDiscoverSeeddata', 'auto_discover_seeddata'],
    DISCOVER_TESTEE_LOOKBACK_DAYS: ['discover.testeeLookbackDays', 'discover.testee_lookback_days', 'discoverTesteeLookbackDays', 'discover_testee_lookback_days'],
    DISCOVER_TESTEE_LIMIT: ['discover.testeeLimit', 'discover.testee_limit', 'discoverTesteeLimit', 'discover_testee_limit'],
    DISCOVER_ASSESSMENT_LIMIT: ['discover.assessmentLimit', 'discover.assessment_limit', 'discoverAssessmentLimit', 'discover_assessment_limit'],
    REPORT_TIMEOUT: ['reportTimeout', 'report_timeout'],
    STRICT_THRESHOLDS: ['strictThresholds', 'strict_thresholds'],
    CHAIN_PROBE_TIMEOUT_SECONDS: ['chainProbeTimeoutSeconds', 'chain_probe_timeout_seconds'],
    CHAIN_PROBE_POLL_SECONDS: ['chainProbePollSeconds', 'chain_probe_poll_seconds'],
    NO_CONNECTION_REUSE: ['noConnectionReuse', 'no_connection_reuse'],
    USE_IDEMPOTENCY_KEY: ['useIdempotencyKey', 'use_idempotency_key'],
  };
  return aliases[name] || [];
}

function listValue(value) {
  if (value === undefined || value === null || value === '') {
    return [];
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item || '').trim()).filter((item) => item.length > 0);
  }
  return String(value)
    .split(',')
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function listEnv(name, fallback) {
  const raw = __ENV[name] || fallback || '';
  return listValue(raw);
}

function listFilePath(path) {
  if (!path) {
    return [];
  }
  const loaded = tryReadTextFile(path, configFileBaseDirs());
  if (!loaded) {
    return [];
  }
  return parseListFileContent(loaded.content);
}

function listTokenFilePath(label, path) {
  if (!path) {
    return [];
  }
  const loaded = tryReadTextFile(path, configFileBaseDirs());
  if (!loaded) {
    TOKEN_FILE_READ_ISSUES.push(`${label}=${path} not found or unreadable`);
    return [];
  }
  const items = parseListFileContent(loaded.content);
  TOKEN_FILE_LOADS.push({ label, path, resolved: loaded.path, count: items.length });
  if (items.length === 0) {
    TOKEN_FILE_READ_ISSUES.push(`${label}=${path} is empty`);
  }
  return items;
}

function debugSetupState() {
  if (!DEBUG_SETUP) {
    return;
  }
  console.log(`[setup-debug] config=${PERF_CONFIG_PATH || '<none>'} profile=${QPS_PROFILE || '<none>'}`);
  console.log(`[setup-debug] collectionBaseUrl=${COLLECTION_BASE_URL} apiserverBaseUrl=${APISERVER_BASE_URL}`);
  console.log(`[setup-debug] tokenFileLoads=${JSON.stringify(TOKEN_FILE_LOADS)}`);
  console.log(`[setup-debug] tokenFileIssues=${JSON.stringify(TOKEN_FILE_READ_ISSUES)}`);
  console.log(`[setup-debug] autoDiscoverSeeddata=${AUTO_DISCOVER_SEEDDATA} testeeSource=${TESTEE_SOURCE || '<any>'} lookbackDays=${DISCOVER_TESTEE_LOOKBACK_DAYS}`);
}

function debugSetupRequest(service, endpoint, path, status, token) {
  if (!DEBUG_SETUP) {
    return;
  }
  console.log(`[setup-debug] ${service} endpoint=${endpoint} status=${status} auth=${token ? 'yes' : 'no'} path=${path}`);
}

function parseListFileContent(content) {
  const raw = String(content || '').trim();
  if (!raw) {
    return [];
  }
  if (raw[0] === '[' || raw[0] === '{') {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item || '').trim()).filter((item) => item.length > 0);
    }
    if (Array.isArray(parsed.tokens)) {
      return parsed.tokens.map((item) => String(item || '').trim()).filter((item) => item.length > 0);
    }
  }
  return raw
    .split(/[,\n\r]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function tokenFileIssueMessage() {
  if (TOKEN_FILE_READ_ISSUES.length === 0) {
    return '';
  }
  return ` Token file issues: ${TOKEN_FILE_READ_ISSUES.join('; ')}.`;
}

function tryReadTextFile(path, baseDirs) {
  try {
    return readTextFile(path, baseDirs);
  } catch (_) {
    return null;
  }
}

function readTextFile(path, baseDirs) {
  const candidates = filePathCandidates(path, baseDirs);
  let lastError = null;
  for (let i = 0; i < candidates.length; i += 1) {
    try {
      return { path: candidates[i], content: open(candidates[i]) };
    } catch (err) {
      lastError = err;
    }
  }
  throw new Error(`Cannot open ${path}. Tried: ${candidates.join(', ')}. ${lastError || ''}`);
}

function filePathCandidates(path, baseDirs) {
  const raw = String(path || '').trim();
  if (!raw) {
    return [];
  }
  if (isAbsolutePath(raw)) {
    return [raw];
  }

  const candidates = [];
  (baseDirs || []).forEach((baseDir) => {
    const normalizedBase = normalizeDirPath(baseDir);
    if (normalizedBase) {
      candidates.push(`${normalizedBase}/${trimLeadingDotSlash(raw)}`);
    }
  });
  candidates.push(raw);
  return uniqueList(candidates);
}

function perfConfigBaseDirs() {
  // k6 open() 对裸相对路径以脚本目录 scripts/perf 为基准；
  // check-token-preflight.sh 则以配置文件所在目录为基准。此处补齐仓库根等候选路径。
  return uniqueList([
    __ENV.PERF_ROOT_DIR || '',
    __ENV.PWD || '',
    '../..',
  ]);
}

function configFileBaseDirs() {
  return uniqueList([
    PERF_CONFIG_DIR,
    configStringValue(['rootDir', 'root_dir'], ''),
    __ENV.PERF_ROOT_DIR || '',
    __ENV.PWD || '',
    '../..',
  ]);
}

function dirnamePath(path) {
  const normalized = String(path || '').replace(/\/+$/, '');
  const index = normalized.lastIndexOf('/');
  return index > 0 ? normalized.slice(0, index) : '';
}

function normalizeDirPath(path) {
  return String(path || '').trim().replace(/\/+$/, '');
}

function trimLeadingDotSlash(path) {
  return String(path || '').replace(/^\.\/+/, '');
}

function isAbsolutePath(path) {
  return String(path || '').indexOf('/') === 0;
}

function nonEmptyList(primary, fallback) {
  if (Array.isArray(primary) && primary.length > 0) {
    return primary;
  }
  return Array.isArray(fallback) ? fallback : [];
}

function uniqueList(items) {
  const seen = {};
  const out = [];
  (items || []).forEach((item) => {
    const value = String(item || '').trim();
    if (!value || seen[value]) {
      return;
    }
    seen[value] = true;
    out.push(value);
  });
  return out;
}

function uniqueReportSamples(samples) {
  const seen = {};
  const out = [];
  (samples || []).forEach((sample) => {
    const assessmentID = String(sample.assessment_id || '').trim();
    const testeeID = String(sample.testee_id || '').trim();
    const key = `${assessmentID}:${testeeID}`;
    if (!assessmentID || !testeeID || seen[key]) {
      return;
    }
    seen[key] = true;
    out.push({ assessment_id: assessmentID, testee_id: testeeID });
  });
  return out;
}

function responseItems(data) {
  if (!data) {
    return [];
  }
  if (Array.isArray(data.items)) {
    return data.items;
  }
  if (Array.isArray(data.testees)) {
    return data.testees;
  }
  if (Array.isArray(data.assessments)) {
    return data.assessments;
  }
  if (Array.isArray(data.data)) {
    return data.data;
  }
  return [];
}

function dateStringDaysAgo(offset) {
  const date = new Date(Date.now() - offset * 24 * 60 * 60 * 1000);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function intEnv(name, fallback) {
  const value = numberEnv(name, fallback);
  return Number.isFinite(value) ? Math.floor(value) : fallback;
}

function numberEnv(name, fallback) {
  let raw = __ENV[name];
  if (raw === undefined || raw === '') {
    raw = configFirstValue(configAliasesForEnv(name));
  }
  if (raw === undefined || raw === null || raw === '') {
    raw = fallback;
  }
  const value = Number(raw);
  return Number.isFinite(value) ? value : fallback;
}

function boolEnv(name, fallback) {
  let raw = __ENV[name];
  if (raw === undefined || raw === '') {
    raw = configFirstValue(configAliasesForEnv(name));
  }
  if (raw === undefined || raw === null || raw === '') {
    return fallback;
  }
  const normalized = String(raw).trim().toLowerCase();
  if (['1', 'true', 'yes', 'y', 'on'].indexOf(normalized) >= 0) {
    return true;
  }
  if (['0', 'false', 'no', 'n', 'off'].indexOf(normalized) >= 0) {
    return false;
  }
  return fallback;
}

function normalizeBaseURL(url) {
  return url.replace(/\/+$/, '');
}

function is2xx(status) {
  return status >= 200 && status < 300;
}
