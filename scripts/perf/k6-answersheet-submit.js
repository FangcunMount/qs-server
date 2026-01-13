import http from 'k6/http';
import { check } from 'k6';

// 固定配置，直接在脚本里改，不再依赖 CLI 传参
const BASE_URL = 'http://47.94.204.124:8082'; // 默认指向 collection-server
const TOKEN = 'eyJhbGciOiJSUzI1NiIsImtpZCI6ImI0NmExZTY4LWJlODMtNDFmNS1hYWIxLTA3MTUxNjlhOGVmNSIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjo2MDA5OTcwMzIyNjEzMzM1NTAsImFjY291bnRfaWQiOjYwMDk5NzAzMjI3ODExMDc2NiwiZXhwIjoxNzY4Mjg5NDY3LCJqdGkiOiIwIiwiaWF0IjoxNzY4Mjg4NTY3LCJuYmYiOjE3NjgyODg1NjcsInN1YiI6IjYwMDk5NzAzMjI2MTMzMzU1MCJ9.DmYg7gAXMS6n0DRbQJ6h2ZkDQmxFcGOu8P2Ek1rwleFNeA9Qeg2eCFH7BJn9Cte6nSgZ9AJTfjdu5fqzy1iYl4YC1x2JcOda_1IO0WIgcJ7moAQOwsGR6Hyzvd2BTSpEqwovfRDhX3TcqKuIOYiGdj-wKEnofAhJhvzqqlkLIs1W9FmJkOgxaDFEPs6TorsuuaTVmT-diZd6RmU7X-DfGuB0W7Lsj9Unf1pFfKMZHHaJ0mv3X3VYWZmn-n_leT_qD3vn6YNOhnsC9O1CbZFLguENpRUS0eDC8Xi3yWshPuDQUKE3T3xmZPd8HSKeb7i2YwBqsbpVkkU6_JDY8-b86A';
const Q_CODE = 'kTC43z';
const Q_VER = '3.0.1';
const TESTEE_ID = '601002327771460142';
const TITLE = 'SNAP-IV量表（26项）';
const ANSWERS_JSON = '{"questionnaire_code":"kTC43z","questionnaire_version":"3.0.1","testee_id":"601002327771460142","answers":[{"question_code":"1o8TK1yK","question_type":"Radio","value":"g1B0fi9d","score":0},{"question_code":"s2mDjfLM","question_type":"Radio","value":"jaotwhPn","score":0},{"question_code":"xr4bamDJ","question_type":"Radio","value":"msBRRem0","score":0},{"question_code":"eFaOg0aj","question_type":"Radio","value":"o0I6fUgQ","score":0},{"question_code":"N2wkwerQ","question_type":"Radio","value":"T40zZWo3","score":0},{"question_code":"fFGmZqRX","question_type":"Radio","value":"L9o1wGMx","score":0},{"question_code":"DIpi10Jy","question_type":"Radio","value":"kPzYcRtr","score":0},{"question_code":"Smyp3j77","question_type":"Radio","value":"iySkBtYo","score":0},{"question_code":"ptxhTQF4","question_type":"Radio","value":"X86m5nHG","score":0},{"question_code":"8mnZsIvk","question_type":"Radio","value":"oQiblp0P","score":0},{"question_code":"PeIoQ2cG","question_type":"Radio","value":"1iuI8jAs","score":0},{"question_code":"ifqD5bZx","question_type":"Radio","value":"qQJ9c97Z","score":0},{"question_code":"h6oRzkwo","question_type":"Radio","value":"7XFLyE6z","score":0},{"question_code":"TZprcbG8","question_type":"Radio","value":"wPY00SS5","score":0},{"question_code":"c11KPY2s","question_type":"Radio","value":"3CpUpzNn","score":0},{"question_code":"EdBIIcVk","question_type":"Radio","value":"CQaHxXKU","score":0},{"question_code":"SngzUidL","question_type":"Radio","value":"rXXK3S5G","score":0},{"question_code":"1UNccBXJ","question_type":"Radio","value":"7eK3Fx3U","score":0},{"question_code":"At3poR2A","question_type":"Radio","value":"yFc1Wpk2","score":0},{"question_code":"pzch9Eoj","question_type":"Radio","value":"iIjbeWYK","score":0},{"question_code":"OxucPtrh","question_type":"Radio","value":"wpB1kaRE","score":0},{"question_code":"Jthnbf7n","question_type":"Radio","value":"8lk2CNG6","score":0},{"question_code":"F6ot2x6r","question_type":"Radio","value":"bHqKOdUR","score":0},{"question_code":"MLJ5xis8","question_type":"Radio","value":"eKawzkmb","score":0},{"question_code":"x9p8iuDq","question_type":"Radio","value":"2bC5BhGu","score":0},{"question_code":"V262O2Gv","question_type":"Radio","value":"F2cgLXZy","score":0}],"title":"SNAP-IV量表（26项）"}';

const RATE = 100;
const DURATION = '60s';
const VUS = 10;
const MAX_VUS = 20;

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: RATE, // 每秒期望请求数
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: VUS,
      maxVUs: MAX_VUS,
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'], // 错误率 <1%
    http_req_duration: ['p(95)<800', 'p(99)<1500'], // 延迟阈值
  },
};

export default function () {
  const headers = TOKEN
    ? {
        Authorization: `Bearer ${TOKEN}`,
        'Content-Type': 'application/json',
      }
    : { 'Content-Type': 'application/json' };

  const payload = buildPayload();
  const res = http.post(`${BASE_URL}/api/v1/answersheets`, JSON.stringify(payload), { headers });

  check(res, {
    'status 200': (r) => r.status === 200,
  });
}

function buildPayload() {
  if (ANSWERS_JSON) {
    const raw = JSON.parse(ANSWERS_JSON);
    const payload = {
      questionnaire_code: raw.questionnaire_code || raw.questionnaireCode || Q_CODE,
      questionnaire_version: raw.questionnaire_version || raw.questionnaireVersion || Q_VER,
      testee_id: parseId(raw.testee_id || raw.testeeId || raw.testeeID || TESTEE_ID),
      title: raw.title || TITLE || undefined,
      answers: [],
    };
    if (Array.isArray(raw.answers)) {
      payload.answers = raw.answers.map((a) => ({
        question_code: a.question_code || a.questionCode,
        question_type: a.question_type || a.questionType,
        value: a.value,
      }));
    }
    if (!payload.title) delete payload.title;
    return payload;
  }

  // 默认示例
  return {
    questionnaire_code: Q_CODE,
    questionnaire_version: Q_VER,
    testee_id: parseId(TESTEE_ID),
    title: TITLE || 'demo submission',
    answers: [
      {
        question_code: 'Q1',
        question_type: 'Radio',
        value: 'A',
      },
      {
        question_code: 'Q2',
        question_type: 'Text',
        value: 'demo answer',
      },
    ],
  };
}

// 将 ID 转为安全的数字，超出安全范围则返回字符串
function parseId(val) {
  if (val === undefined || val === null || val === '') return undefined;
  const str = String(val);
  const num = Number(str);
  if (!Number.isNaN(num) && Math.abs(num) <= Number.MAX_SAFE_INTEGER) {
    return num;
  }
  return str;
}
