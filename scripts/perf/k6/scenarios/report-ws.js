import ws from 'k6/ws';
import { check } from 'k6';
import exec from 'k6/execution';
import { scenarioData, pickReportSampleForIteration, flattenReportSamples } from '../lib/data.js';
import { authHeaders, collectionTokenAt, correlatedHeaders } from '../lib/http.js';
import {
  COLLECTION_BASE_URL, REPORT_EVENTS_PATH, REPORT_WS_HOLD_SECONDS,
  LEGACY_REPORT_RPS, MEDICAL_REPORT_RPS, BEHAVIOR_REPORT_RPS, PERSONALITY_REPORT_RPS,
} from '../lib/config.js';
import {
  reportStatusFailed, reportSampleSkipped,
  reportWsConnectDuration, reportWsFirstMessageLatency, reportWsSubscribeToFirstMessageLatency, reportWsSessionDuration,
  medicalReportWsSubscribeToFirstMessageLatency, behaviorReportWsSubscribeToFirstMessageLatency,
  personalityReportWsSubscribeToFirstMessageLatency,
  reportWsConnectSuccessRate, reportWsMessageSuccessRate, reportWsTimeoutTotal,
  reportWsCapacityRejectedTotal, reportWsRateLimitedTotal, reportWsProtocolErrorTotal,
  reportWsTransportErrorTotal, reportWsConnectFailedTotal, reportWsMessageMissingTotal,
  reportWsServerRejectedTotal,
} from '../lib/metrics.js';
import { firstReportWsFailure, reportWsFailureCategory } from '../lib/ws-failure.js';

const subscribeLatencyByModel = {
  medical: medicalReportWsSubscribeToFirstMessageLatency,
  behavior: behaviorReportWsSubscribeToFirstMessageLatency,
  personality: personalityReportWsSubscribeToFirstMessageLatency,
};

const activeReportSampleLanes = [
  ['report_ws_query', LEGACY_REPORT_RPS],
  ['medical_report_ws_query', MEDICAL_REPORT_RPS],
  ['behavior_report_ws_query', BEHAVIOR_REPORT_RPS],
  ['personality_report_ws_query', PERSONALITY_REPORT_RPS],
].filter((item) => item[1] > 0).map((item) => item[0]);

const failureCounterByCategory = {
  capacity_rejected: reportWsCapacityRejectedTotal,
  rate_limited: reportWsRateLimitedTotal,
  protocol_error: reportWsProtocolErrorTotal,
  transport_error: reportWsTransportErrorTotal,
  connect_failed: reportWsConnectFailedTotal,
  message_missing: reportWsMessageMissingTotal,
  server_rejected: reportWsServerRejectedTotal,
};

function reportSampleForScenario(samples, lane) {
  return pickReportSampleForIteration(samples, exec.scenario.iterationInTest, lane, activeReportSampleLanes);
}

function recordReportWsFailure(reason, tags) {
  const category = reportWsFailureCategory(reason);
  reportStatusFailed.add(1, { ...tags, reason, failure_category: category });
  const counter = failureCounterByCategory[category];
  if (counter) {
    counter.add(1, { ...tags, reason });
  }
}

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
  let failureReason = '';
  let timedOut = false;
  const markFailure = (reason) => {
    failureReason = firstReportWsFailure(failureReason, reason);
  };
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
          markFailure(frame.code || 'ws_error');
          socket.close();
        }
      } catch (_err) {
        markFailure('ws_decode_error');
        socket.close();
      }
    });
    socket.on('error', () => {
      if (!firstStatusReceived) {
        markFailure('ws_transport_error');
      }
    });
    socket.setTimeout(() => {
      if (!terminal) {
        if (!firstStatusReceived) {
          timedOut = true;
          markFailure('ws_timeout');
        }
        socket.close();
      }
    }, Math.max(1000, Math.floor(REPORT_WS_HOLD_SECONDS * 1000)));
  });
  reportWsSessionDuration.add(Date.now() - started, tags);
  const connected = !!(res && res.status === 101 && opened);
  const messageReceived = connected && firstStatusReceived;
  reportWsConnectSuccessRate.add(connected, tags);
  reportWsMessageSuccessRate.add(messageReceived, tags);
  if (timedOut) {
    reportWsTimeoutTotal.add(1, tags);
  }
  if (!connected) {
    markFailure('ws_connect_status');
  } else if (!messageReceived) {
    markFailure('ws_status_message_missing');
  }
  if (failureReason) {
    recordReportWsFailure(failureReason, tags);
  }
  check(res, {
    'ws connect status 101': (r) => r && r.status === 101,
    'ws initial status message received': () => messageReceived,
  });
}

export function reportWsQuery(data) {
  const ctx = scenarioData(data);
  const sample = reportSampleForScenario(flattenReportSamples(ctx.reportSamples), 'report_ws_query');
  const kind = sample && (sample.model_type === 'personality' || sample.model_type === 'behavior') ? sample.model_type : 'medical';
  runReportWsQuery(ctx, sample, kind, 'report_ws_query');
}

export function medicalReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, reportSampleForScenario(ctx.reportSamples.medical, 'medical_report_ws_query'), 'medical', 'medical_report_ws_query');
}

export function behaviorReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, reportSampleForScenario(ctx.reportSamples.behavior, 'behavior_report_ws_query'), 'behavior', 'behavior_report_ws_query');
}

export function personalityReportWsQuery(data) {
  const ctx = scenarioData(data);
  runReportWsQuery(ctx, reportSampleForScenario(ctx.reportSamples.personality, 'personality_report_ws_query'), 'personality', 'personality_report_ws_query');
}
