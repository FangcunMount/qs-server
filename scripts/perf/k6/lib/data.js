import { pick, clone, nonEmptyList, uniqueList, uniqueReportSamples, responseItems, dateStringDaysAgo, is2xx, readTextFile, timeSnapshot, addDurationMs } from './util.js';
import { setupDiscoveryFailed } from './metrics.js';
import { timedRequest, authHeaders, jsonHeaders, collectionToken, getCollectionData, getApiserverData, recordHTTPStatus, responseData } from './http.js';

import {
  DISCOVER_TESTEE_LOOKBACK_DAYS,
  CHAIN_PROBE_PERSONALITY_RPS,
  DISCOVER_ASSESSMENT_LIMIT,
  QUESTIONNAIRE_DETAIL_RPS,
  PERSONALITY_QUESTIONNAIRE_DETAIL_RPS,
  PERSONALITY_SESSION_PATH,
  PERSONALITY_SESSION_RPS,
  PERSONALITY_MODEL_CODES,
  CHAIN_PROBE_MEDICAL_RPS,
  AUTO_DISCOVER_SEEDDATA,
  CHAIN_PROBE_MODEL_TYPE,
  PERSONALITY_REPORT_RPS,
  PERSONALITY_SUBMIT_RPS,
  PERSONALITY_QUERY_RPS,
  QUESTIONNAIRE_VERSION,
  DISCOVER_TESTEE_LIMIT,
  tokenFileIssueMessage,
  QUESTIONNAIRE_CODES,
  COLLECTION_BASE_URL,
  IDEMPOTENCY_PREFIX,
  MEDICAL_SUBMIT_RPS,
  APISERVER_BASE_URL,
  MEDICAL_REPORT_RPS,
  SCRIPT_INIT_AT_MS,
  COLLECTION_TOKENS,
  MEDICAL_QUERY_RPS,
  envOrConfigString,
  LEGACY_REPORT_RPS,
  LEGACY_SUBMIT_RPS,
  APISERVER_TOKENS,
  COLLECTION_TOKEN,
  LEGACY_QUERY_RPS,
  configFirstValue,
  DISCOVER_ANSWERS,
  APISERVER_TOKEN,
  ASSESSMENT_IDS,
  REPORT_TIMEOUT,
  TESTEE_SOURCE,
  DEBUG_SETUP,
  QPS_PROFILE,
  SCALE_CODES,
  configFileBaseDirs,
  TOKENS_FILE,
  SUBMIT_MIX,
  TESTEE_IDS,
  ENTRY_IDS,
  STATS_RPS,
  PLAN_IDS,
  boolEnv,
  RUN_ID,
  ORG_ID,
  TOKEN,
  REPORT_MODE,
  REPORT_WEBSOCKET,
  REPORT_VUSER_DEFAULTS,
} from './config.js';

let staticReportSamples = { medical: [], behavior: [], personality: [] };
let staticAnswerTemplates = [];
let staticPersonalityCases = [];

export function scenarioData(data) {
  const fallbackTesteeIDs = TESTEE_IDS;
  const fallbackQuestionnaireCodes = QUESTIONNAIRE_CODES;
  const fallbackReportSamples = normalizeReportSamples(staticReportSamples);
  const fallbackMedicalCases = staticAnswerTemplates.map((item) => normalizeMedicalCase(item)).filter(Boolean);
  const fallbackPersonalityCases = staticPersonalityCases;
  return {
    testeeIDs: nonEmptyList(data && data.testeeIDs, fallbackTesteeIDs),
    questionnaireCodes: nonEmptyList(data && data.questionnaireCodes, fallbackQuestionnaireCodes),
    personalityQuestionnaireCodes: nonEmptyList(data && data.personalityQuestionnaireCodes, []),
    scaleCodes: nonEmptyList(data && data.scaleCodes, SCALE_CODES),
    modelCodes: nonEmptyList(data && data.modelCodes, PERSONALITY_MODEL_CODES),
    reportSamples: normalizeReportSamples(data && data.reportSamples, fallbackReportSamples),
    medicalCases: nonEmptyList(data && data.medicalCases, fallbackMedicalCases),
    personalityCases: nonEmptyList(data && data.personalityCases, fallbackPersonalityCases),
    answerTemplates: nonEmptyList(data && data.medicalCases, fallbackMedicalCases),
  };
}

