import { check } from 'k6';
import { pick, is2xx } from '../lib/util.js';
import { timedRequest, authHeaders, jsonHeaders, collectionToken, recordHTTPStatus } from '../lib/http.js';
import { renderPath, scenarioData } from '../lib/data.js';
import {
  QUESTIONNAIRE_DETAIL_PATHS, PERSONALITY_QUESTIONNAIRE_DETAIL_PATHS, QUERY_PATHS, MEDICAL_QUERY_PATHS, PERSONALITY_QUERY_PATHS,
  COLLECTION_BASE_URL, PERSONALITY_SESSION_PATH,
} from '../lib/config.js';
import {
  questionnaireQueryDuration, questionnaireQueryFailed,
  personalityQuestionnaireQueryDuration, personalityQuestionnaireQueryFailed,
  medicalModelQueryDuration, medicalModelQueryFailed,
  personalityModelQueryDuration, personalityModelQueryFailed,
  personalitySessionDuration, personalitySessionFailed,
} from '../lib/metrics.js';


export function questionnaireDetailQuery(data) {
  const ctx = scenarioData(data);
  const path = renderPath(pick(QUESTIONNAIRE_DETAIL_PATHS.length > 0 ? QUESTIONNAIRE_DETAIL_PATHS : QUERY_PATHS), null, ctx);
  runModelCatalogQuery(path, 'questionnaire_query', questionnaireQueryDuration, questionnaireQueryFailed);
}

export function personalityQuestionnaireDetailQuery(data) {
  const ctx = scenarioData(data);
  const personalityCase = pick(ctx.personalityCases);
  if (!personalityCase || !personalityCase.questionnaire_code || !personalityCase.questionnaire_version) {
    personalityQuestionnaireQueryFailed.add(1, { reason: 'missing_personality_questionnaire_case' });
    return;
  }
  const path = renderPath(pick(PERSONALITY_QUESTIONNAIRE_DETAIL_PATHS), {
    personality_questionnaire_code: encodeURIComponent(personalityCase.questionnaire_code),
    personality_questionnaire_version: encodeURIComponent(personalityCase.questionnaire_version),
  }, ctx);
  runModelCatalogQuery(path, 'personality_questionnaire_query', personalityQuestionnaireQueryDuration, personalityQuestionnaireQueryFailed);
}

export function medicalModelQuery(data) {
  const ctx = scenarioData(data);
  const path = renderPath(pick(MEDICAL_QUERY_PATHS), null, ctx);
  runModelCatalogQuery(path, 'medical_model_query', medicalModelQueryDuration, medicalModelQueryFailed);
}

export function personalityModelQuery(data) {
  const ctx = scenarioData(data);
  const path = renderPath(pick(PERSONALITY_QUERY_PATHS), { model_code: pick(ctx.modelCodes) }, ctx);
  runModelCatalogQuery(path, 'personality_model_query', personalityModelQueryDuration, personalityModelQueryFailed);
}

export function personalitySession(data) {
  const ctx = scenarioData(data);
  const modelCode = pick(ctx.modelCodes);
  const testeeID = pick(ctx.testeeIDs);
  if (!modelCode || !testeeID) {
    personalitySessionFailed.add(1, { reason: 'missing_model_or_testee' });
    return;
  }
  const endpoint = 'personality_session';
  const res = timedRequest(
    'POST',
    COLLECTION_BASE_URL,
    PERSONALITY_SESSION_PATH,
    JSON.stringify({ model_code: modelCode, testee_id: testeeID }),
    jsonHeaders(collectionToken()),
    { endpoint, service: 'collection-server' }
  );
  personalitySessionDuration.add(res.timings.duration, res.tags);
  recordHTTPStatus(res, personalitySessionFailed, endpoint);
  check(res, {
    'personality session status is 200': (r) => r.status === 200,
  });
}

export function questionnaireQuery(data) {
  questionnaireDetailQuery(data);
}

export function runModelCatalogQuery(path, endpoint, durationTrend, failedCounter) {
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
    endpoint,
    service: 'collection-server',
  });
  durationTrend.add(res.timings.duration, res.tags);
  recordHTTPStatus(res, failedCounter, endpoint);
  check(res, {
    'model catalog query status is 2xx': (r) => is2xx(r.status),
  });
}
