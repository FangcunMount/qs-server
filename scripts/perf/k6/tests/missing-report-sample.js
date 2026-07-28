import { check } from 'k6';
import { validateScenarioData } from '../lib/data.js';
import {
  reportStatusQuery,
  medicalReportStatusQuery,
  personalityReportStatusQuery,
} from '../scenarios/report.js';
import {
  reportWsQuery,
  medicalReportWsQuery,
  personalityReportWsQuery,
} from '../scenarios/report-ws.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
    report_sample_skipped: ['count==6'],
  },
};

export default function () {
  const data = {
    testeeIDs: [],
    submitSubjects: [],
    questionnaireCodes: [],
    personalityQuestionnaireCodes: [],
    scaleCodes: [],
    modelCodes: [],
    medicalCases: [],
    personalityCases: [],
    answerTemplates: [],
    reportSamples: { medical: [], behavior: [], personality: [] },
  };

  let validation;
  let validationError = '';
  try {
    validation = validateScenarioData(data);
  } catch (err) {
    validationError = String(err && err.message ? err.message : err);
  }

  check({ validation, validationError }, {
    'missing report samples degrade instead of aborting setup': (state) =>
      state.validationError === ''
      && state.validation
      && state.validation.reportSampleCounts.total === 0
      && state.validation.missingReportSampleKinds.join(',') === 'report',
  });

  reportStatusQuery(data);
  medicalReportStatusQuery(data);
  personalityReportStatusQuery(data);
  reportWsQuery(data);
  medicalReportWsQuery(data);
  personalityReportWsQuery(data);
}