export function validateScenarioData(data) {
  const submitRps = LEGACY_SUBMIT_RPS + MEDICAL_SUBMIT_RPS + PERSONALITY_SUBMIT_RPS;
  const reportRps = LEGACY_REPORT_RPS + MEDICAL_REPORT_RPS + PERSONALITY_REPORT_RPS;
  const chainProbeRps = CHAIN_PROBE_MEDICAL_RPS + CHAIN_PROBE_PERSONALITY_RPS;
  if ((submitRps > 0 || chainProbeRps > 0) && COLLECTION_TOKENS.length === 0) {
    throw new Error(`TOKEN, TOKENS, TOKENS_FILE, COLLECTION_TOKEN, COLLECTION_TOKENS or a valid collectionTokensFile is required for answersheet submit.${tokenFileIssueMessage()}`);
  }
  if (STATS_RPS > 0 && APISERVER_TOKENS.length === 0) {
    throw new Error(`TOKEN, TOKENS, TOKENS_FILE, APISERVER_TOKEN, APISERVER_TOKENS or a valid apiserverTokensFile is required for statistics query.${tokenFileIssueMessage()}`);
  }
  if ((reportRps > 0 || chainProbeRps > 0) && COLLECTION_TOKENS.length === 0) {
    throw new Error(`TOKEN, TOKENS, TOKENS_FILE, COLLECTION_TOKEN, COLLECTION_TOKENS or a valid collectionTokensFile is required for report status query.${tokenFileIssueMessage()}`);
  }
  const needsMedicalCases = LEGACY_SUBMIT_RPS > 0 || MEDICAL_SUBMIT_RPS > 0 || CHAIN_PROBE_MEDICAL_RPS > 0;
  const needsPersonalityCases = PERSONALITY_SUBMIT_RPS > 0 || CHAIN_PROBE_PERSONALITY_RPS > 0 || PERSONALITY_SESSION_RPS > 0 || PERSONALITY_QUESTIONNAIRE_DETAIL_RPS > 0;
  if (needsMedicalCases && data.medicalCases.length === 0) {
    throw new Error('No medical answer templates found. Set ANSWERS_JSON/ANSWERS_FILE, or provide valid collection tokens and SCALE_CODES for auto discovery. Check setup_discovery_failed plus http_401_total/http_403_total/http_5xx_total in the k6 summary.');
  }
  if (needsPersonalityCases && data.personalityCases.length === 0) {
    throw new Error('No personality cases found. Set PERSONALITY_MODEL_CODES with discoverAnswers=true, or provide personalityCases / personalityCasesFile in config.');
  }
  if ((submitRps > 0 || chainProbeRps > 0) && data.testeeIDs.length === 0) {
    throw new Error(
      'No testee IDs found. Set TESTEE_IDS, or ensure AUTO_DISCOVER_SEEDDATA=true with apiserverTokensFile. '
      + 'If apiserver testees returns 200 in preflight but setup is empty, try TESTEE_SOURCE= or increase discover.testeeLookbackDays in qs-perf.config.json. '
      + 'Run with DEBUG_SETUP=true to see discover HTTP statuses.'
    );
  }
  if (LEGACY_REPORT_RPS > 0 && flattenReportSamples(data.reportSamples).length === 0) {
    throw new Error('No report samples found. Set ASSESSMENT_IDS/REPORT_SAMPLES_FILE or run with AUTO_DISCOVER_SEEDDATA=true.');
  }
  if (MEDICAL_REPORT_RPS > 0 && data.reportSamples.medical.length === 0) {
    throw new Error('No medical report samples found. Set medical report samples in config or run with AUTO_DISCOVER_SEEDDATA=true.');
  }
  if (PERSONALITY_REPORT_RPS > 0 && data.reportSamples.personality.length === 0) {
    throw new Error('No personality report samples found. Enable chain probe to generate samples, or run with AUTO_DISCOVER_SEEDDATA=true.');
  }
}

