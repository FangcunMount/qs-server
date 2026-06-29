import http from 'k6/http';
import { pick, is2xx } from './util.js';
import { endpointFailureCounters, setupDiscoveryFailed, http429Total, http401Total, http403Total, http4xxTotal, http5xxTotal, httpTransportErrorTotal, httpTimeoutTotal } from './metrics.js';
import {
  COLLECTION_BASE_URL,
  APISERVER_BASE_URL,
  COLLECTION_TOKENS,
  debugSetupRequest,
  APISERVER_TOKENS,
  HTTP_TIMEOUT,
} from './config.js';

export function timedRequest(method, baseURL, path, body, headers, tags) {
  return http.request(method, `${baseURL}${path}`, body, {
    headers,
    tags,
    timeout: HTTP_TIMEOUT,
  });
}

export function collectionToken() {
  return pick(COLLECTION_TOKENS);
}

export function apiserverToken() {
  return pick(APISERVER_TOKENS);
}

export function recordHTTPStatus(res, endpointFailedCounter, endpoint) {
  if (is2xx(res.status)) {
    return;
  }
  endpointFailedCounter.add(1, res.tags);
  recordEndpointFailureDetail(res, endpoint);
  if (res.status === 429) {
    http429Total.add(1, res.tags);
  }
  if (res.status === 401) {
    http401Total.add(1, res.tags);
  }
  if (res.status === 403) {
    http403Total.add(1, res.tags);
  }
  if (res.status >= 400 && res.status < 500) {
    http4xxTotal.add(1, res.tags);
  }
  if (res.status >= 500) {
    http5xxTotal.add(1, res.tags);
  }
  if (res.status === 0) {
    httpTransportErrorTotal.add(1, res.tags);
    if (isTimeoutResponse(res)) {
      httpTimeoutTotal.add(1, res.tags);
    }
  }
}

export function recordEndpointFailureDetail(res, endpoint) {
  const counters = endpointFailureCounters[endpoint];
  if (!counters) {
    return;
  }
  if (res.status >= 400 && res.status < 500) {
    counters.status4xx.add(1, res.tags);
  }
  if (res.status >= 500) {
    counters.status5xx.add(1, res.tags);
  }
  if (res.status === 0) {
    counters.transportError.add(1, res.tags);
    if (isTimeoutResponse(res)) {
      counters.timeout.add(1, res.tags);
    }
  }
}

export function isTimeoutResponse(res) {
  const message = String((res && res.error) || '');
  return message.toLowerCase().includes('timeout');
}

export function authHeaders(token) {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function jsonHeaders(token, requestID) {
  const headers = Object.assign({ 'Content-Type': 'application/json' }, authHeaders(token));
  if (requestID) {
    headers['X-Request-ID'] = requestID;
  }
  return headers;
}

export function responseData(res) {
  try {
    const parsed = res.json();
    if (parsed && parsed.data !== undefined) {
      return parsed.data || {};
    }
    return parsed || {};
  } catch (_) {
    return {};
  }
}

export function getCollectionData(path, endpoint) {
  const token = collectionToken();
  const res = timedRequest('GET', COLLECTION_BASE_URL, path, null, authHeaders(token), {
    endpoint,
    service: 'collection-server',
  });
  debugSetupRequest('collection-server', endpoint, path, res.status, token);
  if (!is2xx(res.status)) {
    recordHTTPStatus(res, setupDiscoveryFailed, endpoint);
    return null;
  }
  return responseData(res);
}

export function getApiserverData(path, endpoint) {
  const token = apiserverToken();
  const res = timedRequest('GET', APISERVER_BASE_URL, path, null, authHeaders(token), {
    endpoint,
    service: 'qs-apiserver',
  });
  debugSetupRequest('qs-apiserver', endpoint, path, res.status, token);
  if (!is2xx(res.status)) {
    recordHTTPStatus(res, setupDiscoveryFailed, endpoint);
    return null;
  }
  return responseData(res);
}

