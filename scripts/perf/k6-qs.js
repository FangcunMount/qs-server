import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18082';
const RAW_PATH = __ENV.PATH || '/api/v1/public/info';
const TOKEN = __ENV.TOKEN || '';
const TESTEE_ID = __ENV.TESTEE_ID || '';

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
  const path = buildPathWithTestee(RAW_PATH, TESTEE_ID);
  const res = http.get(`${BASE_URL}${path}`, { headers });
  check(res, {
    'status 200': (r) => r.status === 200,
  });
}

// 如果配置了 TESTEE_ID，且路径指向 assessments 列表而未带 testee_id，则自动补上
function buildPathWithTestee(path, testeeID) {
  if (!testeeID) return path;
  const isAssessmentsList = path.startsWith('/api/v1/assessments') && !path.includes('testee_id=');
  if (!isAssessmentsList) return path;
  const sep = path.includes('?') ? '&' : '?';
  return `${path}${sep}testee_id=${encodeURIComponent(testeeID)}`;
}
