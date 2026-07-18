import { check } from 'k6';
import { pick, is2xx } from '../lib/util.js';
import { scenarioData, renderPath } from '../lib/data.js';
import { timedRequest, authHeaders, jsonHeaders, apiserverToken, recordHTTPStatus } from '../lib/http.js';
import { APISERVER_BASE_URL, STATS_PATHS, STATS_CONTENT_BATCH_PATH } from '../lib/config.js';
import { statisticsDuration, statisticsFailed } from '../lib/metrics.js';

function contentBatchPayload(ctx) {
  const items = [];
  const questionnaireCode = pick(ctx.questionnaireCodes);
  const scaleCode = pick(ctx.scaleCodes);
  if (questionnaireCode) {
    items.push({ type: 'questionnaire', code: questionnaireCode });
  }
  if (scaleCode) {
    items.push({ type: 'scale', code: scaleCode });
  }
  return items.length > 0 ? { items: [pick(items)] } : null;
}

export function statisticsQuery(data) {
  const ctx = scenarioData(data);
  const batchPayload = contentBatchPayload(ctx);
  const operations = STATS_PATHS.map((path) => ({ method: 'GET', path, body: null }));
  if (STATS_CONTENT_BATCH_PATH && batchPayload) {
    operations.push({ method: 'POST', path: STATS_CONTENT_BATCH_PATH, body: JSON.stringify(batchPayload) });
  }
  const operation = pick(operations);
  const path = renderPath(operation.path, null, ctx);
  const endpoint = operation.method === 'POST' ? 'statistics_content_batch' : 'statistics_overview';
  const token = apiserverToken();
  const headers = operation.method === 'POST' ? jsonHeaders(token) : authHeaders(token);
  const res = timedRequest(operation.method, APISERVER_BASE_URL, path, operation.body, headers, {
    endpoint,
    service: 'qs-apiserver',
  });

  statisticsDuration.add(res.timings.duration, res.tags);
  recordHTTPStatus(res, statisticsFailed, endpoint);
  check(res, {
    'statistics request status is 2xx': (r) => is2xx(r.status),
  });
}
