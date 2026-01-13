import http from 'k6/http';
import { check } from 'k6';

// collection-server 问卷列表压测脚本（固定配置）
const BASE_URL = 'http://47.94.204.124:8082';
const TOKEN = 'eyJhbGciOiJSUzI1NiIsImtpZCI6ImI0NmExZTY4LWJlODMtNDFmNS1hYWIxLTA3MTUxNjlhOGVmNSIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjo2MDA5OTcwMzIyNjEzMzM1NTAsImFjY291bnRfaWQiOjYwMDk5NzAzMjI3ODExMDc2NiwiZXhwIjoxNzY4MjgzMzU1LCJqdGkiOiIwIiwiaWF0IjoxNzY4MjgyNDU1LCJuYmYiOjE3NjgyODI0NTUsInN1YiI6IjYwMDk5NzAzMjI2MTMzMzU1MCJ9.kHq6xgkJm88gIJYC4Qk3HsmdaOg7H2FCfnZJoIDb4WVOsWFcZdSToY0m79r613oFWLtVPWEwVjarUksAi2xs58TUBUEGrnar2GITiUEFoJjGLZ3vVUSH5Y9vxTVARAmgOVL2j0VOH60b8D-LGMoNV_IqupK7i4g03ndYhYx8J6vWAwD_dccN3As91HS1pI2sImfPTvzn25YXKRRw0vNvdSYJ5Pqfqn750j3_fV7X0Vn0p7ivvNTJ2FsPFKyLZMvxT2tcA_WfISqOTuBRxZLqMMnjrcFvcoznD8EKibQ0GHKbUA-wQqcJfHz1IC2RiwRFq-tWR4D2zfBpyXNCiXsGDw';
const RATE = 120;
const DURATION = '2m';
const VUS = 60;
const MAX_VUS = 200;

const DEFAULT_QUERY = {
  page: 1,
  page_size: 20,
  status: 'published',
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
  const res = http.get(`${BASE_URL}/api/v1/questionnaires?${buildQuery(DEFAULT_QUERY)}`, { headers });

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
