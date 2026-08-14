import { check } from 'k6';
import { assessmentBoundMedicalCases, questionnaireDetailPath } from '../lib/data.js';
import { classifyAssessmentReadiness } from '../scenarios/chain-probe.js';

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

  check({ cases, ready, pending, noAssessment, failed }, {
    'chain probe only selects assessment-bound medical cases': (state) =>
      state.cases.length === 1 && state.cases[0].questionnaire_code === 'bound',
    'model-bound questionnaire discovery requests the exact version': () =>
      questionnaireDetailPath('Q/A', '1.0+1') === '/api/v1/questionnaires/Q%2FA?version=1.0%2B1',
    'ready assessment exposes its id': (state) =>
      state.ready.status === 'ready' && state.ready.assessmentID === '42',
    'pending assessment keeps polling': (state) => state.pending.status === 'pending',
    'no-assessment outcome is terminal': (state) => state.noAssessment.status === 'no_assessment_required',
    'failed assessment is terminal': (state) => state.failed.status === 'failed' && state.failed.assessmentID === '43',
  });
}
