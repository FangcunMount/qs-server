import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18082';
const PATH = __ENV.PATH || '/api/v1/public/info';
const TOKEN = __ENV.TOKEN || '';

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RPS || '200'), // 每秒期望请求数
      timeUnit: '1s',
      duration: __ENV.DURATION || '2m',
      preAllocatedVUs: Number(__ENV.VUS || '50'),
      maxVUs: Number(__ENV.MAX_VUS || '200'),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'], // 错误率 <1%
    http_req_duration: ['p(95)<800', 'p(99)<1500'], // 延迟阈值
  },
};

export default function () {
  const headers = TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};
  const res = http.get(`${BASE_URL}${PATH}`, { headers });
  check(res, {
    'status 200': (r) => r.status === 200,
  });
}
