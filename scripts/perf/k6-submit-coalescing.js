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

const acceptedTotal = new Counter('submit_coalescing_accepted_total');
const failedTotal = new Counter('submit_coalescing_failed_total');
const batchDuration = new Trend('submit_coalescing_batch_duration', true);

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
  thresholds: {
    checks: ['rate==1'],
    submit_coalescing_failed_total: ['count==0'],
    http_req_duration: ['p(99)<2000'],
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

  let payload;
  try {
    payload = JSON.parse(__ENV.SUBMIT_PAYLOAD_JSON);
  } catch (error) {
    fail(`SUBMIT_PAYLOAD_JSON is not valid JSON: ${error}`);
  }
  payload.idempotency_key = idempotencyKey;
  return { payload };
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
