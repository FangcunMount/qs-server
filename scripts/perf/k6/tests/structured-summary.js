import { check } from 'k6';
import { structuredSummaryOutput } from '../lib/summary.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
  },
};

export default function () {
  check(true, { 'summary contract sample succeeds': (value) => value });
}

export function handleSummary(data) {
  return structuredSummaryOutput(data, __ENV.PERF_RAW_SUMMARY_FILE);
}
