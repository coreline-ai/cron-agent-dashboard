import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { DevTeamSeedCard } from './DevTeamSeedCard';
import { ToastProvider } from './ToastProvider';
import { apiClient } from '../api/client';

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <MemoryRouter>
          <DevTeamSeedCard />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}

describe('DevTeamSeedCard', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('renders the seed form with default slug', () => {
    mount();
    const input = screen.getByLabelText(/Workspace slug/) as HTMLInputElement;
    expect(input.value).toBe('ai-dev-team');
    expect(screen.getByRole('button', { name: /AI Dev Team 생성/ })).toBeInTheDocument();
  });

  it('calls POST /system/seed-dev-team with the trimmed slug and working_dir', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      workspace: { id: 'ws-1', slug: 'ai-dev-team', name: 'AI Dev Team' },
      agents: new Array(7).fill({ name: 'X', runtime: 'codex' }),
      skills: new Array(8).fill('skill'),
      assignment_count: 14,
      created_agent_count: 6,
      already_had: false
    } as any);
    mount();
    fireEvent.change(screen.getByLabelText(/Workspace slug/), { target: { value: '  myteam  ' } });
    fireEvent.change(screen.getByLabelText(/Working directory/), { target: { value: '  /tmp/proj  ' } });
    fireEvent.click(screen.getByRole('button', { name: /AI Dev Team 생성/ }));
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith('/system/seed-dev-team', {
        slug: 'myteam',
        working_dir: '/tmp/proj'
      });
    });
  });

  it('disables the submit button while the slug is blank', () => {
    mount();
    fireEvent.change(screen.getByLabelText(/Workspace slug/), { target: { value: '' } });
    expect(screen.getByRole('button', { name: /AI Dev Team 생성/ })).toBeDisabled();
  });
});
