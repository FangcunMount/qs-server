import {
  readTextFile,
  uniqueList,
  dirnamePath,
  normalizeDirPath,
  trimLeadingDotSlash,
  isAbsolutePath,
  perfConfigBaseDirs,
  listValue,
  parseListFileContent,
  tryReadTextFile,
  normalizeBaseURL,
} from './util.js';

export let PERF_CONFIG_DIR = '';

export function loadPerfConfig() {
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

export function envOrConfigString(envName, keys, fallback) {
  const envValue = __ENV[envName];
  if (envValue !== undefined && envValue !== '') {
    return String(envValue);
  }
  return configStringValue(keys, fallback);
}

export function envOrConfigList(envName, keys, fallback) {
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

export function configStringValue(keys, fallback) {
  const value = configFirstValue(keys);
  if (value === undefined || value === null || value === '') {
    return fallback;
  }
  return String(value);
}

export function configFirstValue(keys) {
  const profileValue = configFirstValueFrom(QPS_PROFILE_CONFIG, keys);
  if (profileValue !== undefined && profileValue !== null && profileValue !== '') {
    return profileValue;
  }
  return configFirstValueFrom(PERF_CONFIG, keys);
}

export function configFirstValueFrom(config, keys) {
  for (let i = 0; i < keys.length; i += 1) {
    const value = configPathValue(config, keys[i]);
    if (value !== undefined && value !== null && value !== '') {
      return value;
    }
  }
  return undefined;
}

export function resolveQpsProfileName() {
  const envValue = __ENV.QPS_PROFILE || __ENV.PERF_PROFILE || '';
  if (envValue) {
    return String(envValue);
  }
  const configValue = configFirstValueFrom(PERF_CONFIG, ['qpsProfile', 'qps_profile', 'profile', 'defaultQpsProfile', 'default_qps_profile']);
  return configValue ? String(configValue) : '';
}

export function resolveQpsProfileConfig(profileName) {
  if (!profileName) {
    return {};
  }
  const profile = configPathValue(PERF_CONFIG, `qpsProfiles.${profileName}`) || configPathValue(PERF_CONFIG, `qps_profiles.${profileName}`);
  if (!profile) {
    throw new Error(`QPS_PROFILE=${profileName} was not found in qpsProfiles.`);
  }
  return profile;
}

export function configPathValue(obj, path) {
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

export function configAliasesForEnv(name) {
  const aliases = {
    QUERY_RPS: ['qps.query', 'queryRps', 'query_rps'],
    MEDICAL_QUERY_RPS: ['qps.medicalQuery', 'qps.medical_query', 'medicalQueryRps', 'medical_query_rps'],
    PERSONALITY_QUERY_RPS: ['qps.personalityQuery', 'qps.personality_query', 'personalityQueryRps', 'personality_query_rps'],
    QUESTIONNAIRE_DETAIL_RPS: ['qps.questionnaireQuery', 'qps.questionnaire_query', 'questionnaireQueryRps', 'questionnaire_query_rps'],
    PERSONALITY_SESSION_RPS: ['qps.personalitySession', 'qps.personality_session', 'personalitySessionRps', 'personality_session_rps'],
    SUBMIT_RPS: ['qps.submit', 'submitRps', 'submit_rps'],
    MEDICAL_SUBMIT_RPS: ['qps.medicalSubmit', 'qps.medical_submit', 'medicalSubmitRps', 'medical_submit_rps'],
    PERSONALITY_SUBMIT_RPS: ['qps.personalitySubmit', 'qps.personality_submit', 'personalitySubmitRps', 'personality_submit_rps'],
    REPORT_RPS: ['qps.report', 'reportRps', 'report_rps'],
    MEDICAL_REPORT_RPS: ['qps.medicalReport', 'qps.medical_report', 'medicalReportRps', 'medical_report_rps', 'qps.medicalWaitReport', 'qps.medical_wait_report'],
    PERSONALITY_REPORT_RPS: ['qps.personalityReport', 'qps.personality_report', 'personalityReportRps', 'personality_report_rps', 'qps.personalityWaitReport', 'qps.personality_wait_report'],
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

export function intEnv(name, fallback) {
  const value = numberEnv(name, fallback);
  return Number.isFinite(value) ? Math.floor(value) : fallback;
}

export function numberEnv(name, fallback) {
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

export function boolEnv(name, fallback) {
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

export function hasGranularQueryQps() {
  return hasConfigOrEnvQps([
    'MEDICAL_QUERY_RPS',
    'PERSONALITY_QUERY_RPS',
    'QUESTIONNAIRE_DETAIL_RPS',
    'qps.medicalQuery',
    'qps.personalityQuery',
    'qps.questionnaireQuery',
  ]);
}

export function hasGranularReportQps() {
  return hasConfigOrEnvQps([
    'MEDICAL_REPORT_RPS',
    'PERSONALITY_REPORT_RPS',
    'qps.medicalReport',
    'qps.personalityReport',
    'qps.medicalWaitReport',
    'qps.personalityWaitReport',
  ]);
}

export function hasGranularSubmitQps() {
  return hasConfigOrEnvQps([
    'MEDICAL_SUBMIT_RPS',
    'PERSONALITY_SUBMIT_RPS',
    'qps.medicalSubmit',
    'qps.personalitySubmit',
  ]);
}

export function hasConfigOrEnvQps(keys) {
  for (let i = 0; i < keys.length; i += 1) {
    const key = keys[i];
    if (__ENV[key] !== undefined && __ENV[key] !== '') {
      return true;
    }
    if (configFirstValue([key]) !== undefined && configFirstValue([key]) !== null && configFirstValue([key]) !== '') {
      return true;
    }
  }
  return false;
}

export function resolveSubmitMix() {
  const mix = configPathValue(PERF_CONFIG, 'modelMix') || configPathValue(PERF_CONFIG, 'model_mix') || configPathValue(QPS_PROFILE_CONFIG, 'modelMix') || configPathValue(QPS_PROFILE_CONFIG, 'model_mix') || {};
  const medical = Number(mix.medical !== undefined ? mix.medical : mix.medical_scale);
  const personality = Number(mix.personality !== undefined ? mix.personality : mix.personality_model);
  if (Number.isFinite(medical) && Number.isFinite(personality) && medical + personality > 0) {
    return { medical, personality };
  }
  const medicalEnv = numberEnv('SUBMIT_MIX_MEDICAL', NaN);
  const personalityEnv = numberEnv('SUBMIT_MIX_PERSONALITY', NaN);
  if (Number.isFinite(medicalEnv) && Number.isFinite(personalityEnv) && medicalEnv + personalityEnv > 0) {
    return { medical: medicalEnv, personality: personalityEnv };
  }
  return { medical: 0.8, personality: 0.2 };
}

export function resolveChainProbeRps(modelType) {
  if (CHAIN_PROBE_RPS <= 0) {
    return 0;
  }
  const type = CHAIN_PROBE_MODEL_TYPE;
  if (type === 'medical' && modelType !== 'medical') {
    return 0;
  }
  if (type === 'personality' && modelType !== 'personality') {
    return 0;
  }
  if (type === 'mixed') {
    return CHAIN_PROBE_RPS / 2;
  }
  return CHAIN_PROBE_RPS;
}

export function listEnv(name, fallback) {
  const raw = __ENV[name] || fallback || '';
  return listValue(raw);
}

export function listFilePath(path) {
  if (!path) {
    return [];
  }
  const loaded = tryReadTextFile(path, configFileBaseDirs());
  if (!loaded) {
    return [];
  }
  return parseListFileContent(loaded.content);
}

export function listTokenFilePath(label, path) {
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

export function tokenFileIssueMessage() {
  if (TOKEN_FILE_READ_ISSUES.length === 0) {
    return '';
  }
  return ` Token file issues: ${TOKEN_FILE_READ_ISSUES.join('; ')}.`;
}

export function debugSetupState() {
  if (!DEBUG_SETUP) {
    return;
  }
  console.log(`[setup-debug] config=${PERF_CONFIG_PATH || '<none>'} profile=${QPS_PROFILE || '<none>'}`);
  console.log(`[setup-debug] collectionBaseUrl=${COLLECTION_BASE_URL} apiserverBaseUrl=${APISERVER_BASE_URL}`);
  console.log(`[setup-debug] tokenFileLoads=${JSON.stringify(TOKEN_FILE_LOADS)}`);
  console.log(`[setup-debug] tokenFileIssues=${JSON.stringify(TOKEN_FILE_READ_ISSUES)}`);
  console.log(`[setup-debug] autoDiscoverSeeddata=${AUTO_DISCOVER_SEEDDATA} testeeSource=${TESTEE_SOURCE || '<any>'} lookbackDays=${DISCOVER_TESTEE_LOOKBACK_DAYS}`);
}

export function debugSetupRequest(service, endpoint, path, status, token) {
  if (!DEBUG_SETUP) {
    return;
  }
  console.log(`[setup-debug] ${service} endpoint=${endpoint} status=${status} auth=${token ? 'yes' : 'no'} path=${path}`);
}

export function configFileBaseDirs() {
  return uniqueList([
    PERF_CONFIG_DIR,
    configStringValue(['rootDir', 'root_dir'], ''),
    __ENV.PERF_ROOT_DIR || '',
    __ENV.PWD || '',
    '../..',
  ]);
}


export const PERF_CONFIG_PATH = __ENV.PERF_CONFIG_FILE || __ENV.K6_CONFIG_FILE || '';
export const PERF_CONFIG = loadPerfConfig();
export const TOKEN_FILE_READ_ISSUES = [];
export const TOKEN_FILE_LOADS = [];
export const SCRIPT_INIT_AT_MS = Date.now();
export const QPS_PROFILE = resolveQpsProfileName();
export const QPS_PROFILE_CONFIG = resolveQpsProfileConfig(QPS_PROFILE);
export const DEBUG_SETUP = boolEnv('DEBUG_SETUP', false);

export const COLLECTION_BASE_URL = normalizeBaseURL(
  __ENV.COLLECTION_BASE_URL ||
    __ENV.BASE_URL ||
    configStringValue(['collectionBaseUrl', 'collection_base_url', 'collection.baseUrl', 'collection.base_url'], 'http://127.0.0.1:18083')
);
export const APISERVER_BASE_URL = normalizeBaseURL(
  __ENV.APISERVER_BASE_URL ||
    configStringValue(['apiserverBaseUrl', 'apiserver_base_url', 'apiserver.baseUrl', 'apiserver.base_url'], 'http://127.0.0.1:18082')
);
export const SEEDDATA_SCALE_CODES = '3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35';
export const SEEDDATA_PLAN_IDS = '614333603412718126,614187067651404334';

export const TOKEN = envOrConfigString('TOKEN', ['token'], '');
export const COLLECTION_TOKEN = envOrConfigString('COLLECTION_TOKEN', ['collectionToken', 'collection_token', 'collection.token'], TOKEN);
export const APISERVER_TOKEN = envOrConfigString('APISERVER_TOKEN', ['apiserverToken', 'apiserver_token', 'apiserver.token'], TOKEN);
export const TOKENS_FILE = envOrConfigString('TOKENS_FILE', ['tokensFile', 'tokens_file'], '');
export const COLLECTION_TOKENS_FILE = envOrConfigString('COLLECTION_TOKENS_FILE', ['collectionTokensFile', 'collection_tokens_file', 'collection.tokensFile', 'collection.tokens_file'], '');
export const APISERVER_TOKENS_FILE = envOrConfigString('APISERVER_TOKENS_FILE', ['apiserverTokensFile', 'apiserver_tokens_file', 'apiserver.tokensFile', 'apiserver.tokens_file'], '');
export const COMMON_TOKENS = uniqueList(
  envOrConfigList('TOKENS', ['tokens'], TOKEN).concat(listTokenFilePath('tokensFile', TOKENS_FILE))
);
export const COLLECTION_SPECIFIC_TOKENS = uniqueList(
  envOrConfigList('COLLECTION_TOKENS', ['collectionTokens', 'collection_tokens', 'collection.tokens'], COLLECTION_TOKEN).concat(
    listTokenFilePath('collectionTokensFile', COLLECTION_TOKENS_FILE)
  )
);
export const APISERVER_SPECIFIC_TOKENS = uniqueList(
  envOrConfigList('APISERVER_TOKENS', ['apiserverTokens', 'apiserver_tokens', 'apiserver.tokens'], APISERVER_TOKEN).concat(
    listTokenFilePath('apiserverTokensFile', APISERVER_TOKENS_FILE)
  )
);
export const COLLECTION_TOKENS = COLLECTION_SPECIFIC_TOKENS.length > 0 ? COLLECTION_SPECIFIC_TOKENS : COMMON_TOKENS;
export const APISERVER_TOKENS = APISERVER_SPECIFIC_TOKENS.length > 0 ? APISERVER_SPECIFIC_TOKENS : COMMON_TOKENS;

export const DURATION = envOrConfigString('DURATION', ['duration'], '10m');
export const QUERY_RPS = intEnv('QUERY_RPS', 120);
export const SUBMIT_RPS = intEnv('SUBMIT_RPS', 60);
export const REPORT_RPS = intEnv('REPORT_RPS', 90);
export const STATS_RPS = intEnv('STATS_RPS', 30);
export const CHAIN_PROBE_RPS = numberEnv('CHAIN_PROBE_RPS', 0);
export const CHAIN_PROBE_MODEL_TYPE = envOrConfigString(
  'CHAIN_PROBE_MODEL_TYPE',
  ['chainProbeModelType', 'chain_probe_model_type'],
  'mixed'
).toLowerCase();
export const SUBMIT_MIX = resolveSubmitMix();
export const USE_SPLIT_QUERY_SCENARIOS = hasGranularQueryQps();
export const USE_SPLIT_REPORT_SCENARIOS = hasGranularReportQps();
export const USE_SPLIT_SUBMIT_SCENARIOS = hasGranularSubmitQps();

export const MEDICAL_QUERY_RPS = USE_SPLIT_QUERY_SCENARIOS ? intEnv('MEDICAL_QUERY_RPS', 0) : 0;
export const PERSONALITY_QUERY_RPS = USE_SPLIT_QUERY_SCENARIOS ? intEnv('PERSONALITY_QUERY_RPS', 0) : 0;
export const QUESTIONNAIRE_DETAIL_RPS = USE_SPLIT_QUERY_SCENARIOS ? intEnv('QUESTIONNAIRE_DETAIL_RPS', 0) : 0;
export const LEGACY_QUERY_RPS = USE_SPLIT_QUERY_SCENARIOS ? 0 : QUERY_RPS;
export const PERSONALITY_SESSION_RPS = intEnv('PERSONALITY_SESSION_RPS', 0);

export const MEDICAL_SUBMIT_RPS = USE_SPLIT_SUBMIT_SCENARIOS ? intEnv('MEDICAL_SUBMIT_RPS', 0) : 0;
export const PERSONALITY_SUBMIT_RPS = USE_SPLIT_SUBMIT_SCENARIOS ? intEnv('PERSONALITY_SUBMIT_RPS', 0) : 0;
export const LEGACY_SUBMIT_RPS = USE_SPLIT_SUBMIT_SCENARIOS ? 0 : SUBMIT_RPS;

export const MEDICAL_REPORT_RPS = USE_SPLIT_REPORT_SCENARIOS ? intEnv('MEDICAL_REPORT_RPS', 0) : 0;
export const PERSONALITY_REPORT_RPS = USE_SPLIT_REPORT_SCENARIOS ? intEnv('PERSONALITY_REPORT_RPS', 0) : 0;
export const LEGACY_REPORT_RPS = USE_SPLIT_REPORT_SCENARIOS ? 0 : REPORT_RPS;

export const CHAIN_PROBE_MEDICAL_RPS = resolveChainProbeRps('medical');
export const CHAIN_PROBE_PERSONALITY_RPS = resolveChainProbeRps('personality');

export const MEDICAL_QUERY_PATHS = envOrConfigList(
  'MEDICAL_QUERY_PATHS',
  ['medicalQueryPaths', 'medical_query_paths', 'paths.medicalQuery', 'paths.medical_query'],
  '/api/v1/scales?page=1&page_size=20&status=published,/api/v1/scales/categories,/api/v1/scales/hot?limit=5,/api/v1/scales/{scale_code}'
);
export const PERSONALITY_QUERY_PATHS = envOrConfigList(
  'PERSONALITY_QUERY_PATHS',
  ['personalityQueryPaths', 'personality_query_paths', 'paths.personalityModelQuery', 'paths.personality_model_query'],
  '/api/v1/personality-models?page=1&page_size=20,/api/v1/personality-models/categories,/api/v1/personality-models/{model_code}'
);
export const QUESTIONNAIRE_DETAIL_PATHS = envOrConfigList(
  'QUESTIONNAIRE_DETAIL_PATHS',
  ['questionnaireDetailPaths', 'questionnaire_detail_paths', 'paths.questionnaireDetail', 'paths.questionnaire_detail'],
  '/api/v1/questionnaires/{questionnaire_code}'
);
export const QUERY_PATHS = envOrConfigList(
  'QUESTIONNAIRE_QUERY_PATHS',
  ['questionnaireQueryPaths', 'questionnaire_query_paths', 'paths.questionnaireQuery', 'paths.questionnaire_query'],
  MEDICAL_QUERY_PATHS.concat(QUESTIONNAIRE_DETAIL_PATHS).concat(PERSONALITY_QUERY_PATHS).join(',')
);
export const STATS_PATHS = envOrConfigList(
  'STATISTICS_PATHS',
  ['statisticsPaths', 'statistics_paths', 'paths.statistics'],
  '/api/v1/statistics/overview?preset=7d,/api/v1/statistics/system,/api/v1/statistics/questionnaires/{questionnaire_code}?preset=7d'
);

export const SUBMIT_PATH = envOrConfigString('SUBMIT_PATH', ['submitPath', 'submit_path', 'paths.submit'], '/api/v1/answersheets');
export const REPORT_STATUS_PATH = envOrConfigString(
  'REPORT_STATUS_PATH',
  ['reportStatusPath', 'report_status_path', 'paths.reportStatus', 'paths.report_status'],
  '/api/v1/assessments/{assessment_id}/wait-report?testee_id={testee_id}&timeout={report_timeout}'
);
export const PERSONALITY_REPORT_STATUS_PATH = envOrConfigString(
  'PERSONALITY_REPORT_STATUS_PATH',
  ['personalityReportStatusPath', 'personality_report_status_path', 'paths.personalityReportStatus', 'paths.personality_report_status'],
  '/api/v1/personality-assessments/{assessment_id}/wait-report?testee_id={testee_id}&timeout={report_timeout}'
);
export const PERSONALITY_REPORT_PATH = envOrConfigString(
  'PERSONALITY_REPORT_PATH',
  ['personalityReportPath', 'personality_report_path', 'paths.personalityReport', 'paths.personality_report'],
  '/api/v1/personality-assessments/{assessment_id}/report?testee_id={testee_id}'
);
export const PERSONALITY_SESSION_PATH = envOrConfigString(
  'PERSONALITY_SESSION_PATH',
  ['personalitySessionPath', 'personality_session_path', 'paths.personalitySession', 'paths.personality_session'],
  '/api/v1/personality-assessment-sessions'
);
export const SUBMIT_STATUS_PATH = envOrConfigString(
  'SUBMIT_STATUS_PATH',
  ['submitStatusPath', 'submit_status_path', 'paths.submitStatus', 'paths.submit_status'],
  '/api/v1/answersheets/submit-status?request_id={request_id}'
);
export const ANSWERSHEET_ASSESSMENT_PATH = envOrConfigString(
  'ANSWERSHEET_ASSESSMENT_PATH',
  ['answersheetAssessmentPath', 'answersheet_assessment_path', 'paths.answersheetAssessment', 'paths.answersheet_assessment'],
  '/api/v1/answersheets/{answersheet_id}/assessment'
);

export const TESTEE_IDS = envOrConfigList('TESTEE_IDS', ['testeeIds', 'testee_ids'], __ENV.TESTEE_ID || '');
export const ASSESSMENT_IDS = envOrConfigList('ASSESSMENT_IDS', ['assessmentIds', 'assessment_ids'], __ENV.ASSESSMENT_ID || '');
export const QUESTIONNAIRE_CODES = envOrConfigList('QUESTIONNAIRE_CODES', ['questionnaireCodes', 'questionnaire_codes'], __ENV.QUESTIONNAIRE_CODE || __ENV.Q_CODE || '');
export const QUESTIONNAIRE_VERSION = __ENV.QUESTIONNAIRE_VERSION || __ENV.Q_VER || configStringValue(['questionnaireVersion', 'questionnaire_version'], '');
export const SCALE_CODES = envOrConfigList('SCALE_CODES', ['scaleCodes', 'scale_codes'], __ENV.SCALE_CODE || SEEDDATA_SCALE_CODES);
export const PERSONALITY_MODEL_CODES = envOrConfigList(
  'PERSONALITY_MODEL_CODES',
  ['personalityModelCodes', 'personality_model_codes'],
  __ENV.PERSONALITY_MODEL_CODE || 'MBTI_OEJTS,SBTI_FUN'
);
export const PLAN_IDS = envOrConfigList('PLAN_IDS', ['planIds', 'plan_ids'], __ENV.PLAN_ID || SEEDDATA_PLAN_IDS);
export const ENTRY_IDS = envOrConfigList('ENTRY_IDS', ['entryIds', 'entry_ids'], __ENV.ENTRY_ID || '');
export const ORG_ID = envOrConfigString('ORG_ID', ['orgId', 'org_id'], '1');
export const TESTEE_SOURCE = envOrConfigString('TESTEE_SOURCE', ['testeeSource', 'testee_source'], 'daily_simulation');
export const DISCOVER_ANSWERS = boolEnv('DISCOVER_ANSWERS', true);
export const AUTO_DISCOVER_SEEDDATA = boolEnv('AUTO_DISCOVER_SEEDDATA', false);
export const DISCOVER_TESTEE_LOOKBACK_DAYS = intEnv('DISCOVER_TESTEE_LOOKBACK_DAYS', 7);
export const DISCOVER_TESTEE_LIMIT = intEnv('DISCOVER_TESTEE_LIMIT', 100);
export const DISCOVER_ASSESSMENT_LIMIT = intEnv('DISCOVER_ASSESSMENT_LIMIT', 100);
export const REPORT_TIMEOUT = intEnv('REPORT_TIMEOUT', 20);
export let STATIC_REPORT_SAMPLES = { medical: [], personality: [] };
export let STATIC_ANSWER_TEMPLATES = [];

export const RUN_ID = envOrConfigString('RUN_ID', ['runId', 'run_id'], `${Date.now()}`);
export const IDEMPOTENCY_PREFIX = envOrConfigString('IDEMPOTENCY_PREFIX', ['idempotencyPrefix', 'idempotency_prefix'], `k6-300qps-${RUN_ID}`);
export const STRICT_THRESHOLDS = boolEnv('STRICT_THRESHOLDS', false);
export const CHAIN_PROBE_TIMEOUT_SECONDS = intEnv('CHAIN_PROBE_TIMEOUT_SECONDS', 120);
export const CHAIN_PROBE_POLL_SECONDS = numberEnv('CHAIN_PROBE_POLL_SECONDS', 1);
export const HTTP_TIMEOUT = envOrConfigString('HTTP_TIMEOUT', ['httpTimeout', 'http_timeout'], '30s');
export const USER_AGENT = envOrConfigString('USER_AGENT', ['userAgent', 'user_agent'], 'qs-server-k6-300qps/1.0');
