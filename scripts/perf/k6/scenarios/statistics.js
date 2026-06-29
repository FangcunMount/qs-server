import { check } from 'k6';
import { pick, is2xx } from '../lib/util.js';
import { scenarioData, renderPath } from '../lib/data.js';
import { timedRequest, authHeaders, apiserverToken, recordHTTPStatus } from '../lib/http.js';
import { APISERVER_BASE_URL, STATS_PATHS } from '../lib/config.js';
import { statisticsDuration, statisticsFailed } from '../lib/metrics.js';


export function statisticsQuery(data) {
  const ctx = scenarioData(data);
  const path = renderPath(pick(STATS_PATHS), null, ctx);
  const endpoint = 'statistics_query';
  const res = timedRequest('GET', APISERVER_BASE_URL, path, null, authHeaders(apiserverToken()), {
    endpoint,
    service: 'qs-apiserver',
  });

  statisticsDuration.add(res.timings.duration, res.tags);
  recordHTTPStatus(res, statisticsFailed, endpoint);
  check(res, {
    'statistics query status is 2xx': (r) => is2xx(r.status),
  });
}

