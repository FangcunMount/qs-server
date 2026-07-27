import http from 'k6/http';

import {
  COLLECTION_TOKENS,
  HTTP_TIMEOUT,
  QUESTIONNAIRE_VERSION,
  TESTEE_IDS,
  tokenFileIssueMessage,
} from './config.js';
import { buildAnswersFromQuestionnaire, normalizeAnswers } from './data.js';
import { authHeaders, responseData } from './http.js';

const defaultQuestionnaireCode = 'SNAP-VI';

export function resolveCollectionToken() {
  const token = COLLECTION_TOKENS.length > 0 ? COLLECTION_TOKENS[0] : '';
  if (!token) {
    throw new Error(
      `No collection token found. Run make perf-init and make perf-tokens, or set COLLECTION_TOKEN.${tokenFileIssueMessage()}`
    );
  }
  return token;
}

export function discoverSubmitCases(collectionURLs, requestedCount) {
  if (!Array.isArray(collectionURLs) || collectionURLs.length === 0) {
    throw new Error('At least one collection-server URL is required to discover submit fixtures');
  }
  if (!Number.isInteger(requestedCount) || requestedCount < 1) {
    throw new Error('requestedCount must be a positive integer');
  }
  if (COLLECTION_TOKENS.length < requestedCount) {
    throw new Error(
      `Submit fixture discovery needs ${requestedCount} collection token(s), found ${COLLECTION_TOKENS.length}. Run make perf-tokens with enough collection_users.${tokenFileIssueMessage()}`
    );
  }
  if (TESTEE_IDS.length < requestedCount) {
    throw new Error(
      `Submit fixture discovery needs ${requestedCount} TESTEE_IDS value(s), found ${TESTEE_IDS.length}. Provide IDs in the same order as collection_users/tokens.`
    );
  }

  const questionnaireCode = String(__ENV.QUESTIONNAIRE_CODE || defaultQuestionnaireCode).trim();
  const questionnaireVersion = String(QUESTIONNAIRE_VERSION || '').trim();
  const query = questionnaireVersion
    ? `?version=${encodeURIComponent(questionnaireVersion)}`
    : '';
  const baseURL = String(collectionURLs[0]).replace(/\/+$/, '');
  const token = COLLECTION_TOKENS[0];
  const response = http.get(
    `${baseURL}/api/v1/questionnaires/${encodeURIComponent(questionnaireCode)}${query}`,
    {
      headers: authHeaders(token),
      timeout: HTTP_TIMEOUT,
      responseCallback: http.expectedStatuses(200),
      tags: {
        endpoint: 'discover_submit_questionnaire',
        questionnaire_code: questionnaireCode,
      },
    }
  );
  if (response.status !== 200) {
    throw new Error(
      `Questionnaire discovery failed: code=${questionnaireCode} status=${response.status}. Set QUESTIONNAIRE_CODE/QUESTIONNAIRE_VERSION if SNAP-VI uses a different deployed code.`
    );
  }

  const questionnaire = responseData(response);
  const discoveredCode = String(questionnaire.code || questionnaireCode).trim();
  const discoveredVersion = String(questionnaire.version || questionnaireVersion).trim();
  if (!discoveredCode || !discoveredVersion) {
    throw new Error('Questionnaire discovery response is missing code or version');
  }
  const builtAnswers = buildAnswersFromQuestionnaire(questionnaire);
  if (builtAnswers.length === 0) {
    throw new Error(`Questionnaire ${discoveredCode} has no supported answerable questions`);
  }
  const answers = normalizeAnswers(builtAnswers);

  const cases = [];
  for (let index = 0; index < requestedCount; index += 1) {
    const testeeID = String(TESTEE_IDS[index] || '').trim();
    if (!/^\d+$/.test(testeeID)) {
      throw new Error(`TESTEE_IDS[${index}] must be a uint64 string`);
    }
    cases.push({
      token: COLLECTION_TOKENS[index],
      payload: {
        questionnaire_code: discoveredCode,
        questionnaire_version: discoveredVersion,
        title: questionnaire.title || `${discoveredCode} K6 runtime acceptance`,
        testee_id: testeeID,
        answers: JSON.parse(JSON.stringify(answers)),
      },
    });
  }

  console.log(
    [
      'submit_fixture_discovered',
      `questionnaire=${discoveredCode}`,
      `version=${discoveredVersion}`,
      `questions=${answers.length}`,
      `cases=${cases.length}`,
    ].join(' ')
  );
  return cases;
}
