import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const scenario = String(__ENV.COALESCING_SCENARIO || 'healthy').trim().toLowerCase();
const supportedScenarios = [
  'healthy',
  'conflict',
  'redis_lock_failure',
  'redis_signal_failure',
  'redis_unavailable',
];
const requestCount = Number(
  __ENV.COALESCING_REQUESTS || (scenario.startsWith('redis_') ? 10 : 100)
);
const collectionURLs = splitURLs(
  __ENV.COLLECTION_BASE_URLS || 'http://127.0.0.1:18083,http://127.0.0.1:18084'
);
const submitPath = __ENV.SUBMIT_PATH || '/api/v1/answersheets';
const token = __ENV.COLLECTION_TOKEN || __ENV.TOKEN || '';
const idempotencyKey = __ENV.IDEMPOTENCY_KEY || `coalesce-${Date.now()}`;
const verifyMetrics = String(__ENV.VERIFY_METRICS || 'true').toLowerCase() !== 'false';
const isolatedMetrics = String(__ENV.PERF_ISOLATED_ENV || 'false').toLowerCase() === 'true';
const collectionMetricsURLs = splitURLs(
  __ENV.COLLECTION_METRICS_URLS || collectionURLs.map((url) => `${url}/metrics`).join(',')
);
const apiserverMetricsURL = String(__ENV.APISERVER_METRICS_URL || '').trim();

const acceptedTotal = new Counter('submit_coalescing_accepted_total');
const conflictTotal = new Counter('submit_coalescing_conflict_total');
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
  if (!supportedScenarios.includes(scenario)) {
    fail(`COALESCING_SCENARIO must be one of: ${supportedScenarios.join(', ')}`);
  }
  if (collectionURLs.length < 2 || new Set(collectionURLs).size !== collectionURLs.length) {
    fail('COLLECTION_BASE_URLS must contain at least two distinct collection-server instances');
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
  if (scenario === 'conflict' && !__ENV.CONFLICT_PAYLOAD_JSON) {
    fail('CONFLICT_PAYLOAD_JSON is required for COALESCING_SCENARIO=conflict');
  }
  if (verifyMetrics && !isolatedMetrics) {
    fail('PERF_ISOLATED_ENV=true is required because acceptance uses exact global metric deltas');
  }
  if (
    verifyMetrics &&
    (collectionMetricsURLs.length !== collectionURLs.length ||
      new Set(collectionMetricsURLs).size !== collectionMetricsURLs.length)
  ) {
    fail('COLLECTION_METRICS_URLS must contain one distinct endpoint per collection instance');
  }
  if (verifyMetrics && !apiserverMetricsURL) {
    fail('APISERVER_METRICS_URL is required unless VERIFY_METRICS=false');
  }

  const primaryPayload = parsePayload('SUBMIT_PAYLOAD_JSON', __ENV.SUBMIT_PAYLOAD_JSON);
  primaryPayload.idempotency_key = idempotencyKey;

  let conflictPayload = null;
  if (scenario === 'conflict') {
    conflictPayload = parsePayload('CONFLICT_PAYLOAD_JSON', __ENV.CONFLICT_PAYLOAD_JSON);
    conflictPayload.idempotency_key = idempotencyKey;
    if (JSON.stringify(conflictPayload) === JSON.stringify(primaryPayload)) {
      fail('CONFLICT_PAYLOAD_JSON must differ from SUBMIT_PAYLOAD_JSON');
    }
  }

  return {
    primaryPayload,
    conflictPayload,
    metricsBefore: verifyMetrics ? readMetricsSnapshot() : null,
  };
}

