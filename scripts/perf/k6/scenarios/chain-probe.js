import { sleep, check } from 'k6';
import { responseItems } from '../lib/util.js';
import { scenarioData, buildMedicalSubmitPayload, buildSubmitPayloadFromCase, buildPersonalityCaseFromSession, renderPath } from '../lib/data.js';
import { timedRequest, authHeaders, jsonHeaders, collectionToken, apiserverToken, responseData, recordHTTPStatus } from '../lib/http.js';
import {
  APISERVER_BASE_URL, COLLECTION_BASE_URL, SUBMIT_PATH, SUBMIT_STATUS_PATH,
  REPORT_STATUS_PATH, PERSONALITY_REPORT_STATUS_PATH, PERSONALITY_REPORT_PATH,
  IDEMPOTENCY_PREFIX, CHAIN_PROBE_TIMEOUT_SECONDS, CHAIN_PROBE_POLL_SECONDS, REPORT_TIMEOUT,
  CHAIN_PROBE_MODEL_TYPE,
} from '../lib/config.js';
import {
  chainProbeFailed, submitToAssessmentLatency, assessmentToReportLatency,
  reportGeneratedLatency, medicalReportGeneratedLatency, personalityReportGeneratedLatency, chainProbeTerminal,
  personalityReportFetchDuration, personalityReportFetchSuccessRate,
} from '../lib/metrics.js';


export function asyncChainProbeMedical(data) {
  runAsyncChainProbe(scenarioData(data), 'medical');
}

export function asyncChainProbePersonality(data) {
  runAsyncChainProbe(scenarioData(data), 'personality');
}

export function asyncChainProbe(data) {
  runAsyncChainProbe(scenarioData(data), CHAIN_PROBE_MODEL_TYPE === 'personality' ? 'personality' : 'medical');
}

export function runAsyncChainProbe(ctx, modelType) {
  const start = Date.now();
  let payload = null;
  if (modelType === 'personality') {
    const personalityCase = buildPersonalityCaseFromSession(ctx, 'chain_probe_personality_session');
    if (!personalityCase) {
      chainProbeFailed.add(1, { reason: 'personality_session_failed', model_type: modelType });
      return;
    }
    payload = buildSubmitPayloadFromCase(personalityCase);
  } else {
    payload = buildMedicalSubmitPayload(ctx);
  }
  if (!payload) {
    chainProbeFailed.add(1, { reason: 'missing_submit_payload', model_type: modelType });
    return;
  }

  const requestID = payload.idempotency_key || `${IDEMPOTENCY_PREFIX}-chain-${modelType}-${__VU}-${__ITER}-${start}`;
  const submitRes = timedRequest('POST', COLLECTION_BASE_URL, SUBMIT_PATH, JSON.stringify(payload), jsonHeaders(collectionToken(), requestID), {
    endpoint: 'chain_probe_submit',
    service: 'collection-server',
    model_type: modelType,
  });

  if (submitRes.status !== 202) {
    chainProbeFailed.add(1, { reason: 'submit_not_accepted', model_type: modelType });
    return;
  }

  const answerSheetID = waitSubmitDone(requestID);
  if (!answerSheetID) {
    chainProbeFailed.add(1, { reason: 'submit_status_timeout', model_type: modelType });
    return;
  }

  const assessmentID = waitAssessmentID(answerSheetID, payload.testee_id, modelType);
  if (!assessmentID) {
    chainProbeFailed.add(1, { reason: 'assessment_lookup_failed', model_type: modelType });
    return;
  }
  submitToAssessmentLatency.add(Date.now() - start, { model_type: modelType });

  const reportPathTemplate = modelType === 'personality' ? PERSONALITY_REPORT_STATUS_PATH : REPORT_STATUS_PATH;
  const assessmentStart = Date.now();
  const terminalStatus = waitReportTerminal(assessmentID, payload.testee_id, ctx, reportPathTemplate, modelType === 'personality' ? 'chain_probe_personality_report_status' : 'chain_probe_report_status');
  if (!terminalStatus) {
    chainProbeFailed.add(1, { reason: 'report_timeout', model_type: modelType });
    return;
  }
  assessmentToReportLatency.add(Date.now() - assessmentStart, { model_type: modelType, assessment_status: terminalStatus });

  const totalLatency = Date.now() - start;
  const latencyTags = {
    endpoint: modelType === 'personality' ? 'async_chain_probe_personality' : 'async_chain_probe_medical',
    service: 'collection-server',
    assessment_status: terminalStatus,
    model_type: modelType,
  };
  reportGeneratedLatency.add(totalLatency, latencyTags);
  if (modelType === 'personality') {
    personalityReportGeneratedLatency.add(totalLatency, latencyTags);
    if (terminalStatus === 'interpreted') {
      const reportPath = renderPath(PERSONALITY_REPORT_PATH, {
        assessment_id: assessmentID,
        testee_id: payload.testee_id,
      }, ctx);
      const reportRes = timedRequest('GET', COLLECTION_BASE_URL, reportPath, null, authHeaders(collectionToken()), {
        endpoint: 'chain_probe_personality_report',
        service: 'collection-server',
        model_type: modelType,
      });
      recordHTTPStatus(reportRes, chainProbeFailed, 'chain_probe_personality_report');
      const reportOk = check(reportRes, { 'personality report fetch status is 200': (r) => r.status === 200 });
      personalityReportFetchSuccessRate.add(reportOk, { model_type: modelType });
      personalityReportFetchDuration.add(reportRes.timings.duration, { model_type: modelType });
      if (!reportOk) {
        chainProbeFailed.add(1, { reason: 'personality_report_fetch_failed', model_type: modelType });
        return;
      }
    }
  } else {
    medicalReportGeneratedLatency.add(totalLatency, latencyTags);
  }
  chainProbeTerminal.add(1, { assessment_status: terminalStatus, model_type: modelType });
}

