import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ChainSummaryPanel } from './ChainSummaryPanel';
import { apiClient } from '../api/client';
import type { Run, WorkspaceSummary } from '../api/queries';

function renderWithClient(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

const queuedRun: Run = {
  id: 'run-1',
  issue_id: 'issue-1',
  agent_id: 'agent-1',
  agent_name: 'Lead',
  status: 'queued',
  trigger_type: 'user',
  chain_id: 'chain-abc12345',
  chain_depth: 0,
  enqueued_at: '2026-05-22T10:00:00Z',
  attempt: 0,
  max_attempts: 3,
  agent_instructions_version: 1,
  input_tokens: 0,
  output_tokens: 0,
  total_cost_micros: 0,
  stdout_size_bytes: 0,
  exit_code: null,
  terminal_reason: '',
  failure_kind: '',
  cancel_reason: '',
  error_message: ''
} as Run;

const completedRun: Run = {
  ...queuedRun,
  id: 'run-2',
  status: 'completed',
  chain_id: 'chain-different'
};

const workspaceWithGuards: WorkspaceSummary = {
  id: 'ws-1',
  slug: 'demo',
  name: 'Demo',
  description: '',
  identifier_prefix: 'D',
  auto_chain_max_depth: 5,
  auto_chain_daily_cost_micros: 10_000_000
};

describe('ChainSummaryPanel', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('marks the row data-guard=warn when chain depth crosses 75% of the limit', () => {
    const nearLimit: Run = { ...queuedRun, chain_depth: 4 } as Run;
    const { container } = renderWithClient(
      <ChainSummaryPanel runs={[nearLimit]} issueID="issue-1" workspace={workspaceWithGuards} />
    );
    const row = container.querySelector('.chain-summary-row');
    expect(row?.getAttribute('data-guard')).toBe('warn');
  });

  it('marks the row data-guard=over when chain cost crosses the daily limit', () => {
    const overCost: Run = {
      ...queuedRun,
      total_cost_micros: 15_000_000
    } as Run;
    const { container } = renderWithClient(
      <ChainSummaryPanel runs={[overCost]} issueID="issue-1" workspace={workspaceWithGuards} />
    );
    const row = container.querySelector('.chain-summary-row');
    expect(row?.getAttribute('data-guard')).toBe('over');
  });

  it('shows a 체인 취소 button when the chain has a queued or running last run', () => {
    renderWithClient(<ChainSummaryPanel runs={[queuedRun]} issueID="issue-1" />);
    expect(screen.getByRole('button', { name: /체인 취소/ })).toBeInTheDocument();
  });

  it('hides the 체인 취소 button when the chain is terminal', () => {
    renderWithClient(<ChainSummaryPanel runs={[completedRun]} issueID="issue-1" />);
    expect(screen.queryByRole('button', { name: /체인 취소/ })).not.toBeInTheDocument();
  });

  it('calls POST /runs/chain/<id>/cancel when the operator confirms', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({ chain_id: queuedRun.chain_id, cancelled: 1 } as any);
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderWithClient(<ChainSummaryPanel runs={[queuedRun]} issueID="issue-1" />);
    fireEvent.click(screen.getByRole('button', { name: /체인 취소/ }));
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith(`/runs/chain/${queuedRun.chain_id}/cancel`, {});
    });
  });

  it('does not call cancel when the operator dismisses the confirm dialog', () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({ chain_id: queuedRun.chain_id, cancelled: 0 } as any);
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    renderWithClient(<ChainSummaryPanel runs={[queuedRun]} issueID="issue-1" />);
    fireEvent.click(screen.getByRole('button', { name: /체인 취소/ }));
    expect(post).not.toHaveBeenCalled();
  });
});