export default function (data) {
  const requests = [];
  for (let index = 0; index < requestCount; index += 1) {
    const useConflictPayload = scenario === 'conflict' && index % 2 === 1;
    const payload = useConflictPayload ? data.conflictPayload : data.primaryPayload;
    requests.push({
      method: 'POST',
      url: `${collectionURLs[index % collectionURLs.length]}${submitPath}`,
      body: JSON.stringify(payload),
      params: {
        responseCallback:
          scenario === 'conflict' ? http.expectedStatuses(202, 409) : http.expectedStatuses(202),
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
          'X-Request-ID': `${idempotencyKey}-request-${index}`,
        },
        tags: {
          endpoint: 'answersheet_submit_coalescing',
          target_instance: String(index % collectionURLs.length),
          coalescing_scenario: scenario,
          payload_variant: useConflictPayload ? 'conflict' : 'primary',
        },
      },
    });
  }

  const started = Date.now();
  const responses = http.batch(requests);
  batchDuration.add(Date.now() - started, { coalescing_scenario: scenario });

  let answerSheetID = '';
  let accepted = 0;
  let conflicts = 0;
  let invalid = 0;
  let allAcceptedIDsMatch = true;

  for (const response of responses) {
    submitHTTPDuration.add(response.timings.duration, { coalescing_scenario: scenario });
    const parsed = responseData(response);
    if (
      response.status === 202 &&
      parsed.status === 'accepted' &&
      Boolean(parsed.answersheet_id)
    ) {
      accepted += 1;
      acceptedTotal.add(1, { coalescing_scenario: scenario });
      if (!answerSheetID) {
        answerSheetID = String(parsed.answersheet_id);
      } else if (answerSheetID !== String(parsed.answersheet_id)) {
        allAcceptedIDsMatch = false;
      }
      continue;
    }
    if (scenario === 'conflict' && response.status === 409) {
      conflicts += 1;
      conflictTotal.add(1, { coalescing_scenario: scenario });
      continue;
    }
    invalid += 1;
    failedTotal.add(1, {
      coalescing_scenario: scenario,
      status: String(response.status),
    });
  }

  const responseContractOK = check(
    { accepted, conflicts, invalid, allAcceptedIDsMatch, answerSheetID },
    scenario === 'conflict'
      ? {
          'conflict storm returns only 202 or 409': (value) => value.invalid === 0,
          'conflict storm has one accepted fingerprint': (value) => value.accepted > 0,
          'conflict storm rejects the other fingerprint': (value) => value.conflicts > 0,
          'all accepted conflict responses use one answersheet_id': (value) =>
            value.allAcceptedIDsMatch && Boolean(value.answerSheetID),
        }
      : {
          'all duplicate submissions return 202': (value) =>
            value.accepted === requestCount && value.invalid === 0,
          'all duplicate submissions return one answersheet_id': (value) =>
            value.allAcceptedIDsMatch && Boolean(value.answerSheetID),
        }
  );
  if (!responseContractOK) {
    fail(`SubmitCoalescer HTTP contract failed for scenario=${scenario}`);
  }

  console.log(
    [
      'coalescing_result',
      `scenario=${scenario}`,
      `requests=${requestCount}`,
      `instances=${collectionURLs.length}`,
      `accepted=${accepted}`,
      `conflict=${conflicts}`,
      `invalid=${invalid}`,
      `answersheet_id=${answerSheetID}`,
    ].join(' ')
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
  const explicitReadbackConflicts = metricDelta(
    before.apiserver,
    after.apiserver,
    'qs_apiserver_answersheet_durable_operation_total',
    { operation: 'explicit_readback', outcome: 'conflict' }
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
  const collectionReadbackHits = coalescerOutcomeDelta(before, after, 'readback_hit');
  const owner = coalescerOutcomeDelta(before, after, 'owner');
  const contenderSignaled = coalescerOutcomeDelta(before, after, 'contender_signaled');
  const contenderTimeout = coalescerOutcomeDelta(before, after, 'contender_timeout');
  const degradedOpen = coalescerOutcomeDelta(before, after, 'degraded_open');
  const signalError = coalescerOutcomeDelta(before, after, 'signal_error');
  const readbackError = coalescerOutcomeDelta(before, after, 'readback_error');
  const gateRejects = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_submit_gate_reject_total',
    {}
  );
  const leaseAcquired = redisOperationCountDelta(before, after, 'lease_acquire', 'acquired');
  const leaseContention = redisOperationCountDelta(before, after, 'lease_acquire', 'contention');
  const leaseError = redisOperationCountDelta(before, after, 'lease_acquire', 'error');
  const signalWriteError = redisOperationCountDelta(before, after, 'signal_write', 'error');
  const waitSamples = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_wait_seconds_count',
    {}
  );
  const redisSamples = collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_redis_seconds_count',
    {}
  );
  const collectionInstanceActivity = collectionMetricDeltas(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_total',
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

  const observed = {
    transactionCommitted,
    explicitReadbackHits,
    explicitReadbackConflicts,
    collectionReadbackHits,
    preflightOK,
    profileLinkOK,
    grpcSaveOK,
    owner,
    contenderSignaled,
    contenderTimeout,
    degradedOpen,
    signalError,
    readbackError,
    gateRejects,
    leaseAcquired,
    leaseContention,
    leaseError,
    signalWriteError,
    waitSamples,
    redisSamples,
    collectionInstanceActivity,
  };

  console.log(
    [
      'coalescing_metrics',
      `scenario=${scenario}`,
      `transaction_committed=${transactionCommitted}`,
      `explicit_readback_hit=${explicitReadbackHits}`,
      `explicit_readback_conflict=${explicitReadbackConflicts}`,
      `collection_readback_hit=${collectionReadbackHits}`,
      `preflight_ok=${preflightOK}`,
      `profile_link_ok=${profileLinkOK}`,
      `grpc_save_ok=${grpcSaveOK}`,
      `owner=${owner}`,
      `contender_signaled=${contenderSignaled}`,
      `contender_timeout=${contenderTimeout}`,
      `degraded_open=${degradedOpen}`,
      `signal_error=${signalError}`,
      `lease_acquired=${leaseAcquired}`,
      `lease_contention=${leaseContention}`,
      `lease_error=${leaseError}`,
      `signal_write_error=${signalWriteError}`,
      `gate_reject=${gateRejects}`,
      `instance_activity=${collectionInstanceActivity.join(',')}`,
      `wait_p95_ms=${secondsToMilliseconds(waitP95)}`,
      `wait_p99_ms=${secondsToMilliseconds(waitP99)}`,
      `redis_p95_ms=${secondsToMilliseconds(redisP95)}`,
      `redis_p99_ms=${secondsToMilliseconds(redisP99)}`,
    ].join(' ')
  );

  const commonChecks = {
    'exactly one new AnswerSheet transaction commits': (value) =>
      value.transactionCommitted === 1,
    'both collection instances process the storm': (value) =>
      value.collectionInstanceActivity.length === collectionURLs.length &&
      value.collectionInstanceActivity.every((count) => count > 0),
    'coalescing scenarios do not consume submit gate rejects': (value) =>
      value.gateRejects === 0,
  };
  const scenarioChecks = checksForScenario();
  const metricsOK = check(observed, { ...commonChecks, ...scenarioChecks });
  if (!metricsOK) {
    fail(`SubmitCoalescer Prometheus delta contract failed for scenario=${scenario}`);
  }
}

