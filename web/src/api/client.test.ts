import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError, apiAuth, apiClient } from './client';

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
    const onOpen = vi.fn();
    const unsubscribe = apiClient.streamSSE('/events/stream', {
      onOpen,
      onEvent: (event, data) => events.push({ event, data })
    });

    await waitFor(() => expect(events).toEqual([{ event: 'wake', data: '{"ok":true}' }]));
    const [, init] = fetchMock.mock.calls[0];
    expect(onOpen).toHaveBeenCalledOnce();
    expect((init?.headers as Record<string, string>).accept).toBe('text/event-stream');
    expect((init?.headers as Record<string, string>).authorization).toBe('Bearer stream-secret');

    unsubscribe();
  });

  it('dispatches multi-line events when the stream closes without a trailing blank line', async () => {
    const encoder = new TextEncoder();
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          new ReadableStream({
            start(controller) {
              controller.enqueue(encoder.encode('event: note\rdata: first\rdata: second'));
              controller.close();
            }
          }),
          { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
        )
      )
    );

    const events: Array<{ event: string; data: string }> = [];
    const unsubscribe = apiClient.streamSSE('/events/stream', {
      onEvent: (event, data) => events.push({ event, data })
    });

    await waitFor(() => expect(events).toEqual([{ event: 'note', data: 'first\nsecond' }]));
    unsubscribe();
  });

  it('reports non-ok stream responses through onError', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ error: { message: 'no token' } }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' }
        })
      )
    );
    const onError = vi.fn();

    const unsubscribe = apiClient.streamSSE('/events/stream', {
      onEvent: vi.fn(),
      onError
    });

    await waitFor(() => expect(onError).toHaveBeenCalledOnce());
    expect(onError.mock.calls[0][0]).toBeInstanceOf(ApiError);
    expect(onError.mock.calls[0][0].message).toBe('no token');
    unsubscribe();
  });
});
