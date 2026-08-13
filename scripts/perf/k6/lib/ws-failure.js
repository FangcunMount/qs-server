export function firstReportWsFailure(current, candidate) {
  const existing = String(current || '').trim();
  if (existing) {
    return existing;
  }
  const next = String(candidate || '').trim();
  return next || 'ws_unknown_error';
}

export function reportWsFailureCategory(reason) {
  switch (String(reason || '').trim()) {
    case 'capacity_exhausted':
      return 'capacity_rejected';
    case 'rate_limited':
      return 'rate_limited';
    case 'ws_transport_error':
      return 'transport_error';
    case 'ws_connect_status':
      return 'connect_failed';
    case 'ws_timeout':
      return 'timeout';
    case 'ws_status_message_missing':
      return 'message_missing';
    case 'ws_decode_error':
    case 'bad_request':
    case 'already_subscribed':
      return 'protocol_error';
    default:
      return 'server_rejected';
  }
}