function checksForScenario() {
  const expectedReadbacks = requestCount - 1;
  switch (scenario) {
    case 'healthy':
      return {
        'all duplicate contenders hit explicit durable readback': (value) =>
          value.explicitReadbackHits >= expectedReadbacks,
        'collection observes durable readback for all duplicates': (value) =>
          value.collectionReadbackHits >= expectedReadbacks,
        'mutable questionnaire preflight runs once': (value) => value.preflightOK === 1,
        'ProfileLink resolution runs once': (value) => value.profileLinkOK === 1,
        'durable save gRPC runs once': (value) => value.grpcSaveOK === 1,
        'healthy storm has at least one lease owner': (value) =>
          value.owner >= 1 && value.leaseAcquired >= 1,
        'healthy storm produces lease contention': (value) =>
          value.leaseContention > 0 &&
          value.contenderSignaled + value.contenderTimeout > 0 &&
          value.waitSamples > 0,
        'healthy storm records Redis latency': (value) => value.redisSamples > 0,
        'healthy storm does not degrade open': (value) => value.degradedOpen === 0,
        'healthy storm has no signal failure': (value) =>
          value.signalError === 0 && value.signalWriteError === 0,
        'healthy storm has no durable readback error': (value) => value.readbackError === 0,
      };
    case 'conflict':
      return {
        'conflicting fingerprint reaches durable conflict readback': (value) =>
          value.explicitReadbackConflicts > 0,
        'all conflict contenders resolve through durable readback': (value) =>
          value.explicitReadbackHits + value.explicitReadbackConflicts >= expectedReadbacks,
        'conflict storm creates only one mutable submission': (value) =>
          value.preflightOK === 1 && value.profileLinkOK === 1 && value.grpcSaveOK === 1,
        'conflict storm uses the Redis lease': (value) =>
          value.leaseAcquired >= 1 && value.leaseContention > 0,
        'conflict storm does not degrade open': (value) => value.degradedOpen === 0,
      };
    case 'redis_lock_failure':
      return {
        'lock failure is observed as degraded-open efficiency loss': (value) =>
          value.degradedOpen > 0 && value.leaseError > 0,
      };
    case 'redis_signal_failure':
      return {
        'signal failure is observed without invalidating durable success': (value) =>
          value.signalError > 0 && value.signalWriteError > 0,
      };
    case 'redis_unavailable':
      return {
        'complete Redis outage degrades open to Mongo truth': (value) =>
          value.degradedOpen > 0 && value.leaseError > 0,
      };
    default:
      return {};
  }
}

function parsePayload(name, raw) {
  try {
    return JSON.parse(raw);
  } catch (error) {
    fail(`${name} is not valid JSON: ${error}`);
  }
}

function responseData(response) {
  try {
    const envelope = response.json();
    return envelope && envelope.data !== undefined ? envelope.data || {} : envelope || {};
  } catch (_) {
    return {};
  }
}

function splitURLs(raw) {
  return String(raw || '')
    .split(',')
    .map((value) => value.trim().replace(/\/+$/, ''))
    .filter(Boolean);
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
    tags: {
      endpoint: 'submit_coalescing_metrics',
      coalescing_scenario: scenario,
    },
  });
  if (response.status !== 200) {
    fail(`metrics endpoint ${url} returned HTTP ${response.status}`);
  }
  return response.body;
}

function coalescerOutcomeDelta(before, after, outcome) {
  return collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_total',
    { outcome }
  );
}

function redisOperationCountDelta(before, after, operation, outcome) {
  return collectionMetricDelta(
    before.collections,
    after.collections,
    'qs_collection_answersheet_submit_coalescer_redis_seconds_count',
    { operation, outcome }
  );
}

function metricDelta(before, after, name, labels) {
  return metricSum(after, name, labels) - metricSum(before, name, labels);
}

function collectionMetricDelta(before, after, name, labels) {
  return collectionMetricDeltas(before, after, name, labels).reduce(
    (total, value) => total + value,
    0
  );
}

function collectionMetricDeltas(before, after, name, labels) {
  return after.map((snapshot, index) => metricDelta(before[index], snapshot, name, labels));
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
