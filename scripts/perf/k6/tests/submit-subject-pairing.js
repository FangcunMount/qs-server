import { check } from 'k6';
import {
  buildMedicalSubmitRequest,
  buildPersonalitySubmitRequest,
  normalizeReportSample,
} from '../lib/data.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
  },
};

export default function () {
  const submitSubjects = [
    { collection_token_index: 0, testee_id: '1001' },
    { collection_token_index: 1, testee_id: '2002' },
  ];
  const data = {
    submitSubjects,
    medicalCases: [{
      model_type: 'medical',
      questionnaire_code: 'MEDICAL',
      questionnaire_version: '1.0',
      testee_id: 'stale-medical-testee',
      answers: [{ question_code: 'Q1', question_type: 'Radio', value: 'A' }],
    }],
    answerTemplates: [],
    personalityCases: [{
      model_type: 'personality',
      questionnaire_code: 'PERSONALITY',
      questionnaire_version: '1.0',
      testee_id: 'stale-personality-testee',
      answers: [{ question_code: 'Q1', question_type: 'Radio', value: 'A' }],
    }],
  };

  const medical = buildMedicalSubmitRequest(data);
  const personality = buildPersonalitySubmitRequest(data);
  const report = normalizeReportSample({
    model_type: 'medical',
    assessment_id: '3003',
    testee_id: submitSubjects[1].testee_id,
    collection_token_index: submitSubjects[1].collection_token_index,
  });

  check({ medical, personality, report }, {
    'medical submit keeps token and testee paired': (state) =>
      state.medical.payload.testee_id === submitSubjects[state.medical.collectionTokenIndex].testee_id,
    'personality submit keeps token and testee paired': (state) =>
      state.personality.payload.testee_id === submitSubjects[state.personality.collectionTokenIndex].testee_id,
    'report sample keeps its authorized token index': (state) =>
      state.report.collection_token_index === 1 && state.report.testee_id === '2002',
  });
}
