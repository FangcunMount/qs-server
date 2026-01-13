import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18082';
const RAW_PATH = __ENV.PATH || '/api/v1/public/info';
const TOKEN = __ENV.TOKEN || '';
const TESTEE_ID = __ENV.TESTEE_ID || '';
const METHOD = (__ENV.METHOD || '').toUpperCase() || (RAW_PATH.includes('answersheets') ? 'POST' : 'GET');
const QUESTIONNAIRE_CODE = __ENV.Q_CODE || __ENV.QUESTIONNAIRE_CODE || '';
const QUESTIONNAIRE_VER = __ENV.Q_VER || __ENV.QUESTIONNAIRE_VER || '';
const FILLER_ID = __ENV.FILLER_ID || '';
const ANSWERS_JSON = __ENV.ANSWERS || '';

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RPS || '100'), // 每秒期望请求数
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
  let path = buildPathWithTestee(RAW_PATH, TESTEE_ID);

  let res;
  if (METHOD === 'POST') {
    headers['Content-Type'] = 'application/json';
    const payload = buildAnswerSheetPayload();
    res = http.post(`${BASE_URL}${path}`, JSON.stringify(payload), { headers });
  } else {
    res = http.get(`${BASE_URL}${path}`, { headers });
  }

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

// 构建答卷提交 payload（管理员提交）
function buildAnswerSheetPayload() {
  // 优先使用环境变量传入的答案 JSON（格式需符合接口）
  if (ANSWERS_JSON) {
    const raw = JSON.parse(ANSWERS_JSON);
    // 规范字段名称
    const payload = {
      questionnaire_code: raw.questionnaire_code || raw.questionnaireCode || QUESTIONNAIRE_CODE,
      questionnaire_version: raw.questionnaire_version || raw.questionnaireVersion || QUESTIONNAIRE_VER,
      title: raw.title,
      testee_id: parseId(raw.testee_id || raw.testeeId || raw.testeeID || TESTEE_ID),
      filler_id: parseId(raw.filler_id || raw.fillerId || raw.fillerID || raw.writer_id || raw.writerId || raw.writerID || FILLER_ID),
      answers: [],
    };

    if (Array.isArray(raw.answers)) {
      payload.answers = raw.answers.map((a) => ({
        question_code: a.question_code || a.questionCode,
        question_type: a.question_type || a.questionType,
        value: a.value,
      }));
    }

    // 删除未设置的可选字段
    if (!payload.filler_id) delete payload.filler_id;
    if (!payload.title) delete payload.title;

    return payload;
  }

  // 基础必填字段
  const payload = {
    questionnaire_code: QUESTIONNAIRE_CODE || 'QCODE_DEMO',
    questionnaire_version: QUESTIONNAIRE_VER || '1.0',
    testee_id: parseId(TESTEE_ID) || 601002327771460142, // 默认 demo 受试者
    filler_id: parseId(FILLER_ID),
    answers: [
      {
        question_code: 'Q1',
        question_type: 'single',
        value: 'A',
      },
      {
        question_code: 'Q2',
        question_type: 'text',
        value: 'demo answer',
      },
    ],
  };

  // 删除未设置的可选字段
  if (!payload.filler_id) {
    delete payload.filler_id;
  }

  return payload;
}

// 将 ID 转为 Number（在安全范围内），超出安全范围则保留原始字符串
function parseId(val) {
  if (val === undefined || val === null || val === '') return undefined;
  const str = String(val);
  const num = Number(str);
  if (!Number.isNaN(num) && Math.abs(num) <= Number.MAX_SAFE_INTEGER) {
    return num;
  }
  // 超出 JS 安全整数范围，返回字符串以避免精度丢失（需后端支持字符串数字）
  return str;
}
