import { check } from 'k6';
import { scenarioData, weightedPickModelType, buildMedicalSubmitRequest, buildPersonalitySubmitRequest } from '../lib/data.js';
import { timedRequest, jsonHeaders, collectionTokenAt, recordHTTPStatus, responseData } from '../lib/http.js';
import { COLLECTION_BASE_URL, SUBMIT_PATH, SUBMIT_MIX, IDEMPOTENCY_PREFIX } from '../lib/config.js';
import {
  answerSubmitDuration, medicalAnswerSubmitDuration, personalityAnswerSubmitDuration,
  answerSubmitAccepted, answerSubmitFailed,
  answerSubmitSuccessRate, medicalAnswerSubmitSuccessRate, personalityAnswerSubmitSuccessRate,
} from '../lib/metrics.js';


export function answerSubmit(data) {
  const ctx = scenarioData(data);
  const modelType = weightedPickModelType(SUBMIT_MIX, ctx);
  submitAnswerSheet(ctx, modelType);
}

export function medicalAnswerSubmit(data) {
  submitAnswerSheet(scenarioData(data), 'medical');
}

export function personalityAnswerSubmit(data) {
  submitAnswerSheet(scenarioData(data), 'personality');
}

export function submitAnswerSheet(ctx, modelType) {
  let request;
  if (modelType === 'personality') {
    request = buildPersonalitySubmitRequest(ctx);
  } else {
    request = buildMedicalSubmitRequest(ctx);
    modelType = request.modelType;
  }
  const modelDuration = modelType === 'personality' ? personalityAnswerSubmitDuration : medicalAnswerSubmitDuration;
  const modelSuccessRate = modelType === 'personality' ? personalityAnswerSubmitSuccessRate : medicalAnswerSubmitSuccessRate;
  const payload = request.payload;
  if (!payload) {
    answerSubmitFailed.add(1, { reason: 'missing_submit_payload', model_type: modelType });
    answerSubmitSuccessRate.add(false, { model_type: modelType });
    modelSuccessRate.add(false, { model_type: modelType });
    return;
  }
  const requestID = payload.idempotency_key || `${IDEMPOTENCY_PREFIX}-req-${__VU}-${__ITER}-${Date.now()}`;
  const headers = jsonHeaders(collectionTokenAt(request.collectionTokenIndex), requestID);
  const endpoint = 'answersheet_submit';
  const res = timedRequest('POST', COLLECTION_BASE_URL, SUBMIT_PATH, JSON.stringify(payload), headers, {
    endpoint,
    service: 'collection-server',
    model_type: modelType,
  });

  answerSubmitDuration.add(res.timings.duration, res.tags);
  modelDuration.add(res.timings.duration, res.tags);
  const data = responseData(res);
  const accepted = res.status === 202 && data.status === 'accepted' && Boolean(data.answersheet_id);
  if (accepted) {
    answerSubmitAccepted.add(1, res.tags);
  }
  recordHTTPStatus(res, answerSubmitFailed, endpoint);
  answerSubmitSuccessRate.add(accepted, res.tags);
  modelSuccessRate.add(accepted, res.tags);
  check(res, {
    'answersheet submit status is 202': (r) => r.status === 202,
    'answersheet submit is durably accepted': () => accepted,
  });
}
