import {
  medicalReportWsSubscribeToFirstMessageLatency,
  behaviorReportWsSubscribeToFirstMessageLatency,
  personalityReportWsSubscribeToFirstMessageLatency,
} from '../lib/metrics.js';
import { structuredSummaryOutput } from '../lib/summary.js';

export const options = { vus: 1, iterations: 1 };

export default function () {
  medicalReportWsSubscribeToFirstMessageLatency.add(101);
  behaviorReportWsSubscribeToFirstMessageLatency.add(102);
  personalityReportWsSubscribeToFirstMessageLatency.add(103);
}

export function handleSummary(data) {
  return structuredSummaryOutput(data, __ENV.PERF_RAW_SUMMARY_FILE);
}
