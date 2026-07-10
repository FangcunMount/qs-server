import http from 'k6/http';
import { check } from 'k6';

// collection-server 已发布测评模型目录压测脚本。
const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8082';
const TOKEN = __ENV.TOKEN || '';
const RATE = Number(__ENV.RPS || '150');
const DURATION = __ENV.DURATION || '2m';
const VUS = Number(__ENV.VUS || '80');
const MAX_VUS = Number(__ENV.MAX_VUS || '200');

const DEFAULT_QUERY = {
  kind: 'scale',
  page: 1,
  page_size: 20,
};

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: VUS,
      maxVUs: MAX_VUS,
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<800', 'p(99)<1500'],
  },
};

export default function () {
  const headers = TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};
  const res = http.get(`${BASE_URL}/api/v1/assessment-models?${buildQuery(DEFAULT_QUERY)}`, { headers });

  check(res, {
    'status 200': (r) => r.status === 200,
  });
}

function buildQuery(params) {
  return Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== null && v !== '')
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&');
}
