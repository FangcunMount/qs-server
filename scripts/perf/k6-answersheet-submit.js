import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const baseURL = String(
  __ENV.COLLECTION_BASE_URL || __ENV.BASE_URL || 'http://127.0.0.1:18083'
).replace(/\/+$/, '');
const submitPath = __ENV.SUBMIT_PATH || '/api/v1/answersheets';
const token = __ENV.COLLECTION_TOKEN || __ENV.TOKEN || '';
const idempotencyPrefix = __ENV.IDEMPOTENCY_PREFIX || `k6-submit-${Date.now()}`;
const rate = Number(__ENV.RPS || 100);
const duration = __ENV.DURATION || '60s';
const preAllocatedVUs = Number(__ENV.VUS || 20);
const maxVUs = Number(__ENV.MAX_VUS || 100);

const acceptedTotal = new Counter('answersheet_submit_accepted_total');
const successRate = new Rate('answersheet_submit_success_rate');
const submitDuration = new Trend('answersheet_submit_duration', true);

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
    },
  },
  thresholds: {
    checks: ['rate>0.99'],
    answersheet_submit_success_rate: ['rate>0.99'],
    answersheet_submit_duration: ['p(95)<800', 'p(99)<1500'],
  },
};

export function setup() {
  if (!token) {
    fail('COLLECTION_TOKEN or TOKEN is required');
  }
  if (!__ENV.SUBMIT_PAYLOAD_JSON) {
    fail('SUBMIT_PAYLOAD_JSON is required');
  }
  try {
    return { payload: JSON.parse(__ENV.SUBMIT_PAYLOAD_JSON) };
  } catch (error) {
    fail(`SUBMIT_PAYLOAD_JSON is not valid JSON: ${error}`);
  }
}

export default function (data) {
  const payload = JSON.parse(JSON.stringify(data.payload));
  payload.idempotency_key =
    payload.idempotency_key || `${idempotencyPrefix}-${__VU}-${__ITER}-${Date.now()}`;
  const requestID = `${payload.idempotency_key}-request`;
  const response = http.post(`${baseURL}${submitPath}`, JSON.stringify(payload), {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      'X-Request-ID': requestID,
    },
    tags: { endpoint: 'answersheet_submit' },
  });

  submitDuration.add(response.timings.duration);
  const dataBody = responseData(response);
  const accepted =
    response.status === 202 &&
    dataBody.status === 'accepted' &&
    Boolean(dataBody.answersheet_id);
  if (accepted) {
    acceptedTotal.add(1);
  }
  successRate.add(accepted);
  check(response, {
    'answersheet submit status is 202': (result) => result.status === 202,
    'answersheet submit is durably accepted': () => accepted,
  });
}

function responseData(response) {
  try {
    const envelope = response.json();
    return envelope && envelope.data !== undefined ? envelope.data || {} : envelope || {};
  } catch (_) {
    return {};
  }
}
