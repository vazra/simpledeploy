import { sqliteExec, replaceMetricsAtomic } from './db.js';

// Synthetic metric helpers for driving the alert evaluator.

export function deleteAppMetrics(slug) {
  sqliteExec(`DELETE FROM metrics WHERE app_id = (SELECT id FROM apps WHERE slug='${slug}');`);
}

// startMetricRefresher keeps a full window of CPU points for slug in the
// metrics table (replacing anything the collector wrote) every 5s, so the
// evaluator always sees a breaching (high) or healthy window. The first
// write throws on error; later errors are kept in lastError() so a waitFor
// timeout can report them.
export function startMetricRefresher(slug, { high }) {
  let lastError = null;
  const write = () =>
    replaceMetricsAtomic(slug, { cpu: high ? 95 : 5, memoryMb: high ? 100 : 50 });
  write();
  const handle = setInterval(() => {
    try {
      write();
      lastError = null;
    } catch (e) {
      lastError = e;
    }
  }, 5_000);
  return {
    stop: () => clearInterval(handle),
    lastError: () => (lastError ? `metric refresher: ${lastError.message}` : ''),
  };
}
