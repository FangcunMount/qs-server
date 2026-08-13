import { check } from 'k6';
import { structuredSummaryOutput } from '../lib/summary.js';

const scenarios = {
  structured_summary: {
    executor: 'shared-iterations',
    exec: 'default',
    vus: 1,
    iterations: 1,
    maxDuration: '10s',
  },
};

export const options = {
  scenarios,
  thresholds: {
    checks: ['rate==1'],
  },
};

export default function () {
  check(true, { 'summary contract sample succeeds': (value) => value });
}

export function handleSummary(data) {
  return structuredSummaryOutput(data, __ENV.PERF_RAW_SUMMARY_FILE, { scenarios });
}