export function weightedPickModelType(mix, ctx) {
  if (!ctx || !ctx.personalityCases || ctx.personalityCases.length === 0) {
    return 'medical';
  }
  const total = mix.medical + mix.personality;
  if (total <= 0) {
    return 'medical';
  }
  const roll = Math.random() * total;
  return roll < mix.medical ? 'medical' : 'personality';
}

export function normalizeMedicalCase(item) {
  if (!item) {
    return null;
  }
  const questionnaireCode = String(item.questionnaire_code || item.questionnaireCode || '');
  if (!questionnaireCode) {
    return null;
  }
  return {
    model_type: normalizeExecutionModelType(item.model_type || item.modelType || item.kind || 'medical'),
    scale_code: String(item.scale_code || item.scaleCode || ''),
    questionnaire_code: questionnaireCode,
    questionnaire_version: String(item.questionnaire_version || item.questionnaireVersion || QUESTIONNAIRE_VERSION || ''),
    title: item.title || '',
    testee_id: String(item.testee_id || item.testeeId || ''),
    answers: item.answers || [],
  };
}

export function normalizeExecutionModelType(raw) {
  const value = String(raw || '').trim().toLowerCase();
  if (value === 'behavior' || value === 'behavior_ability' || value === 'behavioral_rating' || value === 'cognitive') {
    return 'behavior';
  }
  if (value === 'personality' || value === 'typology') {
    return 'personality';
  }
  return 'medical';
}

export function normalizePersonalityCase(item) {
  if (!item) {
    return null;
  }
  const questionnaireCode = String(item.questionnaire_code || item.questionnaireCode || '');
  if (!questionnaireCode) {
    return null;
  }
  return {
    model_type: 'personality',
    model_code: String(item.model_code || item.modelCode || ''),
    questionnaire_code: questionnaireCode,
    questionnaire_version: String(item.questionnaire_version || item.questionnaireVersion || ''),
    submit_contract: item.submit_contract || item.submitContract || {},
    endpoints: item.endpoints || {},
    title: item.title || '',
    testee_id: String(item.testee_id || item.testeeId || ''),
    answers: item.answers || [],
  };
}

export function normalizeReportSamples(raw, fallback) {
  if (!raw) {
    return fallback || { medical: [], behavior: [], personality: [] };
  }
  if (Array.isArray(raw)) {
    const medical = [];
    const behavior = [];
    const personality = [];
    raw.forEach((item) => {
      const sample = normalizeReportSample(item);
      if (!sample) {
        return;
      }
      if (sample.model_type === 'personality') {
        personality.push(sample);
      } else if (sample.model_type === 'behavior') {
        behavior.push(sample);
      } else {
        medical.push(sample);
      }
    });
    return { medical, behavior, personality };
  }
  if (raw.medical || raw.behavior || raw.personality) {
    return {
      medical: (raw.medical || []).map((item) => normalizeReportSample(item, 'medical')).filter(Boolean),
      behavior: (raw.behavior || []).map((item) => normalizeReportSample(item, 'behavior')).filter(Boolean),
      personality: (raw.personality || []).map((item) => normalizeReportSample(item, 'personality')).filter(Boolean),
    };
  }
  return fallback || { medical: [], behavior: [], personality: [] };
}

