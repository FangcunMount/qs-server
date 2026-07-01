import ws from 'k6/ws';
import { check } from 'k6';
import { scenarioData, pickReportSample, flattenReportSamples } from '../lib/data.js';
import { authHeaders, collectionToken } from '../lib/http.js';
import { COLLECTION_BASE_URL, REPORT_EVENTS_PATH, REPORT_WS_HOLD_SECONDS } from '../lib/config.js';
import { reportStatusDuration, reportStatusFailed } from '../lib/metrics.js';

function wsBaseURL(httpBase) {
  if (httpBase.startsWith('https://')) {
    return `wss://${httpBase.slice('https://'.length)}`;
  }
  if (httpBase.startsWith('http://')) {
    return `ws://${httpBase.slice('http://'.length)}`;
  }
  return httpBase;
}

function runReportWsQuery(ctx, sample, kind, endpoint) {
  if (!sample) {
    reportStatusFailed.add(1, { reason: 'missing_report_sample' });
    return;
  }
  const url = `${wsBaseURL(COLLECTION_BASE_URL)}${REPORT_EVENTS_PATH}`;
  const headers = authHeaders(collectionToken());
  const started = Date.now();
  const res = ws.connect(url, { headers }, (socket) => {
    let terminal = false;
    socket.on('open', () => {
      socket.send(JSON.stringify({
        op: 'subscribe',
        assessment_id: String(sample.assessment_id),
        kind,
        testee_id: String(sample.testee_id),
      }));
    });
    socket.on('message', (data) => {
      try {
        const frame = JSON.parse(data);
        if (frame.op === 'status' && frame.data) {
          const status = frame.data.status || '';
          if (status === 'interpreted' || status === 'failed') {
            terminal = true;
            socket.close();
          }
        }
        if (frame.op === 'error') {
          reportStatusFailed.add(1, { reason: frame.code || 'ws_error' });
          socket.close();
        }
      } catch (_err) {
        reportStatusFailed.add(1, { reason: 'ws_decode_error' });
        socket.close();
      }
    });
    socket.setTimeout(() => {
      if (!terminal) {
        socket.close();
      }
    }, Math.max(1000, Math.floor(REPORT_WS_HOLD_SECONDS * 1000)));
  });
  reportStatusDuration.add(Date.now() - started, { endpoint, service: 'collection-server', model_type: kind });
  check(res, { 'ws connect status 101': (r) => r && r.status === 101 });
}

export function reportWsQuery(data) {
  const ctx = scenarioData(data);
  const sample = pickReportSample(flattenReportSamples(ctx.reportSamples));
  const kind = sample && sample.model_type === 'personality' ? 'personality' : 'medical';
  runReportWsQuery(ctx, sample, kind, 'report_ws_query');
}

export function medicalReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, pickReportSample(ctx.reportSamples.medical), 'medical', 'medical_report_ws_query');
}

export function personalityReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, pickReportSample(ctx.reportSamples.personality), 'personality', 'personality_report_ws_query');
}