export function waitSubmitDone(requestID) {
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

export function findAssessmentIDByAnswerSheet(data, answerSheetID) {
  const target = String(answerSheetID);
  for (const item of responseItems(data)) {
    const sheetID = String(item.answer_sheet_id || item.answerSheetId || item.answersheet_id || '');
    if (sheetID && sheetID === target) {
      const assessmentID = String(item.id || item.assessment_id || item.assessmentId || '');
      if (assessmentID) {
        return assessmentID;
      }
      if (item.status === 'failed') {
        return '';
      }
    }
  }
  return null;
}

export function waitAssessmentID(answerSheetID, testeeID, modelType) {
  const deadline = Date.now() + CHAIN_PROBE_TIMEOUT_SECONDS * 1000;
  const personality = modelType === 'personality';
  while (Date.now() < deadline) {
    const path = personality
      ? `/api/v1/typology-assessments?testee_id=${encodeURIComponent(testeeID)}&page=1&page_size=20`
      : `/api/v1/evaluations/assessments?testee_id=${encodeURIComponent(testeeID)}&page=1&page_size=20`;
    const res = timedRequest(
      'GET',
      personality ? COLLECTION_BASE_URL : APISERVER_BASE_URL,
      path,
      null,
      authHeaders(personality ? collectionToken() : apiserverToken()),
      {
        endpoint: personality ? 'chain_probe_typology_assessment_lookup' : 'chain_probe_evaluation_assessment_lookup',
        service: personality ? 'collection-server' : 'qs-apiserver',
        model_type: modelType,
      }
    );
    if (res.status === 200) {
      const assessmentID = findAssessmentIDByAnswerSheet(responseData(res), answerSheetID);
      if (assessmentID === '') {
        return '';
      }
      if (assessmentID) {
        return assessmentID;
      }
    }
    sleep(CHAIN_PROBE_POLL_SECONDS);
  }
  return '';
}

export function waitReportTerminal(assessmentID, testeeID, data, pathTemplate, endpoint) {
  const deadline = Date.now() + CHAIN_PROBE_TIMEOUT_SECONDS * 1000;
  while (Date.now() < deadline) {
    const path = renderPath(pathTemplate || REPORT_STATUS_PATH, {
      assessment_id: assessmentID,
      testee_id: testeeID,
      report_timeout: String(REPORT_TIMEOUT),
    }, data);
    const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
      endpoint: endpoint || 'chain_probe_report_status',
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
