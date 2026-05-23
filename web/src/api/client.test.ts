import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { apiAuth, apiClient } from './client';

afterEach(() => {
  apiAuth.clearToken();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('apiClient.streamSSE', () => {
  it('sends the stored bearer token and dispatches SSE events', async () => {
    apiAuth.setToken('stream-secret');
    const encoder = new TextEncoder();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(encoder.encode(': stream open\n\nevent: wake\ndata: {"ok":true}\n\n'));
            controller.close();
          }
        }),
        { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
      )
    );
    vi.stubGlobal('fetch', fetchMock);

    const events: Array<{ event: string; data: string }> = [];
    const unsubscribe = apiClient.streamSSE('/events/stream', {
      onEvent: (event, data) => events.push({ event, data })
    });

    await waitFor(() => expect(events).toEqual([{ event: 'wake', data: '{"ok":true}' }]));
    const [, init] = fetchMock.mock.calls[0];
    expect((init?.headers as Record<string, string>).authorization).toBe('Bearer stream-secret');

    unsubscribe();
  });
});