export function normalizeReportSample(item, defaultModelType) {
  if (!item) {
    return null;
  }
  const assessmentID = String(item.assessment_id || item.assessmentId || item.id || '');
  const testeeID = String(item.testee_id || item.testeeId || pick(TESTEE_IDS));
  if (!assessmentID || !testeeID) {
    return null;
  }
  return {
    model_type: normalizeExecutionModelType(item.model_type || item.modelType || defaultModelType || 'medical'),
    assessment_id: assessmentID,
    testee_id: testeeID,
  };
}

export function pickReportSample(samples) {
  if (!samples || samples.length === 0) {
    return null;
  }
  return pick(samples);
}

export function flattenReportSamples(reportSamples) {
  const normalized = normalizeReportSamples(reportSamples);
  return normalized.medical.concat(normalized.behavior).concat(normalized.personality);
}
export function buildMedicalSubmitRequest(data) {
  const template = clone(pick(data.medicalCases.length > 0 ? data.medicalCases : data.answerTemplates));
  return {
    modelType: normalizeExecutionModelType(template && template.model_type),
    payload: buildSubmitPayloadFromCase(template),
  };
}
export function buildMedicalSubmitPayload(data) {
  return buildMedicalSubmitRequest(data).payload;
}

export function buildPersonalitySubmitPayload(data) {
  const template = clone(pick(data.personalityCases));
  return buildSubmitPayloadFromCase(template);
}

export function buildAnswerPayload(data) {
  return buildMedicalSubmitPayload(data);
}

