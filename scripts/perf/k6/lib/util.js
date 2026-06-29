export function timeSnapshot(ms) {
  if (!ms) {
    return null;
  }
  const date = new Date(ms);
  return {
    epoch_ms: ms,
    local: formatLocalTime(date),
    utc: date.toISOString(),
    nginx_time: formatNginxTime(date),
    timezone_offset: formatTimezoneOffset(date),
  };
}

export function addDurationMs(startMs, duration) {
  const durationMs = parseDurationMs(duration);
  if (!startMs || durationMs === null) {
    return null;
  }
  return startMs + durationMs;
}

export function parseDurationMs(duration) {
  const text = String(duration || '').trim();
  if (!text) {
    return null;
  }
  const re = /(\d+(?:\.\d+)?)(ms|s|m|h)/g;
  let match = re.exec(text);
  let total = 0;
  let matched = false;
  while (match) {
    matched = true;
    const value = Number(match[1]);
    const unit = match[2];
    if (unit === 'ms') {
      total += value;
    } else if (unit === 's') {
      total += value * 1000;
    } else if (unit === 'm') {
      total += value * 60 * 1000;
    } else if (unit === 'h') {
      total += value * 60 * 60 * 1000;
    }
    match = re.exec(text);
  }
  return matched ? Math.floor(total) : null;
}

export function formatLocalTime(date) {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
    + `T${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}.${pad3(date.getMilliseconds())}`
    + formatTimezoneOffset(date);
}

export function formatNginxTime(date) {
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  return `${pad2(date.getDate())}/${months[date.getMonth()]}/${date.getFullYear()}:`
    + `${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())} ${formatTimezoneOffset(date).replace(':', '')}`;
}

export function formatTimezoneOffset(date) {
  const offsetMinutes = -date.getTimezoneOffset();
  const sign = offsetMinutes >= 0 ? '+' : '-';
  const absolute = Math.abs(offsetMinutes);
  return `${sign}${pad2(Math.floor(absolute / 60))}:${pad2(absolute % 60)}`;
}

export function pad2(value) {
  const text = String(value);
  return text.length >= 2 ? text : `0${text}`;
}

export function pad3(value) {
  const text = String(value);
  if (text.length >= 3) {
    return text;
  }
  return text.length === 2 ? `0${text}` : `00${text}`;
}

export function errorMessage(err) {
  if (!err) {
    return '';
  }
  return err.message ? String(err.message) : String(err);
}

export function pick(items) {
  if (!items || items.length === 0) {
    return '';
  }
  const vu = typeof __VU === 'undefined' ? 0 : __VU;
  const iter = typeof __ITER === 'undefined' ? 0 : __ITER;
  return items[(iter + vu) % items.length];
}

export function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

export function nonEmptyList(primary, fallback) {
  if (Array.isArray(primary) && primary.length > 0) {
    return primary;
  }
  return Array.isArray(fallback) ? fallback : [];
}

export function uniqueList(items) {
  const seen = {};
  const out = [];
  (items || []).forEach((item) => {
    const value = String(item || '').trim();
    if (!value || seen[value]) {
      return;
    }
    seen[value] = true;
    out.push(value);
  });
  return out;
}

export function uniqueReportSamples(samples) {
  const seen = {};
  const out = [];
  (samples || []).forEach((sample) => {
    const assessmentID = String(sample.assessment_id || '').trim();
    const testeeID = String(sample.testee_id || '').trim();
    const key = `${assessmentID}:${testeeID}`;
    if (!assessmentID || !testeeID || seen[key]) {
      return;
    }
    seen[key] = true;
    out.push({ assessment_id: assessmentID, testee_id: testeeID });
  });
  return out;
}

export function responseItems(data) {
  if (!data) {
    return [];
  }
  if (Array.isArray(data.items)) {
    return data.items;
  }
  if (Array.isArray(data.testees)) {
    return data.testees;
  }
  if (Array.isArray(data.assessments)) {
    return data.assessments;
  }
  if (Array.isArray(data.data)) {
    return data.data;
  }
  return [];
}

export function dateStringDaysAgo(offset) {
  const date = new Date(Date.now() - offset * 24 * 60 * 60 * 1000);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function normalizeBaseURL(url) {
  return url.replace(/\/+$/, '');
}

export function is2xx(status) {
  return status >= 200 && status < 300;
}

export function filePathCandidates(path, baseDirs) {
  const raw = String(path || '').trim();
  if (!raw) {
    return [];
  }
  if (isAbsolutePath(raw)) {
    return [raw];
  }

  const candidates = [];
  (baseDirs || []).forEach((baseDir) => {
    const normalizedBase = normalizeDirPath(baseDir);
    if (normalizedBase) {
      candidates.push(`${normalizedBase}/${trimLeadingDotSlash(raw)}`);
    }
  });
  candidates.push(raw);
  return uniqueList(candidates);
}

export function perfConfigBaseDirs() {
  // k6 open() 对裸相对路径以脚本目录 scripts/perf 为基准；
  // check-token-preflight.sh 则以配置文件所在目录为基准。此处补齐仓库根等候选路径。
  return uniqueList([
    __ENV.PERF_ROOT_DIR || '',
    __ENV.PWD || '',
    '../..',
  ]);
}

export function dirnamePath(path) {
  const normalized = String(path || '').replace(/\/+$/, '');
  const index = normalized.lastIndexOf('/');
  return index > 0 ? normalized.slice(0, index) : '';
}

export function normalizeDirPath(path) {
  return String(path || '').trim().replace(/\/+$/, '');
}

export function trimLeadingDotSlash(path) {
  return String(path || '').replace(/^\.\/+/, '');
}

export function isAbsolutePath(path) {
  return String(path || '').indexOf('/') === 0;
}

export function tryReadTextFile(path, baseDirs) {
  try {
    return readTextFile(path, baseDirs);
  } catch (_) {
    return null;
  }
}

export function readTextFile(path, baseDirs) {
  const candidates = filePathCandidates(path, baseDirs);
  let lastError = null;
  for (let i = 0; i < candidates.length; i += 1) {
    try {
      return { path: candidates[i], content: open(candidates[i]) };
    } catch (err) {
      lastError = err;
    }
  }
  throw new Error(`Cannot open ${path}. Tried: ${candidates.join(', ')}. ${lastError || ''}`);
}

export function listValue(value) {
  if (value === undefined || value === null || value === '') {
    return [];
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item || '').trim()).filter((item) => item.length > 0);
  }
  return String(value)
    .split(',')
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

export function parseListFileContent(content) {
  const raw = String(content || '').trim();
  if (!raw) {
    return [];
  }
  if (raw[0] === '[' || raw[0] === '{') {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item || '').trim()).filter((item) => item.length > 0);
    }
    if (Array.isArray(parsed.tokens)) {
      return parsed.tokens.map((item) => String(item || '').trim()).filter((item) => item.length > 0);
    }
  }
  return raw
    .split(/[,\n\r]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

