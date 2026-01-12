import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18081'; // collection-server 默认端口按需改
const TOKEN = __ENV.TOKEN || ''; // 需要保护接口时传 Bearer Token

// 触发 POST 提交答卷：ENABLE_SUBMIT=true 并提供 ANSWER_BODY（JSON 字符串）
const ENABLE_SUBMIT = (__ENV.ENABLE_SUBMIT || 'false').toLowerCase() === 'true';
const ANSWER_BODY = __ENV.ANSWER_BODY || '';

const endpoints = [
  { name: 'public_info', method: 'GET', path: __ENV.PUBLIC_INFO_PATH || '/api/v1/public/info', auth: false },
  { name: 'scales_categories', method: 'GET', path: __ENV.SCALES_CATEGORIES_PATH || '/api/v1/scales/categories', auth: false },
  { name: 'scales_list', method: 'GET', path: __ENV.SCALES_PATH || '/api/v1/scales', auth: false },
  { name: 'questionnaires_list', method: 'GET', path: __ENV.QUESTIONNAIRES_PATH || '/api/v1/questionnaires', auth: true },
  { name: 'assessments_list', method: 'GET', path: __ENV.ASSESSMENTS_PATH || '/api/v1/assessments', auth: true },
  {
    name: 'answersheet_submit',
    method: 'POST',
    path: __ENV.ANSWERSHEETS_PATH || '/api/v1/answersheets',
    auth: true,
    body: ANSWER_BODY,
    enabled: ENABLE_SUBMIT,
  },
];

const RATE = Number(__ENV.RPS || '120'); // 每秒期望请求数（整体）
const DURATION = __ENV.DURATION || '2m';
const VUS = Number(__ENV.VUS || '60');
const MAX_VUS = Number(__ENV.MAX_VUS || '500');

export const options = {
  scenarios: {
    multi: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: VUS,
      maxVUs: MAX_VUS,
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<1000', 'p(99)<2000'],
  },
};

let authWarningPrinted = false;

export default function () {
  const headers = TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};

  endpoints.forEach((ep) => {
    if (ep.enabled === false) {
      return;
    }

    if (ep.auth && !TOKEN) {
      if (!authWarningPrinted) {
        console.warn(`Skipping auth-required endpoint ${ep.name} because TOKEN is empty`);
        authWarningPrinted = true;
      }
      return;
    }

    const url = `${BASE_URL}${ep.path}`;
    const body = ep.method === 'POST' ? ep.body || '{}' : null;
    const res = http.request(ep.method, url, body, { headers, tags: { endpoint: ep.name } });

    check(res, {
      [`${ep.name} status 2xx`]: (r) => r.status >= 200 && r.status < 300,
    });
  });

  sleep(1);
}
