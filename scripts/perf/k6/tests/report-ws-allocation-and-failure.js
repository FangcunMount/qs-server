import { check } from 'k6';
import { pickReportSampleForIteration, reportSamplesForLane } from '../lib/data.js';
import { firstReportWsFailure, reportWsFailureCategory } from '../lib/ws-failure.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: { checks: ['rate==1'] },
};

export default function () {
  const lanes = ['medical', 'behavior', 'personality'];
  const samples = [];
  for (let id = 1; id <= 30; id += 1) {
    samples.push({ assessment_id: String(1000 + id), testee_id: String(id) });
    samples.push({ assessment_id: String(2000 + id), testee_id: String(id) });
  }
  const lanePools = lanes.map((lane) => reportSamplesForLane(samples, lane, lanes));
  const owners = {};
  lanePools.forEach((pool, laneIndex) => pool.forEach((sample) => {
    owners[sample.testee_id] = (owners[sample.testee_id] || []).concat(laneIndex);
  }));

  const first = pickReportSampleForIteration(samples, 0, 'medical', lanes);
  const wrapped = pickReportSampleForIteration(samples, lanePools[0].length, 'medical', lanes);
  const failure = firstReportWsFailure(
    firstReportWsFailure('', 'capacity_exhausted'),
    'ws_status_message_missing'
  );

  check({ lanePools, owners, first, wrapped, failure }, {
    'each lane receives deterministic samples': (state) => state.lanePools.every((pool) => pool.length > 0),
    'testees belong to one active websocket lane': (state) => Object.values(state.owners).every((value) => value.length === 1),
    'multiple assessments do not duplicate a testee in one lane': (state) =>
      state.lanePools.every((pool) => new Set(pool.map((sample) => sample.testee_id)).size === pool.length),
    'iteration selection wraps deterministically': (state) => state.first.testee_id === state.wrapped.testee_id,
    'first websocket failure wins': (state) => state.failure === 'capacity_exhausted',
    'capacity failure keeps explicit classification': (state) => reportWsFailureCategory(state.failure) === 'capacity_rejected',
  });
}
