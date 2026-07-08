export const REPORT_MODE_WEBSOCKET = 'websocket';
export const REPORT_MODE_SHORT_POLL = 'short_poll';
export const REPORT_MODE_LONG_POLL = 'long_poll';

export const LONG_POLL_MEDICAL_REPORT_PATH =
  '/api/v1/assessments/{assessment_id}/wait-report?testee_id={testee_id}&timeout={report_timeout}';
export const LONG_POLL_PERSONALITY_REPORT_PATH =
  '/api/v1/typology-assessments/{assessment_id}/wait-report?testee_id={testee_id}&timeout={report_timeout}';
export const SHORT_POLL_MEDICAL_REPORT_PATH = '/api/v1/assessments/{assessment_id}/report-status?testee_id={testee_id}';
export const SHORT_POLL_PERSONALITY_REPORT_PATH =
  '/api/v1/typology-assessments/{assessment_id}/report-status?testee_id={testee_id}';

export function normalizeReportMode(raw) {
  const value = String(raw || '')
    .trim()
    .toLowerCase()
    .replace(/-/g, '_');
  if (value === 'ws' || value === REPORT_MODE_WEBSOCKET) {
    return REPORT_MODE_WEBSOCKET;
  }
  if (value === 'short' || value === REPORT_MODE_SHORT_POLL || value === 'report_status' || value === 'http_query' || value === 'poll') {
    return REPORT_MODE_SHORT_POLL;
  }
  if (value === 'long' || value === REPORT_MODE_LONG_POLL || value === 'wait_report') {
    return REPORT_MODE_LONG_POLL;
  }
  return '';
}

export function resolveReportModeFromInputs(inputs = {}) {
  const envMode = normalizeReportMode(inputs.envMode);
  if (envMode) {
    return envMode;
  }
  const configMode = normalizeReportMode(inputs.configMode);
  if (configMode) {
    return configMode;
  }
  if (inputs.websocketEnabled) {
    return REPORT_MODE_WEBSOCKET;
  }
  if (inputs.shortPollEnabled) {
    return REPORT_MODE_SHORT_POLL;
  }
  return REPORT_MODE_SHORT_POLL;
}

export function isWebSocketReportMode(mode) {
  return mode === REPORT_MODE_WEBSOCKET;
}

export function isShortPollReportMode(mode) {
  return mode === REPORT_MODE_SHORT_POLL;
}

export function defaultReportStatusPath(mode, kind) {
  if (isShortPollReportMode(mode)) {
    return kind === 'personality' ? SHORT_POLL_PERSONALITY_REPORT_PATH : SHORT_POLL_MEDICAL_REPORT_PATH;
  }
  return kind === 'personality' ? LONG_POLL_PERSONALITY_REPORT_PATH : LONG_POLL_MEDICAL_REPORT_PATH;
}
