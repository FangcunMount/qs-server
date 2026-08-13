import ws from 'k6/ws';
import { check } from 'k6';
import { scenarioData, pickReportSample, flattenReportSamples } from '../lib/data.js';
import { authHeaders, collectionTokenAt, correlatedHeaders } from '../lib/http.js';
import { COLLECTION_BASE_URL, REPORT_EVENTS_PATH, REPORT_WS_HOLD_SECONDS } from '../lib/config.js';
import {
  reportStatusFailed, reportSampleSkipped,
  reportWsConnectDuration, reportWsFirstMessageLatency, reportWsSubscribeToFirstMessageLatency, reportWsSessionDuration,
  medicalReportWsSubscribeToFirstMessageLatency, behaviorReportWsSubscribeToFirstMessageLatency,
  personalityReportWsSubscribeToFirstMessageLatency,
  reportWsConnectSuccessRate, reportWsMessageSuccessRate, reportWsTimeoutTotal,
} from '../lib/metrics.js';

const subscribeLatencyByModel = {
  medical: medicalReportWsSubscribeToFirstMessageLatency,
  behavior: behaviorReportWsSubscribeToFirstMessageLatency,
  personality: personalityReportWsSubscribeToFirstMessageLatency,
};

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
  const tags = { endpoint, service: 'collection-server', model_type: kind };
  if (!sample) {
    reportSampleSkipped.add(1, { ...tags, reason: 'missing_report_sample' });
    return;
  }
  const url = `${wsBaseURL(COLLECTION_BASE_URL)}${REPORT_EVENTS_PATH}`;
  const headers = correlatedHeaders(authHeaders(collectionTokenAt(sample.collection_token_index)), tags);
  const started = Date.now();
  let opened = false;
  let firstStatusReceived = false;
  let subscribedAt = 0;
  let protocolError = false;
  let timedOut = false;
  const res = ws.connect(url, { headers }, (socket) => {
    let terminal = false;
    socket.on('open', () => {
      opened = true;
      reportWsConnectDuration.add(Date.now() - started, tags);
      subscribedAt = Date.now();
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
          if (!firstStatusReceived) {
            firstStatusReceived = true;
            reportWsFirstMessageLatency.add(Date.now() - started, tags);
            if (subscribedAt > 0) {
              const subscribeLatency = Date.now() - subscribedAt;
              reportWsSubscribeToFirstMessageLatency.add(subscribeLatency, tags);
              subscribeLatencyByModel[kind].add(subscribeLatency, tags);
            }
          }
          const status = frame.data.status || '';
          if (status === 'interpreted' || status === 'failed') {
            terminal = true;
            socket.close();
          }
        }
        if (frame.op === 'error') {
          protocolError = true;
          reportStatusFailed.add(1, { ...tags, reason: frame.code || 'ws_error' });
          socket.close();
        }
      } catch (_err) {
        protocolError = true;
        reportStatusFailed.add(1, { ...tags, reason: 'ws_decode_error' });
        socket.close();
      }
    });
    socket.on('error', () => {
      protocolError = true;
      reportStatusFailed.add(1, { ...tags, reason: 'ws_transport_error' });
    });
    socket.setTimeout(() => {
      if (!terminal) {
        if (!firstStatusReceived) {
          timedOut = true;
        }
        socket.close();
      }
    }, Math.max(1000, Math.floor(REPORT_WS_HOLD_SECONDS * 1000)));
  });
  reportWsSessionDuration.add(Date.now() - started, tags);
  const connected = !!(res && res.status === 101 && opened);
  const messageReceived = connected && firstStatusReceived && !protocolError;
  reportWsConnectSuccessRate.add(connected, tags);
  reportWsMessageSuccessRate.add(messageReceived, tags);
  if (timedOut) {
    reportWsTimeoutTotal.add(1, tags);
  }
  if (!connected) {
    reportStatusFailed.add(1, { ...tags, reason: 'ws_connect_status' });
  } else if (!messageReceived) {
    reportStatusFailed.add(1, { ...tags, reason: 'ws_status_message_missing' });
  }
  check(res, {
    'ws connect status 101': (r) => r && r.status === 101,
    'ws initial status message received': () => messageReceived,
  });
}

export function reportWsQuery(data) {
  const ctx = scenarioData(data);
  const sample = pickReportSample(flattenReportSamples(ctx.reportSamples));
  const kind = sample && (sample.model_type === 'personality' || sample.model_type === 'behavior') ? sample.model_type : 'medical';
  runReportWsQuery(ctx, sample, kind, 'report_ws_query');
}

export function medicalReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, pickReportSample(ctx.reportSamples.medical), 'medical', 'medical_report_ws_query');
}

export function behaviorReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, pickReportSample(ctx.reportSamples.behavior), 'behavior', 'behavior_report_ws_query');
}

export function personalityReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, pickReportSample(ctx.reportSamples.personality), 'personality', 'personality_report_ws_query');
}
