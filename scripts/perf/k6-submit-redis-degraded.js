import http from 'k6/http';
import exec from 'k6/execution';
import { check, fail } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { discoverSubmitCases } from './k6/lib/submit-fixture.js';

const mode = String(__ENV.DEGRADED_SUBMIT_MODE || 'low').trim().toLowerCase();
const supportedModes = ['low', 'global_overload', 'user_overload'];
const collectionURLs = String(__ENV.COLLECTION_BASE_URLS || '')
  .split(',')
  .map((value) => value.trim().replace(/\/+$/, ''))
  .filter(Boolean);
const submitPath = __ENV.SUBMIT_PATH || '/api/v1/answersheets';
const rate = Number(__ENV.RPS || defaultRate(mode));
const warmupDuration = __ENV.WARMUP_DURATION || '15s';
const steadyDuration = __ENV.STEADY_DURATION || __ENV.DURATION || '60s';
const preAllocatedVUs = Number(__ENV.VUS || Math.max(40, rate));
const maxVUs = Number(__ENV.MAX_VUS || Math.max(120, rate * 2));
const expectedReplicas = Number(__ENV.EXPECTED_COLLECTION_REPLICAS || 2);
const fallbackGlobalQPS = Number(__ENV.FALLBACK_GLOBAL_QPS || 30);
const fallbackUserQPS = Number(__ENV.FALLBACK_USER_QPS || 10);
const steadyRateTolerance = Number(__ENV.STEADY_RATE_TOLERANCE || 0.05);
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
  thresholds['degraded_submit_accepted_rate{phase:steady}'] = ['rate>0.99'];
  thresholds['degraded_submit_rate_limited_total{phase:steady}'] = ['count==0'];
} else {
  const fallbackQPS = mode === 'global_overload' ? fallbackGlobalQPS : fallbackUserQPS;
  const steadyAcceptedCeiling = expectedReplicas * fallbackQPS * (1 + steadyRateTolerance);
  thresholds['degraded_submit_accepted_total{phase:steady}'] = [
    'count>0',
    `rate<=${steadyAcceptedCeiling}`,
  ];
  thresholds['degraded_submit_rate_limited_total{phase:steady}'] = ['count>0'];
}

export const options = {
  scenarios: {
    degradedSubmitWarmup: {
      executor: 'constant-arrival-rate',
      exec: 'warmup',
      rate,
      timeUnit: '1s',
      duration: warmupDuration,
      preAllocatedVUs,
      maxVUs,
    },
    degradedSubmitSteady: {
      executor: 'constant-arrival-rate',
      exec: 'steady',
      startTime: warmupDuration,
      rate,
      timeUnit: '1s',
      duration: steadyDuration,
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
  if (!Number.isInteger(expectedReplicas) || expectedReplicas < 2) {
    fail('EXPECTED_COLLECTION_REPLICAS must be an integer of at least 2');
  }
  if (!Number.isFinite(fallbackGlobalQPS) || fallbackGlobalQPS <= 0) {
    fail('FALLBACK_GLOBAL_QPS must be a positive number');
  }
  if (!Number.isFinite(fallbackUserQPS) || fallbackUserQPS <= 0) {
    fail('FALLBACK_USER_QPS must be a positive number');
  }
  if (!Number.isFinite(steadyRateTolerance) || steadyRateTolerance < 0 || steadyRateTolerance > 0.5) {
    fail('STEADY_RATE_TOLERANCE must be between 0 and 0.5');
  }
  let cases = null;
  if (__ENV.SUBMIT_CASES_JSON) {
    try {
      cases = JSON.parse(__ENV.SUBMIT_CASES_JSON);
    } catch (error) {
      fail(`SUBMIT_CASES_JSON is not valid JSON: ${error}`);
    }
  } else {
    cases = discoverSubmitCases(collectionURLs, requiredCaseCount(mode));
  }
  if (!Array.isArray(cases) || cases.length === 0) {
    fail('Submit cases must be a non-empty array of {token,payload}');
  }
  for (const [index, item] of cases.entries()) {
    if (!item || !item.token || !item.payload || typeof item.payload !== 'object') {
      fail(`Submit case[${index}] must contain token and payload`);
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

export function warmup(data) {
  submit(data, 'warmup');
}

export function steady(data) {
  submit(data, 'steady');
}

function submit(data, phase) {
  const iteration = Number(exec.scenario.iterationInTest);
  const submitCase = data.cases[iteration % data.cases.length];
  const baseURL = collectionURLs[iteration % collectionURLs.length];
  const payload = JSON.parse(JSON.stringify(submitCase.payload));
  payload.idempotency_key = `${idempotencyPrefix}-${phase}-${iteration}`;
  const metricTags = { phase };

  const response = http.post(`${baseURL}${submitPath}`, JSON.stringify(payload), {
    responseCallback:
      mode === 'low' ? http.expectedStatuses(202) : http.expectedStatuses(202, 429),
    headers: {
      Authorization: `Bearer ${submitCase.token}`,
      'Content-Type': 'application/json',
      'X-Request-ID': `${payload.idempotency_key}-request`,
    },
    tags: { endpoint: 'answersheet_submit', degraded_mode: mode, phase },
  });
  requestDuration.add(response.timings.duration, metricTags);

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

  acceptedRate.add(accepted, metricTags);
  acceptedTotal.add(accepted ? 1 : 0, metricTags);
  rateLimitedTotal.add(rateLimited ? 1 : 0, metricTags);
  if (rateLimited) {
    if (!validRetryAfter) {
      retryAfterMissingTotal.add(1, metricTags);
    }
  } else if (!accepted) {
    unexpectedTotal.add(1, metricTags);
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

function requiredCaseCount(selectedMode) {
  switch (selectedMode) {
    case 'global_overload':
      return 6;
    case 'user_overload':
      return 1;
    default:
      return 2;
  }
}
