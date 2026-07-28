import { uniqueList, errorMessage, timeSnapshot, addDurationMs } from './lib/util.js';
import {
  PERF_CONFIG_PATH,
  DURATION,
  HTTP_TIMEOUT,
  REPORT_TIMEOUT,
  RUN_ID,
  SCRIPT_INIT_AT_MS,
  QUESTIONNAIRE_CODES,
  SCALE_CODES,
  PERSONALITY_MODEL_CODES,
  USER_AGENT,
  debugSetupState,
  boolEnv,
} from './lib/config.js';
import { buildThresholds } from './lib/metrics.js';
import { scenarios } from './lib/options.js';
import {
  discoverTesteeIDs,
  discoverSubmitSubjects,
  discoverMedicalCases,
  discoverPersonalityCases,
  discoverReportSamples,
  validateScenarioData,
  buildRunTiming,
  logPerfTimeEvent,
  hydrateStaticFixtures,
} from './lib/data.js';

import './lib/options.js';

export * from './scenarios/model-query.js';
export * from './scenarios/submit.js';
export * from './scenarios/report.js';
export * from './scenarios/report-ws.js';
export * from './scenarios/statistics.js';
export * from './scenarios/chain-probe.js';

export const options = {
  scenarios,
  thresholds: buildThresholds(),
  noConnectionReuse: boolEnv('NO_CONNECTION_REUSE', false),
  userAgent: USER_AGENT,
};

export function setup() {
  hydrateStaticFixtures();
  const runTiming = buildRunTiming();
  logPerfTimeEvent('setup_start', runTiming.setupStartAtMs, {
    script_init: timeSnapshot(runTiming.scriptInitAtMs),
    config: PERF_CONFIG_PATH || '<none>',
    duration: DURATION,
    http_timeout: HTTP_TIMEOUT,
    report_timeout_seconds: REPORT_TIMEOUT,
    report_mode: runTiming.report_mode,
    report_vuser_defaults: runTiming.report_vuser_defaults,
    qps: runTiming.qps,
    base_urls: runTiming.baseUrls,
  });
  debugSetupState();
  try {
    const discoveredTesteeIDs = discoverTesteeIDs();
    const submitSubjects = discoverSubmitSubjects();
    const testeeIDs = uniqueList(
      discoveredTesteeIDs.concat(submitSubjects.map((subject) => subject.testee_id))
    );
    const medicalBundle = discoverMedicalCases(testeeIDs);
    const personalityBundle = discoverPersonalityCases(testeeIDs, submitSubjects);
    const reportSamples = discoverReportSamples(testeeIDs, submitSubjects);
    const data = {
      testeeIDs,
      submitSubjects,
      questionnaireCodes: uniqueList(
        QUESTIONNAIRE_CODES.concat(medicalBundle.questionnaireCodes)
      ),
      personalityQuestionnaireCodes: personalityBundle.questionnaireCodes,
      scaleCodes: SCALE_CODES,
      modelCodes: uniqueList(PERSONALITY_MODEL_CODES.concat(personalityBundle.modelCodes)),
      medicalCases: medicalBundle.cases,
      personalityCases: personalityBundle.cases,
      answerTemplates: medicalBundle.cases,
      reportSamples,
    };
    const validation = validateScenarioData(data);
    logPerfTimeEvent('setup_data_ready', Date.now(), {
      testee_count: data.testeeIDs.length,
      submit_subject_count: data.submitSubjects.length,
      collection_tokens_with_testees: uniqueList(data.submitSubjects.map((subject) => subject.collection_token_index)).length,
      medical_case_count: data.medicalCases.length,
      personality_case_count: data.personalityCases.length,
      report_sample_counts: validation.reportSampleCounts,
      skipped_report_sample_kinds: validation.missingReportSampleKinds,
      report_scenarios_degraded: validation.missingReportSampleKinds.length > 0,
    });
    runTiming.trafficStartAtMs = Date.now();
    runTiming.trafficPlannedEndAtMs = addDurationMs(runTiming.trafficStartAtMs, DURATION);
    logPerfTimeEvent('traffic_start_estimate', runTiming.trafficStartAtMs, {
      setup_duration_ms: runTiming.trafficStartAtMs - runTiming.setupStartAtMs,
      traffic_planned_end: timeSnapshot(runTiming.trafficPlannedEndAtMs),
      note: 'scenario traffic starts immediately after setup returns',
    });
    data._perfRun = runTiming;
    return data;
  } catch (err) {
    logPerfTimeEvent('setup_failed', Date.now(), {
      setup_duration_ms: Date.now() - runTiming.setupStartAtMs,
      error: errorMessage(err),
    });
    throw err;
  }
}

export function teardown(data) {
  const runTiming = data && data._perfRun ? data._perfRun : {};
  const endedAtMs = Date.now();
  logPerfTimeEvent('run_end', endedAtMs, {
    run_id: runTiming.runId || RUN_ID,
    script_init: timeSnapshot(runTiming.scriptInitAtMs || SCRIPT_INIT_AT_MS),
    setup_start: timeSnapshot(runTiming.setupStartAtMs),
    traffic_start: timeSnapshot(runTiming.trafficStartAtMs),
    traffic_planned_end: timeSnapshot(runTiming.trafficPlannedEndAtMs),
    traffic_elapsed_ms: runTiming.trafficStartAtMs ? endedAtMs - runTiming.trafficStartAtMs : null,
    run_elapsed_ms: runTiming.setupStartAtMs ? endedAtMs - runTiming.setupStartAtMs : null,
  });
}
