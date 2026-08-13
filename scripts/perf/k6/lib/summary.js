function legacyThresholds(thresholds) {
  const result = {};
  for (const [expression, state] of Object.entries(thresholds || {})) {
    result[expression] = !(state && state.ok === true);
  }
  return result;
}

// k6 handleSummary() exposes metric values under `values`, while the existing
// perfctl contract consumes the legacy --summary-export shape. Normalize the
// in-memory summary so perfctl can own the human-readable terminal output.
export function buildPerfRawSummary(data) {
  const metrics = {};
  for (const [name, metric] of Object.entries((data && data.metrics) || {})) {
    const normalized = { ...((metric && metric.values) || {}) };
    if (metric && metric.type === 'rate' && normalized.rate !== undefined) {
      normalized.value = normalized.rate;
      delete normalized.rate;
    }
    if (metric && metric.thresholds && Object.keys(metric.thresholds).length > 0) {
      normalized.thresholds = legacyThresholds(metric.thresholds);
    }
    metrics[name] = normalized;
  }

  const result = { metrics };
  if (data && data.root_group !== undefined) {
    result.root_group = data.root_group;
  }
  if (data && data.setup_data !== undefined) {
    result.setup_data = data.setup_data;
  }
  return result;
}

export function structuredSummaryOutput(data, outputPath) {
  const encoded = `${JSON.stringify(buildPerfRawSummary(data), null, 2)}\n`;
  if (!outputPath) {
    return { stdout: encoded };
  }
  return { [outputPath]: encoded };
}
