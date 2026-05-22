import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';

import { WorkspaceWebhookSection } from './WorkspaceWebhookSection';
import { apiClient } from '../api/client';
import type { Webhook, WorkspaceSummary } from '../api/queries';

const workspace: WorkspaceSummary = {
  id: 'ws-1',
  slug: 'demo',
  name: 'Demo',
  description: '',
  identifier_prefix: 'D'
};

function renderWithClient(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('WorkspaceWebhookSection', () => {
  it('renders the empty-state copy when no webhook is registered', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation(async (path: string) => {
      if (path.includes('/webhooks')) {
        return { webhooks: [] } as any;
      }
      return {} as any;
    });

    renderWithClient(<WorkspaceWebhookSection workspace={workspace} />);

    expect(await screen.findByText(/등록된 webhook이 없습니다/)).toBeInTheDocument();
    expect(screen.getByText(/HMAC-SHA256/)).toBeInTheDocument();
  });

  it('renders registered webhook rows with the event chips and signature badge', async () => {
    const sample: Webhook = {
      id: 'wh-1',
      workspace_id: 'ws-1',
      url: 'https://example.com/hook',
      has_secret: true,
      events: ['run.completed', 'issue.done'],
      enabled: true,
      mask_pii: false,
      failed_delivery_count: 0,
      created_at: '2026-05-21T22:00:00Z',
      updated_at: '2026-05-21T22:00:00Z'
    };
    vi.spyOn(apiClient, 'get').mockImplementation(async (path: string) => {
      if (path.endsWith('/webhooks')) return { webhooks: [sample] } as any;
      if (path.includes('/deliveries')) return { deliveries: [] } as any;
      return {} as any;
    });

    renderWithClient(<WorkspaceWebhookSection workspace={workspace} />);

    expect(await screen.findByText('https://example.com/hook')).toBeInTheDocument();
    expect(screen.getByText('서명 사용')).toBeInTheDocument();
    // "run.completed" appears both as a checkbox label in the create form and
    // as an event chip on the registered row — getAllByText collects both.
    expect(screen.getAllByText('run.completed').length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText('issue.done').length).toBeGreaterThanOrEqual(2);
  });
});
