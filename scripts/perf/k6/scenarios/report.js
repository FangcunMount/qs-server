import { check, sleep } from 'k6';
import { scenarioData, pickReportSample, flattenReportSamples, renderPath } from '../lib/data.js';
import { timedRequest, authHeaders, collectionToken, recordHTTPStatus, responseData } from '../lib/http.js';
import {
  COLLECTION_BASE_URL, REPORT_STATUS_PATH, PERSONALITY_REPORT_STATUS_PATH, REPORT_TIMEOUT, REPORT_SHORT_POLL,
} from '../lib/config.js';
import {
  reportStatusDuration, reportStatusFailed, medicalReportStatusDuration, medicalReportStatusFailed,
  personalityReportStatusDuration, personalityReportStatusFailed,
  reportStatusSuccessRate, reportStatusTerminal, reportStatusPending,
} from '../lib/metrics.js';


export function reportStatusQuery(data) {
  const ctx = scenarioData(data);
  const sample = pickReportSample(flattenReportSamples(ctx.reportSamples));
  const pathTemplate = sample && sample.model_type === 'personality' ? PERSONALITY_REPORT_STATUS_PATH : REPORT_STATUS_PATH;
  runReportStatusQuery(ctx, sample, pathTemplate, 'report_status_query', reportStatusDuration, reportStatusFailed);
}

export function medicalReportStatusQuery(data) {
  const ctx = scenarioData(data);
  runReportStatusQuery(ctx, pickReportSample(ctx.reportSamples.medical), REPORT_STATUS_PATH, 'medical_report_status_query', medicalReportStatusDuration, medicalReportStatusFailed);
}

export function personalityReportStatusQuery(data) {
  const ctx = scenarioData(data);
  runReportStatusQuery(
    ctx,
    pickReportSample(ctx.reportSamples.personality),
    PERSONALITY_REPORT_STATUS_PATH,
    'personality_report_status_query',
    personalityReportStatusDuration,
    personalityReportStatusFailed
  );
}

export function runReportStatusQuery(ctx, sample, pathTemplate, endpoint, durationTrend, failedCounter) {
  if (!sample) {
    failedCounter.add(1, { reason: 'missing_report_sample' });
    reportStatusSuccessRate.add(false);
    return;
  }
  const path = renderPath(pathTemplate, {
    assessment_id: sample.assessment_id,
    testee_id: sample.testee_id,
    report_timeout: String(REPORT_TIMEOUT),
  }, ctx);
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(collectionToken()), {
    endpoint,
    service: 'collection-server',
    model_type: sample.model_type || 'medical',
  });

  durationTrend.add(res.timings.duration, res.tags);
  const ok = res.status === 200;
  recordHTTPStatus(res, failedCounter, endpoint);
  reportStatusSuccessRate.add(ok, res.tags);
  if (ok) {
    const status = responseData(res).status || '';
    if (status === 'interpreted' || status === 'failed') {
      reportStatusTerminal.add(1, Object.assign({}, res.tags, { assessment_status: status }));
    } else {
      reportStatusPending.add(1, Object.assign({}, res.tags, { assessment_status: status || 'unknown' }));
      if (REPORT_SHORT_POLL) {
        const pollMs = Number(responseData(res).next_poll_after_ms) || 3000;
        sleep(Math.max(pollMs, 500) / 1000);
      }
    }
  }

  check(res, {
    'report status query status is 200': (r) => r.status === 200,
  });
}

