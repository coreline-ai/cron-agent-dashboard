import type { Run } from '../api/queries';

export type ChainSummary = {
  chainID: string;
  runs: Run[];
  totalRuns: number;
  maxChainDepth: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostMicros: number;
  firstEnqueuedAt?: string;
  lastActivityAt?: string;
  // Run.status in queries.ts is RunStatus | string (the server can return
  // newly-added statuses without forcing a frontend rebuild), so mirror that
  // here.
  lastStatus?: string;
  lastAgentName?: string;
};

/**
 * summarizeChains groups runs by chain_id and produces one ChainSummary per
 * group. Runs without a chain_id (e.g. legacy data) fall under their own
 * synthetic chain so the panel still surfaces them. The summaries are
 * returned in reverse chronological order by first enqueue time so the
 * newest chain appears at the top of the panel.
 */
export function summarizeChains(runs: Run[]): ChainSummary[] {
  const groups = new Map<string, ChainSummary>();
  for (const run of runs) {
    const key = run.chain_id?.trim() || run.id;
    let summary = groups.get(key);
    if (!summary) {
      summary = {
        chainID: key,
        runs: [],
        totalRuns: 0,
        maxChainDepth: 0,
        totalInputTokens: 0,
        totalOutputTokens: 0,
        totalCostMicros: 0
      };
      groups.set(key, summary);
    }
    summary.runs.push(run);
    summary.totalRuns += 1;
    const depth = run.chain_depth ?? 0;
    if (depth > summary.maxChainDepth) {
      summary.maxChainDepth = depth;
    }
    summary.totalInputTokens += run.input_tokens ?? 0;
    summary.totalOutputTokens += run.output_tokens ?? 0;
    summary.totalCostMicros += run.total_cost_micros ?? 0;
  }

  for (const summary of groups.values()) {
    // Use the enqueued_at order to derive a stable "first" / "last" pair
    // even when finished_at is missing (still-running runs).
    summary.runs.sort((a, b) => (a.enqueued_at ?? '').localeCompare(b.enqueued_at ?? ''));
    summary.firstEnqueuedAt = summary.runs[0]?.enqueued_at;
    const last = summary.runs[summary.runs.length - 1];
    summary.lastActivityAt = last?.finished_at?.trim() || last?.heartbeat_at?.trim() || last?.started_at?.trim() || last?.enqueued_at;
    summary.lastStatus = last?.status;
    summary.lastAgentName = last?.agent_name;
  }

  return Array.from(groups.values()).sort((a, b) =>
    (b.firstEnqueuedAt ?? '').localeCompare(a.firstEnqueuedAt ?? '')
  );
}
