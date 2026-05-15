import { describe, expect, it } from 'vitest';

import { formatCostMicros, formatTokens, summarizeRunUsage } from './IssueSummaryRail';
import type { Run } from '../api/queries';

describe('IssueSummaryRail usage helpers', () => {
  it('summarizes token and cost metrics across runs', () => {
    const runs = [
      { id: 'r1', status: 'done', trigger_type: 'issue_created', input_tokens: 1200, output_tokens: 300, total_cost_micros: 2500 },
      { id: 'r2', status: 'failed', trigger_type: 'rerun', input_tokens: 0, output_tokens: 0, total_cost_micros: 0 },
      { id: 'r3', status: 'done', trigger_type: 'mention', input_tokens: 10, output_tokens: 20, total_cost_micros: 500 }
    ] as Run[];

    expect(summarizeRunUsage(runs)).toEqual({
      inputTokens: 1210,
      outputTokens: 320,
      totalTokens: 1530,
      totalCostMicros: 3000,
      measuredRuns: 2
    });
  });

  it('formats token and cost values for compact UI display', () => {
    expect(formatTokens(1530)).toBe('1.5k');
    expect(formatTokens(2_500_000)).toBe('2.50M');
    expect(formatCostMicros(3000)).toBe('$0.0030');
  });
});
