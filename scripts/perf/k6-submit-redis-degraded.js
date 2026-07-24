import http from 'k6/http';
import exec from 'k6/execution';
import { check, fail } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const mode = String(__ENV.DEGRADED_SUBMIT_MODE || 'low').trim().toLowerCase();
const supportedModes = ['low', 'global_overload', 'user_overload'];
const collectionURLs = String(__ENV.COLLECTION_BASE_URLS || '')
  .split(',')
  .map((value) => value.trim().replace(/\/+$/, ''))
  .filter(Boolean);
const submitPath = __ENV.SUBMIT_PATH || '/api/v1/answersheets';
const rate = Number(__ENV.RPS || defaultRate(mode));
const duration = __ENV.DURATION || '30s';
const preAllocatedVUs = Number(__ENV.VUS || Math.max(40, rate));
const maxVUs = Number(__ENV.MAX_VUS || Math.max(120, rate * 2));
const idempotencyPrefix =
  __ENV.IDEMPOTENCY_PREFIX || `redis-degraded-${mode}-${Date.now()}`;

const acceptedTotal = new Counter('degraded_submit_accepted_total');
const rateLimitedTotal = new Counter('degraded_submit_rate_limited_total');
const retryAfterMissingTotal = new Counter('degraded_submit_retry_after_missing_total');
const unexpectedTotal = new Counter('degraded_submit_unexpected_total');
const acceptedRate = new Rate('degraded_submit_accepted_rate');
const requestDuration = new Trend('degraded_submit_duration', true);

const thresholds = {
  degraded_submit_unexpected_total: ['count==0'],
  degraded_submit_retry_after_missing_total: ['count==0'],
  degraded_submit_duration: ['p(99)<2000'],
};
if (mode === 'low') {
  thresholds.degraded_submit_accepted_rate = ['rate>0.99'];
  thresholds.degraded_submit_rate_limited_total = ['count==0'];
} else {
  thresholds.degraded_submit_accepted_total = ['count>0'];
  thresholds.degraded_submit_rate_limited_total = ['count>0'];
}

export const options = {
  scenarios: {
    degradedSubmit: {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
    },
  },
  thresholds,
};

export function setup() {
  if (!supportedModes.includes(mode)) {
    fail(`DEGRADED_SUBMIT_MODE must be one of: ${supportedModes.join(', ')}`);
  }
  if (collectionURLs.length < 2 || new Set(collectionURLs).size !== collectionURLs.length) {
    fail('COLLECTION_BASE_URLS must contain at least two distinct collection instances');
  }
  if (!Number.isFinite(rate) || rate <= 0) {
    fail('RPS must be a positive number');
  }
  let cases;
  try {
    cases = JSON.parse(__ENV.SUBMIT_CASES_JSON || '[]');
  } catch (error) {
    fail(`SUBMIT_CASES_JSON is not valid JSON: ${error}`);
  }
  if (!Array.isArray(cases) || cases.length === 0) {
    fail('SUBMIT_CASES_JSON must be a non-empty array of {token,payload}');
  }
  for (const [index, item] of cases.entries()) {
    if (!item || !item.token || !item.payload || typeof item.payload !== 'object') {
      fail(`SUBMIT_CASES_JSON[${index}] must contain token and payload`);
    }
  }
  if (mode === 'low' && cases.length < 2) {
    fail('low mode requires at least two writers so 20 QPS stays below the 10 QPS per-instance user fallback');
  }
  if (mode === 'global_overload' && cases.length < 6) {
    fail('global_overload requires at least six writers so the global fallback is the intended bottleneck');
  }
  if (mode === 'user_overload' && cases.length !== 1) {
    fail('user_overload requires exactly one writer');
  }
  return { cases };
}

export default function (data) {
  const iteration = Number(exec.scenario.iterationInTest);
  const submitCase = data.cases[iteration % data.cases.length];
  const baseURL = collectionURLs[iteration % collectionURLs.length];
  const payload = JSON.parse(JSON.stringify(submitCase.payload));
  payload.idempotency_key = `${idempotencyPrefix}-${iteration}`;

  const response = http.post(`${baseURL}${submitPath}`, JSON.stringify(payload), {
    responseCallback:
      mode === 'low' ? http.expectedStatuses(202) : http.expectedStatuses(202, 429),
    headers: {
      Authorization: `Bearer ${submitCase.token}`,
      'Content-Type': 'application/json',
      'X-Request-ID': `${payload.idempotency_key}-request`,
    },
    tags: { endpoint: 'answersheet_submit', degraded_mode: mode },
  });
  requestDuration.add(response.timings.duration);

  const body = responseData(response);
  const accepted =
    response.status === 202 &&
    body.status === 'accepted' &&
    Boolean(body.answersheet_id);
  const rateLimited = response.status === 429;
  const retryAfter = response.headers['Retry-After'];
  const validRetryAfter =
    rateLimited &&
    retryAfter !== undefined &&
    Number.isFinite(Number(retryAfter)) &&
    Number(retryAfter) >= 1;

  acceptedRate.add(accepted);
  if (accepted) {
    acceptedTotal.add(1);
  } else if (rateLimited) {
    rateLimitedTotal.add(1);
    if (!validRetryAfter) {
      retryAfterMissingTotal.add(1);
    }
  } else {
    unexpectedTotal.add(1);
  }

  check(response, {
    'response is durable 202 or bounded 429': () => accepted || rateLimited,
    '429 includes Retry-After': () => !rateLimited || validRetryAfter,
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

function defaultRate(selectedMode) {
  switch (selectedMode) {
    case 'global_overload':
      return 120;
    case 'user_overload':
      return 30;
    default:
      return 20;
  }
}
