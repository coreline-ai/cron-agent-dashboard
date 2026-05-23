import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { WorkspaceRunsPage } from './WorkspaceRunsPage';
import { apiClient } from '../api/client';

function renderWithRoute(initialPath: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="/w/:slug/runs" element={<WorkspaceRunsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('WorkspaceRunsPage', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('renders the run rows newest-first and links to the issue detail page', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation(async (path: string) => {
      if (path.endsWith('/runs?limit=500')) {
        return {
          runs: [
            {
              id: 'run-2',
              issue_id: 'iss-2',
              status: 'failed',
              agent_name: 'Writer',
              chain_id: 'chain-22222222',
              enqueued_at: '2026-05-23T20:00:00Z',
              error_message: 'timeout',
              trigger_type: 'user'
            },
            {
              id: 'run-1',
              issue_id: 'iss-1',
              status: 'done',
              agent_name: 'Lead',
              chain_id: 'chain-11111111',
              enqueued_at: '2026-05-23T19:50:00Z',
              trigger_type: 'user'
            }
          ]
        } as any;
      }
      if (path.endsWith('/agents')) {
        return { agents: [{ id: 'agent-1', name: 'Lead', runtime: 'codex', instructions: 'lead', is_main: true }] } as any;
      }
      return {} as any;
    });
    renderWithRoute('/w/demo/runs');
    expect(await screen.findByText(/run · 2건/)).toBeInTheDocument();
    const links = screen.getAllByRole('link');
    expect(links[0].getAttribute('href')).toBe('/w/demo/issues/iss-2');
    expect(links[1].getAttribute('href')).toBe('/w/demo/issues/iss-1');
    expect(screen.getByText(/timeout/)).toBeInTheDocument();
  });

  it('filters by status', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation(async (path: string) => {
      if (path.endsWith('/runs?limit=500')) {
        return {
          runs: [
            { id: 'run-A', issue_id: 'iss-A', status: 'done', enqueued_at: '2026-05-23T20:00:00Z', trigger_type: 'user' },
            { id: 'run-B', issue_id: 'iss-B', status: 'failed', enqueued_at: '2026-05-23T19:50:00Z', trigger_type: 'user' }
          ]
        } as any;
      }
      if (path.endsWith('/agents')) {
        return { agents: [] } as any;
      }
      return {} as any;
    });
    renderWithRoute('/w/demo/runs');
    await screen.findByText(/run · 2건/);
    const statusSelect = screen.getByLabelText(/상태/) as HTMLSelectElement;
    statusSelect.value = 'done';
    statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    await waitFor(() => {
      expect(screen.getByText(/run · 1건/)).toBeInTheDocument();
    });
  });
});
