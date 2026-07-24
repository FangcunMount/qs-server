import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const requestCount = Number(__ENV.COALESCING_REQUESTS || 100);
const collectionURLs = String(
  __ENV.COLLECTION_BASE_URLS || 'http://127.0.0.1:18083,http://127.0.0.1:18084'
)
  .split(',')
  .map((value) => value.trim().replace(/\/+$/, ''))
  .filter(Boolean);
const submitPath = __ENV.SUBMIT_PATH || '/api/v1/answersheets';
const token = __ENV.COLLECTION_TOKEN || __ENV.TOKEN || '';
const idempotencyKey = __ENV.IDEMPOTENCY_KEY || `coalesce-${Date.now()}`;
const verifyMetrics = String(__ENV.VERIFY_METRICS || 'true').toLowerCase() !== 'false';
const collectionMetricsURLs = String(
  __ENV.COLLECTION_METRICS_URLS || collectionURLs.map((url) => `${url}/metrics`).join(',')
)
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean);
const apiserverMetricsURL = String(__ENV.APISERVER_METRICS_URL || '').trim();

const acceptedTotal = new Counter('submit_coalescing_accepted_total');
const failedTotal = new Counter('submit_coalescing_failed_total');
const batchDuration = new Trend('submit_coalescing_batch_duration', true);
const submitHTTPDuration = new Trend('submit_coalescing_http_duration', true);

export const options = {
  scenarios: {
    duplicate_storm: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
      maxDuration: '30s',
    },
  },
  batch: requestCount,
  batchPerHost: requestCount,
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  thresholds: {
    checks: ['rate==1'],
    submit_coalescing_failed_total: ['count==0'],
    submit_coalescing_http_duration: ['p(99)<2000'],
  },
};

export function setup() {
  if (collectionURLs.length < 2) {
    fail('COLLECTION_BASE_URLS must contain at least two collection-server instances');
  }
  if (!token) {
    fail('COLLECTION_TOKEN or TOKEN is required');
  }
  if (!Number.isInteger(requestCount) || requestCount < 2 || requestCount > 1000) {
    fail('COALESCING_REQUESTS must be an integer between 2 and 1000');
  }
  if (!__ENV.SUBMIT_PAYLOAD_JSON) {
    fail('SUBMIT_PAYLOAD_JSON is required');
  }
  if (verifyMetrics && collectionMetricsURLs.length !== collectionURLs.length) {
    fail('COLLECTION_METRICS_URLS must contain one metrics endpoint per collection-server instance');
  }
  if (verifyMetrics && !apiserverMetricsURL) {
    fail('APISERVER_METRICS_URL is required unless VERIFY_METRICS=false');
  }

  let payload;
  try {
    payload = JSON.parse(__ENV.SUBMIT_PAYLOAD_JSON);
  } catch (error) {
    fail(`SUBMIT_PAYLOAD_JSON is not valid JSON: ${error}`);
  }
  payload.idempotency_key = idempotencyKey;
  return {
    payload,
    metricsBefore: verifyMetrics ? readMetricsSnapshot() : null,
  };
}

