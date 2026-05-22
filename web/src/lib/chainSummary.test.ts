import { describe, expect, it } from 'vitest';

import { summarizeChains } from './chainSummary';
import type { Run } from '../api/queries';

const baseRun = {
  status: 'done' as const,
  trigger_type: 'mention',
  agent_name: 'Lead',
  input_tokens: 0,
  output_tokens: 0,
  total_cost_micros: 0,
  attempt: 1,
  max_attempts: 1
} as unknown as Run;

describe('summarizeChains', () => {
  it('groups runs by chain_id and aggregates depth + tokens + cost', () => {
    const runs: Run[] = [
      {
        ...baseRun,
        id: 'r1',
        chain_id: 'c1',
        chain_depth: 0,
        agent_name: 'Lead',
        input_tokens: 10,
        output_tokens: 20,
        total_cost_micros: 1_000,
        enqueued_at: '2026-05-22T10:00:00Z',
        finished_at: '2026-05-22T10:00:30Z',
        status: 'done'
      } as Run,
      {
        ...baseRun,
        id: 'r2',
        chain_id: 'c1',
        chain_depth: 1,
        agent_name: 'Writer',
        input_tokens: 5,
        output_tokens: 8,
        total_cost_micros: 500,
        enqueued_at: '2026-05-22T10:01:00Z',
        finished_at: '2026-05-22T10:01:30Z',
        status: 'done'
      } as Run,
      {
        ...baseRun,
        id: 'r3',
        chain_id: 'c1',
        chain_depth: 1,
        agent_name: 'Lead',
        enqueued_at: '2026-05-22T10:02:00Z',
        status: 'running'
      } as Run
    ];
    const summaries = summarizeChains(runs);
    expect(summaries).toHaveLength(1);
    const c1 = summaries[0];
    expect(c1.chainID).toBe('c1');
    expect(c1.totalRuns).toBe(3);
    expect(c1.maxChainDepth).toBe(1);
    expect(c1.totalInputTokens).toBe(15);
    expect(c1.totalOutputTokens).toBe(28);
    expect(c1.totalCostMicros).toBe(1500);
    expect(c1.firstEnqueuedAt).toBe('2026-05-22T10:00:00Z');
    // last activity: r3 is running with no finished_at — fall back to its enqueued_at.
    expect(c1.lastActivityAt).toBe('2026-05-22T10:02:00Z');
    expect(c1.lastStatus).toBe('running');
    expect(c1.lastAgentName).toBe('Lead');
  });

  it('separates two chains and orders the newest chain first', () => {
    const runs: Run[] = [
      { ...baseRun, id: 'a', chain_id: 'old', enqueued_at: '2026-05-22T10:00:00Z' } as Run,
      { ...baseRun, id: 'b', chain_id: 'new', enqueued_at: '2026-05-22T12:00:00Z' } as Run
    ];
    const summaries = summarizeChains(runs);
    expect(summaries.map((s) => s.chainID)).toEqual(['new', 'old']);
  });

  it('falls back to run.id when chain_id is missing so legacy data still groups cleanly', () => {
    const runs: Run[] = [
      { ...baseRun, id: 'lone', chain_id: '', enqueued_at: '2026-05-22T10:00:00Z' } as Run
    ];
    const summaries = summarizeChains(runs);
    expect(summaries).toHaveLength(1);
    expect(summaries[0].chainID).toBe('lone');
  });

  it('returns an empty array on empty input', () => {
    expect(summarizeChains([])).toEqual([]);
  });
});
