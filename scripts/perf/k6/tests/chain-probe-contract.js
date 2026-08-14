import { check } from 'k6';
import { assessmentBoundMedicalCases, questionnaireDetailPath } from '../lib/data.js';
import { chainProbePollDelaySeconds, classifyAssessmentReadiness } from '../scenarios/chain-probe.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: { checks: ['rate==1'] },
};

export default function () {
  const cases = assessmentBoundMedicalCases([
    { questionnaire_code: 'generic', scale_code: '' },
    { questionnaire_code: 'bound', scale_code: 'scale-1' },
  ]);
  const ready = classifyAssessmentReadiness({ status: 'ready', assessment_id: '42' });
  const pending = classifyAssessmentReadiness({ status: 'pending' });
  const noAssessment = classifyAssessmentReadiness({ status: 'no_assessment_required' });
  const failed = classifyAssessmentReadiness({ status: 'failed', assessment_id: '43' });

  const backoff = [0, 1, 2, 3, 4, 5].map((attempt) =>
    chainProbePollDelaySeconds(attempt, 0, 1, 10));
  const serverHint = chainProbePollDelaySeconds(1, 5000, 1, 10);
  const cappedHint = chainProbePollDelaySeconds(1, 30000, 1, 10);

  check({ cases, ready, pending, noAssessment, failed, backoff, serverHint, cappedHint }, {
    'chain probe only selects assessment-bound medical cases': (state) =>
      state.cases.length === 1 && state.cases[0].questionnaire_code === 'bound',
    'model-bound questionnaire discovery requests the exact version': () =>
      questionnaireDetailPath('Q/A', '1.0+1') === '/api/v1/questionnaires/Q%2FA?version=1.0%2B1',
    'ready assessment exposes its id': (state) =>
      state.ready.status === 'ready' && state.ready.assessmentID === '42',
    'pending assessment keeps polling': (state) => state.pending.status === 'pending',
    'no-assessment outcome is terminal': (state) => state.noAssessment.status === 'no_assessment_required',
    'failed assessment is terminal': (state) => state.failed.status === 'failed' && state.failed.assessmentID === '43',
    'chain probe polling backs off and caps at ten seconds': (state) =>
      JSON.stringify(state.backoff) === JSON.stringify([1, 2, 4, 8, 10, 10]),
    'chain probe polling respects a larger server hint': (state) => state.serverHint === 5,
    'chain probe polling caps an excessive server hint': (state) => state.cappedHint === 10,
  });
}