export default function (data) {
  const body = JSON.stringify(data.payload);
  const requests = [];
  for (let index = 0; index < requestCount; index += 1) {
    requests.push({
      method: 'POST',
      url: `${collectionURLs[index % collectionURLs.length]}${submitPath}`,
      body,
      params: {
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
          'X-Request-ID': `${idempotencyKey}-request-${index}`,
        },
        tags: {
          endpoint: 'answersheet_submit_coalescing',
          instance: String(index % collectionURLs.length),
        },
      },
    });
  }

  const started = Date.now();
  const responses = http.batch(requests);
  batchDuration.add(Date.now() - started);

  let answerSheetID = '';
  let allAccepted = true;
  let allSameID = true;
  for (const response of responses) {
    submitHTTPDuration.add(response.timings.duration);
    let parsed = {};
    try {
      const envelope = response.json();
      parsed = envelope && envelope.data !== undefined ? envelope.data || {} : envelope || {};
    } catch (_) {
      parsed = {};
    }
    const accepted =
      response.status === 202 &&
      parsed.status === 'accepted' &&
      Boolean(parsed.answersheet_id);
    if (!accepted) {
      allAccepted = false;
      failedTotal.add(1, { status: String(response.status) });
      continue;
    }
    acceptedTotal.add(1);
    if (!answerSheetID) {
      answerSheetID = String(parsed.answersheet_id);
    } else if (answerSheetID !== String(parsed.answersheet_id)) {
      allSameID = false;
    }
  }

  check(responses, {
    'all duplicate submissions return 202': () => allAccepted,
    'all duplicate submissions return one answersheet_id': () => allSameID && Boolean(answerSheetID),
  });
  console.log(
    `coalescing_result requests=${requestCount} instances=${collectionURLs.length} answersheet_id=${answerSheetID}`
  );
}

export function teardown(data) {
  if (!verifyMetrics) {
    console.warn('metrics verification skipped because VERIFY_METRICS=false');
    return;
  }

  const after = readMetricsSnapshot();
  const before = data.metricsBefore;
  const transactionCommitted = metricDelta(
    before.apiserver,
    after.apiserver,
    'qs_apiserver_answersheet_durable_operation_total',
    { operation: 'transaction', outcome: 'committed' }
  );
  const explicitReadbackHits = metricDelta(
    before.apiserver,
    after.apiserver,
    'qs_apiserver_answersheet_durable_operation_total',
    { operation: 'explicit_readback', outcome: 'hit' }
  );
  const preflightOK = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_stage_duration_seconds_count',
    { stage: 'preflight', outcome: 'ok' }
  );
  const profileLinkOK = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_stage_duration_seconds_count',
    { stage: 'profile_link', outcome: 'ok' }
  );
  const grpcSaveOK = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_stage_duration_seconds_count',
    { stage: 'grpc_save', outcome: 'ok' }
  );
  const collectionReadbackHits = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_total',
    { outcome: 'readback_hit' }
  );
  const gateRejects = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_submit_gate_reject_total',
    {}
  );
  const waitP95 = collectionHistogramQuantileDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_wait_seconds',
    0.95
  );
  const waitP99 = collectionHistogramQuantileDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_wait_seconds',
    0.99
  );
  const redisP95 = collectionHistogramQuantileDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_redis_seconds',
    0.95
  );
  const redisP99 = collectionHistogramQuantileDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_redis_seconds',
    0.99
  );

  console.log(
    [
      'coalescing_metrics',
      `transaction_committed=${transactionCommitted}`,
      `explicit_readback_hit=${explicitReadbackHits}`,
      `collection_readback_hit=${collectionReadbackHits}`,
      `preflight_ok=${preflightOK}`,
      `profile_link_ok=${profileLinkOK}`,
      `grpc_save_ok=${grpcSaveOK}`,
      `gate_reject=${gateRejects}`,
      `wait_p95_ms=${secondsToMilliseconds(waitP95)}`,
      `wait_p99_ms=${secondsToMilliseconds(waitP99)}`,
      `redis_p95_ms=${secondsToMilliseconds(redisP95)}`,
      `redis_p99_ms=${secondsToMilliseconds(redisP99)}`,
    ].join(' ')
  );

  const expectedHits = requestCount - 1;
  const metricsOK = check(
    {
      transactionCommitted,
      explicitReadbackHits,
      collectionReadbackHits,
      preflightOK,
      profileLinkOK,
      grpcSaveOK,
      gateRejects,
    },
    {
      'exactly one new AnswerSheet transaction commits': (value) =>
        value.transactionCommitted === 1,
      'all duplicate contenders hit explicit durable readback': (value) =>
        value.explicitReadbackHits >= expectedHits,
      'collection observes durable readback for all duplicates': (value) =>
        value.collectionReadbackHits >= expectedHits,
      'mutable questionnaire preflight runs once': (value) => value.preflightOK === 1,
      'ProfileLink resolution runs once': (value) => value.profileLinkOK === 1,
      'durable save gRPC runs once': (value) => value.grpcSaveOK === 1,
      'duplicate contention does not consume submit gate rejects': (value) =>
        value.gateRejects === 0,
    }
  );
  if (!metricsOK) {
    fail('SubmitCoalescer Prometheus delta contract failed');
  }
}

