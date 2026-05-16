import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { apiAuth } from '../api/client';
import { AuthTokenPanel } from './AuthTokenPanel';

const tokenStorageKey = 'cron_agent_dashboard_token';

function renderAuthTokenPanel() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false
      }
    }
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <AuthTokenPanel />
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe('apiAuth token storage', () => {
  it('prefers sessionStorage over localStorage and clears both stores', () => {
    apiAuth.setToken('local-token');
    expect(apiAuth.getToken()).toBe('local-token');
    expect(apiAuth.getTokenStorageMode()).toBe('local');

    apiAuth.setSessionToken('session-token');

    expect(apiAuth.getToken()).toBe('session-token');
    expect(apiAuth.getTokenStorageMode()).toBe('session');
    expect(window.sessionStorage.getItem(tokenStorageKey)).toBe('session-token');
    expect(window.localStorage.getItem(tokenStorageKey)).toBeNull();

    apiAuth.clearToken();

    expect(apiAuth.getToken()).toBe('');
    expect(apiAuth.getTokenStorageMode()).toBe('none');
    expect(window.sessionStorage.getItem(tokenStorageKey)).toBeNull();
    expect(window.localStorage.getItem(tokenStorageKey)).toBeNull();
  });
});

describe('AuthTokenPanel', () => {
  it('stores the token in sessionStorage when session-only is selected', () => {
    renderAuthTokenPanel();

    fireEvent.change(screen.getByLabelText('API token'), { target: { value: ' session-panel-token ' } });
    fireEvent.click(screen.getByRole('checkbox', { name: '이번 세션만 저장' }));
    fireEvent.click(screen.getByRole('button', { name: '토큰 저장 후 다시 시도' }));

    expect(window.sessionStorage.getItem(tokenStorageKey)).toBe('session-panel-token');
    expect(window.localStorage.getItem(tokenStorageKey)).toBeNull();
    expect(apiAuth.getToken()).toBe('session-panel-token');
  });
});