export function buildSubmitPayloadFromCase(template) {
  if (!template) {
    return null;
  }
  const testeeID = String(template.testee_id || template.testeeId || '');
  const questionnaireCode = template.questionnaire_code || template.questionnaireCode || '';
  const questionnaireVersion = template.questionnaire_version || template.questionnaireVersion || QUESTIONNAIRE_VERSION || '1.0';
  if (!testeeID || !questionnaireCode) {
    return null;
  }

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

export function normalizeAnswers(answers) {
  const source = Array.isArray(answers) && answers.length > 0 ? answers : defaultAnswers();
  return source.map((answer) => ({
    question_code: answer.question_code || answer.questionCode || 'Q1',
    question_type: answer.question_type || answer.questionType || 'Radio',
    value: stringifyAnswerValue(answer.value === undefined ? 'A' : answer.value),
    score: Number(answer.score || 0),
  }));
}

export function buildAnswersFromQuestionnaire(detail) {
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

export function buildAnswerForQuestion(question, index) {
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

export function answer(question, questionType, value) {
  return {
    question_code: question.code || question.question_code || '',
    question_type: questionType,
    score: 0,
    value,
  };
}

export function resolveQuestionType(question) {
  const normalized = normalizeQuestionType(question.type || question.question_type || '');
  if (['radio', 'checkbox', 'text', 'textarea', 'number', 'section'].indexOf(normalized) >= 0) {
    return normalized.charAt(0).toUpperCase() + normalized.slice(1);
  }
  const options = Array.isArray(question.options) ? question.options : [];
  return options.length > 0 ? 'Radio' : 'Section';
}

export function normalizeQuestionType(raw) {
  return String(raw || '').trim().toLowerCase();
}

export function buildTextAnswer(question, index) {
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

export function buildNumberAnswer(question, index) {
  const minValue = floatRuleValue(question, 'min_value', 1);
  const maxValue = Math.max(minValue, floatRuleValue(question, 'max_value', 100));
  const rangeSize = Math.floor(maxValue - minValue) + 1;
  if (rangeSize <= 1) {
    return minValue;
  }
  return minValue + (index % rangeSize);
}

export function hasRequiredRule(question) {
  return stringRuleValue(question, 'required', '') === 'true';
}

export function intRuleValue(question, ruleType, fallback) {
  const raw = stringRuleValue(question, ruleType, '');
  if (!raw) {
    return fallback;
  }
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? Math.floor(parsed) : fallback;
}

export function floatRuleValue(question, ruleType, fallback) {
  const raw = stringRuleValue(question, ruleType, '');
  if (!raw) {
    return fallback;
  }
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function stringRuleValue(question, ruleType, fallback) {
  const rules = Array.isArray(question.validation_rules) ? question.validation_rules : [];
  const matched = rules.find((rule) => String(rule.rule_type || '').trim() === ruleType);
  if (!matched) {
    return fallback;
  }
  return String(matched.target_value || '').trim();
}

export function normalizeTextLength(value, minLength, maxLength) {
  let text = String(value || '');
  while ([...text].length < minLength) {
    text += '测';
  }
  if (maxLength > 0 && [...text].length > maxLength) {
    text = [...text].slice(0, maxLength).join('');
  }
  return text;
}

export function matchesPattern(value, pattern) {
  try {
    return new RegExp(pattern).test(value);
  } catch (_) {
    return true;
  }
}

export function stringifyAnswerValue(value) {
  if (typeof value === 'string') {
    return value;
  }
  return JSON.stringify(value);
}

export function defaultAnswers() {
  return [
    { question_code: __ENV.DEFAULT_QUESTION_CODE || 'Q1', question_type: __ENV.DEFAULT_QUESTION_TYPE || 'Radio', value: __ENV.DEFAULT_ANSWER_VALUE || 'A' },
  ];
}

export function loadAnswerTemplates() {
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

export function loadPersonalityCases() {
  const casesFile = envOrConfigString('PERSONALITY_CASES_FILE', ['personalityCasesFile', 'personality_cases_file'], '');
  if (casesFile) {
    const parsed = JSON.parse(readTextFile(casesFile, configFileBaseDirs()).content);
    const list = Array.isArray(parsed) ? parsed : [parsed];
    return list.map((item) => normalizePersonalityCase(item)).filter(Boolean);
  }
  const casesJSON = __ENV.PERSONALITY_CASES_JSON || configFirstValue(['personalityCases', 'personality_cases']);
  if (casesJSON) {
    const parsed = typeof casesJSON === 'string' ? JSON.parse(casesJSON) : casesJSON;
    const list = Array.isArray(parsed) ? parsed : [parsed];
    return list.map((item) => normalizePersonalityCase(item)).filter(Boolean);
  }
  return [];
}

export function loadReportSamples() {
  const reportSamplesFile = envOrConfigString('REPORT_SAMPLES_FILE', ['reportSamplesFile', 'report_samples_file'], '');
  if (reportSamplesFile) {
    const parsed = JSON.parse(readTextFile(reportSamplesFile, configFileBaseDirs()).content);
    return normalizeReportSamples(parsed);
  }
  const reportSamples = configFirstValue(['reportSamples', 'report_samples']);
  if (reportSamples) {
    return normalizeReportSamples(reportSamples);
  }
  if (ASSESSMENT_IDS.length === 0) {
    return { medical: [], behavior: [], personality: [] };
  }
  return {
    medical: ASSESSMENT_IDS.map((assessmentID, index) => ({
      model_type: 'medical',
      assessment_id: String(assessmentID),
      testee_id: String(TESTEE_IDS[index % TESTEE_IDS.length]),
    })).filter((item) => item.assessment_id && item.testee_id),
    behavior: [],
    personality: [],
  };
}

export function hydrateStaticFixtures() {
  staticReportSamples = loadReportSamples();
  staticAnswerTemplates = loadAnswerTemplates();
  staticPersonalityCases = loadPersonalityCases();
}

export function discoverMedicalCases(testeeIDs) {
  const fromStatic = staticAnswerTemplates.length > 0 ? staticAnswerTemplates.map((item) => normalizeMedicalCase(item)).filter(Boolean) : [];
  const questionnaireCodes = uniqueList(QUESTIONNAIRE_CODES.concat(fromStatic.map((item) => String(item.questionnaire_code || ''))));
  if (!DISCOVER_ANSWERS) {
    return { questionnaireCodes, cases: fromStatic };
  }
  if (COLLECTION_TOKENS.length === 0 && fromStatic.length > 0) {
    return { questionnaireCodes, cases: fromStatic };
  }
  if (COLLECTION_TOKENS.length === 0) {
    return { questionnaireCodes, cases: fromStatic };
  }

  const discovered = [];
  const scaleQuestionnaireCodes = [];
  const questionnaireModelTypes = {};
  SCALE_CODES.forEach((scaleCode) => {
    const scale = getCollectionData(`/api/v1/assessment-models/${encodeURIComponent(scaleCode)}`, 'discover_assessment_model');
    if (!scale) {
      return;
    }
    const qCode = String(scale.questionnaire_code || scale.questionnaireCode || '');
    if (qCode) {
      scaleQuestionnaireCodes.push(qCode);
      questionnaireModelTypes[qCode] = normalizeExecutionModelType(scale.kind || scale.model_kind || scale.modelKind);
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
    discovered.push(normalizeMedicalCase({
      model_type: questionnaireModelTypes[qCode] || 'medical',
      scale_code: '',
      questionnaire_code: detail.code || qCode,
      questionnaire_version: detail.version || QUESTIONNAIRE_VERSION || '',
      title: detail.title || envOrConfigString('ANSWERSHEET_TITLE', ['answersheetTitle', 'answersheet_title'], 'k6 300qps mixed scenario'),
      testee_id: pick(testeeIDs),
      answers,
    }));
  });

  return {
    questionnaireCodes: uniqueList(questionnaireCodes.concat(scaleQuestionnaireCodes).concat(discovered.map((item) => item.questionnaire_code))),
    cases: fromStatic.concat(discovered),
  };
}

export function discoverPersonalityCases(testeeIDs) {
  const cases = [];
  const modelCodes = uniqueList(PERSONALITY_MODEL_CODES);
  if (!DISCOVER_ANSWERS || COLLECTION_TOKENS.length === 0 || testeeIDs.length === 0) {
    return { modelCodes, questionnaireCodes: [], cases };
  }

  const discoveredModelCodes = [];
  const models = getCollectionData('/api/v1/typology-models?page=1&page_size=50', 'discover_personality_models');
  responseItems(models).forEach((item) => {
    const code = String(item.code || '');
    if (code) {
      discoveredModelCodes.push(code);
    }
  });

  uniqueList(modelCodes.concat(discoveredModelCodes)).forEach((modelCode) => {
    const personalityCase = buildPersonalityCaseFromSession({ testeeIDs, modelCodes: [modelCode] }, 'discover_personality_session', modelCode);
    if (personalityCase) {
      cases.push(personalityCase);
    }
  });

  return {
    modelCodes: uniqueList(modelCodes.concat(discoveredModelCodes)),
    questionnaireCodes: uniqueList(cases.map((item) => item.questionnaire_code)),
    cases,
  };
}

export function buildPersonalityCaseFromSession(ctx, endpoint, forcedModelCode) {
  const modelCode = forcedModelCode || pick(ctx.modelCodes);
  const testeeID = pick(ctx.testeeIDs);
  if (!modelCode || !testeeID) {
    return null;
  }
  const res = timedRequest(
    'POST',
    COLLECTION_BASE_URL,
    PERSONALITY_SESSION_PATH,
    JSON.stringify({ model_code: modelCode, testee_id: testeeID }),
    jsonHeaders(collectionToken()),
    { endpoint, service: 'collection-server' }
  );
  if (!is2xx(res.status)) {
    recordHTTPStatus(res, setupDiscoveryFailed, endpoint);
    return null;
  }
  const session = responseData(res);
  const questionnaire = session.questionnaire || {};
  const submitContract = session.submit_contract || session.submitContract || {};
  const answers = buildAnswersFromQuestionnaire(questionnaire);
  if (answers.length === 0) {
    return null;
  }
  return normalizePersonalityCase({
    model_type: 'personality',
    model_code: modelCode,
    questionnaire_code: submitContract.questionnaire_code || submitContract.questionnaireCode || questionnaire.code || '',
    questionnaire_version: submitContract.questionnaire_version || submitContract.questionnaireVersion || questionnaire.version || '',
    submit_contract: submitContract,
    endpoints: session.endpoints || {},
    title: questionnaire.title || envOrConfigString('ANSWERSHEET_TITLE', ['answersheetTitle', 'answersheet_title'], 'k6 personality assessment'),
    testee_id: String(submitContract.testee_id || submitContract.testeeId || testeeID),
    answers,
  });
}

export function discoverQuestionnairesAndAnswers(testeeIDs) {
  const bundle = discoverMedicalCases(testeeIDs);
  return { questionnaireCodes: bundle.questionnaireCodes, answerTemplates: bundle.cases };
}

export function appendTesteesFromResponse(data, out, requireSourceMatch) {
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

export function discoverTesteeIDs() {
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

export function discoverReportSamples(testeeIDs) {
  if (staticReportSamples.medical.length > 0 || staticReportSamples.behavior.length > 0 || staticReportSamples.personality.length > 0 || !AUTO_DISCOVER_SEEDDATA || APISERVER_TOKENS.length === 0) {
    return staticReportSamples;
  }
  return {
    medical: discoverMedicalReportSamples(testeeIDs),
    behavior: discoverBehaviorReportSamples(testeeIDs),
    personality: discoverPersonalityReportSamples(testeeIDs),
  };
}

export function discoverMedicalReportSamples(testeeIDs) {
  const out = [];
  testeeIDs.slice(0, Math.min(testeeIDs.length, DISCOVER_TESTEE_LIMIT)).forEach((testeeID) => {
    if (out.length >= DISCOVER_ASSESSMENT_LIMIT) {
      return;
    }
    const data = getApiserverData(`/api/v1/evaluations/assessments?testee_id=${encodeURIComponent(testeeID)}&page=1&page_size=20`, 'discover_assessments');
    responseItems(data).forEach((item) => {
      const model = item.model || {};
      if (normalizeExecutionModelType(model.kind || item.model_kind || item.modelKind) === 'behavior') {
        return;
      }
      const assessmentID = String(item.id || item.assessment_id || item.assessmentId || '');
      const sampleTesteeID = String(item.testee_id || item.testeeId || testeeID);
      if (assessmentID && sampleTesteeID) {
        out.push({ model_type: 'medical', assessment_id: assessmentID, testee_id: sampleTesteeID });
      }
    });
  });
  return uniqueReportSamples(out).slice(0, DISCOVER_ASSESSMENT_LIMIT);
}

export function discoverBehaviorReportSamples(testeeIDs) {
  const out = [];
  if (COLLECTION_TOKENS.length === 0) {
    return out;
  }
  testeeIDs.slice(0, Math.min(testeeIDs.length, DISCOVER_TESTEE_LIMIT)).forEach((testeeID) => {
    if (out.length >= DISCOVER_ASSESSMENT_LIMIT) {
      return;
    }
    const data = getCollectionData(`/api/v1/behavior-assessments?testee_id=${encodeURIComponent(testeeID)}&page=1&page_size=20`, 'discover_behavior_assessments');
    responseItems(data).forEach((item) => {
      const assessmentID = String(item.id || item.assessment_id || item.assessmentId || '');
      const sampleTesteeID = String(item.testee_id || item.testeeId || testeeID);
      if (assessmentID && sampleTesteeID) {
        out.push({ model_type: 'behavior', assessment_id: assessmentID, testee_id: sampleTesteeID });
      }
    });
  });
  return uniqueReportSamples(out).slice(0, DISCOVER_ASSESSMENT_LIMIT);
}

export function discoverPersonalityReportSamples(testeeIDs) {
  const out = [];
  if (COLLECTION_TOKENS.length === 0) {
    return out;
  }
  testeeIDs.slice(0, Math.min(testeeIDs.length, DISCOVER_TESTEE_LIMIT)).forEach((testeeID) => {
    if (out.length >= DISCOVER_ASSESSMENT_LIMIT) {
      return;
    }
    const data = getCollectionData(`/api/v1/typology-assessments?testee_id=${encodeURIComponent(testeeID)}&page=1&page_size=20`, 'discover_personality_assessments');
    responseItems(data).forEach((item) => {
      const assessmentID = String(item.id || item.assessment_id || item.assessmentId || '');
      const sampleTesteeID = String(item.testee_id || item.testeeId || testeeID);
      if (assessmentID && sampleTesteeID) {
        out.push({ model_type: 'personality', assessment_id: assessmentID, testee_id: sampleTesteeID });
      }
    });
  });
  return uniqueReportSamples(out).slice(0, DISCOVER_ASSESSMENT_LIMIT);
}

export function buildRunTiming() {
  const setupStartAtMs = Date.now();
  const reportRps = LEGACY_REPORT_RPS;
  const medicalReportRps = MEDICAL_REPORT_RPS;
  const personalityReportRps = PERSONALITY_REPORT_RPS;
  return {
    runId: RUN_ID,
    profile: QPS_PROFILE || '<none>',
    report_mode: REPORT_MODE,
    report_vuser_defaults: REPORT_VUSER_DEFAULTS,
    scriptInitAtMs: SCRIPT_INIT_AT_MS,
    setupStartAtMs,
    trafficStartAtMs: null,
    trafficPlannedEndAtMs: null,
    qps: {
      medical_model_query: MEDICAL_QUERY_RPS,
      personality_model_query: PERSONALITY_QUERY_RPS,
      questionnaire_query: QUESTIONNAIRE_DETAIL_RPS || LEGACY_QUERY_RPS,
      personality_questionnaire_query: PERSONALITY_QUESTIONNAIRE_DETAIL_RPS,
      personality_session: PERSONALITY_SESSION_RPS,
      answersheet_submit: LEGACY_SUBMIT_RPS,
      medical_submit: MEDICAL_SUBMIT_RPS,
      personality_submit: PERSONALITY_SUBMIT_RPS,
      report_status_query: REPORT_WEBSOCKET ? 0 : reportRps,
      medical_report_status_query: REPORT_WEBSOCKET ? 0 : medicalReportRps,
      personality_report_status_query: REPORT_WEBSOCKET ? 0 : personalityReportRps,
      report_ws_query: REPORT_WEBSOCKET ? reportRps : 0,
      medical_report_ws_query: REPORT_WEBSOCKET ? medicalReportRps : 0,
      personality_report_ws_query: REPORT_WEBSOCKET ? personalityReportRps : 0,
      statistics_query: STATS_RPS,
      async_chain_probe_medical: CHAIN_PROBE_MEDICAL_RPS,
      async_chain_probe_personality: CHAIN_PROBE_PERSONALITY_RPS,
      submit_mix: SUBMIT_MIX,
      chain_probe_model_type: CHAIN_PROBE_MODEL_TYPE,
    },
    baseUrls: {
      collection: COLLECTION_BASE_URL,
      apiserver: APISERVER_BASE_URL,
    },
  };
}

export function logPerfTimeEvent(event, atMs, extra) {
  const record = Object.assign(
    {
      event,
      run_id: RUN_ID,
      profile: QPS_PROFILE || '<none>',
      at: timeSnapshot(atMs),
    },
    extra || {}
  );
  console.log(`[perf-time] ${JSON.stringify(record)}`);
}

export function renderPath(path, values, data) {
  const ctx = scenarioData(data);
  const vars = Object.assign(
    {
      testee_id: pick(ctx.testeeIDs),
      assessment_id: pick(ASSESSMENT_IDS),
      questionnaire_code: pick(ctx.questionnaireCodes),
      scale_code: pick(ctx.scaleCodes),
      model_code: pick(ctx.modelCodes),
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