function readMetricsSnapshot() {
  return {
    collections: collectionMetricsURLs.map((url) => readMetrics(url)),
    apiserver: readMetrics(apiserverMetricsURL),
  };
}

function readMetrics(url) {
  const response = http.get(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    tags: { endpoint: 'submit_coalescing_metrics' },
  });
  if (response.status !== 200) {
    fail(`metrics endpoint ${url} returned HTTP ${response.status}`);
  }
  return response.body;
}

function metricDelta(before, after, name, labels) {
  return metricSum(after, name, labels) - metricSum(before, name, labels);
}

function collectionMetricDelta(before, after, name, labels) {
  let total = 0;
  for (let index = 0; index < after.length; index += 1) {
    total += metricDelta(before[index], after[index], name, labels);
  }
  return total;
}

function metricSum(text, name, requiredLabels) {
  let sum = 0;
  for (const line of String(text || '').split('\n')) {
    if (!line.startsWith(name)) {
      continue;
    }
    const parsed = parseMetricLine(line, name);
    if (parsed && labelsMatch(parsed.labels, requiredLabels)) {
      sum += parsed.value;
    }
  }
  return sum;
}

function parseMetricLine(line, name) {
  const match = line.match(new RegExp(`^${name}(?:\\{([^}]*)\\})?\\s+([^\\s]+)`));
  if (!match) {
    return null;
  }
  const labels = {};
  const labelsText = match[1] || '';
  const labelPattern = /([A-Za-z_][A-Za-z0-9_]*)="((?:\\.|[^"])*)"/g;
  let labelMatch;
  while ((labelMatch = labelPattern.exec(labelsText)) !== null) {
    labels[labelMatch[1]] = labelMatch[2];
  }
  return { labels, value: Number(match[2]) };
}

function labelsMatch(actual, required) {
  for (const key of Object.keys(required)) {
    if (actual[key] !== required[key]) {
      return false;
    }
  }
  return true;
}

function collectionHistogramQuantileDelta(before, after, name, quantile) {
  const buckets = {};
  for (let index = 0; index < after.length; index += 1) {
    const beforeBuckets = histogramBuckets(before[index], name);
    const afterBuckets = histogramBuckets(after[index], name);
    for (const le of Object.keys(afterBuckets)) {
      buckets[le] = (buckets[le] || 0) + afterBuckets[le] - (beforeBuckets[le] || 0);
    }
  }
  const ordered = Object.keys(buckets)
    .map((le) => ({ upper: le === '+Inf' ? Infinity : Number(le), count: buckets[le] }))
    .sort((left, right) => left.upper - right.upper);
  if (ordered.length === 0) {
    return NaN;
  }
  const total = ordered[ordered.length - 1].count;
  if (total <= 0) {
    return NaN;
  }
  const rank = total * quantile;
  for (const bucket of ordered) {
    if (bucket.count >= rank) {
      return bucket.upper;
    }
  }
  return ordered[ordered.length - 1].upper;
}

function histogramBuckets(text, name) {
  const buckets = {};
  const bucketName = `${name}_bucket`;
  for (const line of String(text || '').split('\n')) {
    if (!line.startsWith(bucketName)) {
      continue;
    }
    const parsed = parseMetricLine(line, bucketName);
    if (parsed && parsed.labels.le !== undefined) {
      buckets[parsed.labels.le] = (buckets[parsed.labels.le] || 0) + parsed.value;
    }
  }
  return buckets;
}

function secondsToMilliseconds(value) {
  return Number.isFinite(value) ? Math.round(value * 1000 * 1000) / 1000 : 'n/a';
}
