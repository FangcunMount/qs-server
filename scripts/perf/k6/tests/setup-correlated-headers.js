import { check } from 'k6';
import { correlatedHeaders } from '../lib/http.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
  },
};

export function setup() {
  const first = correlatedHeaders({}, { endpoint: 'setup-discovery' });
  const second = correlatedHeaders({}, { endpoint: 'setup-discovery' });
  const preserved = correlatedHeaders(
    { 'X-Request-ID': 'caller-provided-request-id' },
    { endpoint: 'setup-discovery' }
  );

  return {
    firstRequestID: first['X-Request-ID'],
    secondRequestID: second['X-Request-ID'],
    preservedRequestID: preserved['X-Request-ID'],
    perfRunID: first['X-Perf-Run-ID'],
  };
}

export default function (data) {
  check(data, {
    'setup generates a request ID': (state) =>
      typeof state.firstRequestID === 'string' && state.firstRequestID.length > 0,
    'setup request IDs are unique': (state) =>
      state.firstRequestID !== state.secondRequestID,
    'caller-provided request ID is preserved': (state) =>
      state.preservedRequestID === 'caller-provided-request-id',
    'perf run ID is attached': (state) =>
      typeof state.perfRunID === 'string' && state.perfRunID.length > 0,
  });
}
