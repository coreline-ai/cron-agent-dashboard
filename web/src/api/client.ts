export type ApiErrorBody = {
  message?: string;
  code?: string;
  error?: {
    message?: string;
    code?: string;
    details?: unknown;
  };
};

export class ApiError extends Error {
  readonly status: number;
  readonly body: ApiErrorBody | null;

  constructor(status: number, body: ApiErrorBody | null) {
    super(body?.error?.message ?? body?.message ?? `API 요청 실패 (${status})`);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api';
const TOKEN_STORAGE_KEY = 'cron_agent_dashboard_token';

type TokenStorageMode = 'local' | 'session' | 'none';

type SetTokenOptions = {
  sessionOnly?: boolean;
};

type SSEHandlers = {
  onOpen?: () => void;
  onEvent: (event: string, data: string) => void;
  onError?: (error: unknown) => void;
  reconnect?: boolean;
  reconnectDelayMs?: number;
};

function readToken() {
  if (typeof window === 'undefined') {
    return '';
  }
  const sessionToken = window.sessionStorage.getItem(TOKEN_STORAGE_KEY);
  if (sessionToken !== null) {
    return sessionToken;
  }
  return window.localStorage.getItem(TOKEN_STORAGE_KEY) ?? '';
}

function readTokenStorageMode(): TokenStorageMode {
  if (typeof window === 'undefined') {
    return 'none';
  }
  if (window.sessionStorage.getItem(TOKEN_STORAGE_KEY) !== null) {
    return 'session';
  }
  if (window.localStorage.getItem(TOKEN_STORAGE_KEY) !== null) {
    return 'local';
  }
  return 'none';
}

function setToken(token: string, options: SetTokenOptions = {}) {
  if (typeof window === 'undefined') {
    return;
  }
  const normalizedToken = token.trim();
  if (options.sessionOnly) {
    window.sessionStorage.setItem(TOKEN_STORAGE_KEY, normalizedToken);
    window.localStorage.removeItem(TOKEN_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(TOKEN_STORAGE_KEY, normalizedToken);
  window.sessionStorage.removeItem(TOKEN_STORAGE_KEY);
}

function clearToken() {
  if (typeof window === 'undefined') {
    return;
  }
  window.sessionStorage.removeItem(TOKEN_STORAGE_KEY);
  window.localStorage.removeItem(TOKEN_STORAGE_KEY);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const token = readToken();
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: {
      ...Object.fromEntries(headers.entries())
    }
  });

  if (!response.ok) {
    let body: ApiErrorBody | null = null;
    try {
      body = (await response.json()) as ApiErrorBody;
    } catch {
      body = null;
    }
    throw new ApiError(response.status, body);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

function dispatchSSEBlock(block: string, onEvent: SSEHandlers['onEvent']) {
  let eventName = 'message';
  const dataLines: string[] = [];
  for (const rawLine of block.split('\n')) {
    const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;
    if (!line || line.startsWith(':')) {
      continue;
    }
    if (line.startsWith('event:')) {
      eventName = line.slice('event:'.length).trimStart();
      continue;
    }
    if (line.startsWith('data:')) {
      const value = line.slice('data:'.length);
      dataLines.push(value.startsWith(' ') ? value.slice(1) : value);
    }
  }
  if (dataLines.length > 0) {
    onEvent(eventName, dataLines.join('\n'));
  }
}

function drainSSEBuffer(buffer: string, onEvent: SSEHandlers['onEvent']) {
  const normalized = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
  let cursor = 0;
  while (true) {
    const idx = normalized.indexOf('\n\n', cursor);
    if (idx < 0) {
      return normalized.slice(cursor);
    }
    dispatchSSEBlock(normalized.slice(cursor, idx), onEvent);
    cursor = idx + 2;
  }
}

function flushSSEBuffer(buffer: string, onEvent: SSEHandlers['onEvent']) {
  const rest = drainSSEBuffer(buffer, onEvent);
  if (rest.trim()) {
    dispatchSSEBlock(rest, onEvent);
  }
}

function streamSSE(path: string, handlers: SSEHandlers) {
  let stopped = false;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  let controller: AbortController | undefined;

  const connect = async () => {
    controller = new AbortController();
    try {
      const headers = new Headers();
      headers.set('Accept', 'text/event-stream');
      const token = readToken();
      if (token) {
        headers.set('Authorization', `Bearer ${token}`);
      }
      const response = await fetch(`${API_BASE_URL}${path}`, {
        headers: {
          ...Object.fromEntries(headers.entries())
        },
        signal: controller.signal
      });
      if (!response.ok) {
        let body: ApiErrorBody | null = null;
        try {
          body = (await response.json()) as ApiErrorBody;
        } catch {
          body = null;
        }
        throw new ApiError(response.status, body);
      }
      if (!response.body) {
        throw new Error('SSE stream response has no body');
      }
      handlers.onOpen?.();
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (!stopped) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        buffer = drainSSEBuffer(buffer + decoder.decode(value, { stream: true }), handlers.onEvent);
      }
      flushSSEBuffer(buffer + decoder.decode(), handlers.onEvent);
    } catch (error) {
      if (!stopped && !(error instanceof DOMException && error.name === 'AbortError')) {
        handlers.onError?.(error);
      }
    } finally {
      if (!stopped && handlers.reconnect) {
        retryTimer = setTimeout(connect, handlers.reconnectDelayMs ?? 3_000);
      }
    }
  };

  void connect();

  return () => {
    stopped = true;
    if (retryTimer) {
      clearTimeout(retryTimer);
    }
    controller?.abort();
  };
}

export const apiClient = {
  // url resolves a relative API path to the absolute URL fetch should hit.
  // Reuses API_BASE_URL so calls work in both Vite-dev (proxied) and embedded
  // (Go binary) modes.
  url: (path: string) => `${API_BASE_URL}${path}`,
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'POST',
      body: body === undefined ? undefined : JSON.stringify(body)
    }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'PUT',
      body: body === undefined ? undefined : JSON.stringify(body)
    }),
  delete: <T>(path: string) =>
    request<T>(path, {
      method: 'DELETE'
    }),
  // postMultipart streams a FormData body. We can't reuse request() because
  // the JSON Content-Type default would prevent the browser from emitting
  // the multipart boundary.
  postMultipart: async <T>(path: string, body: FormData): Promise<T> => {
    const headers = new Headers();
    const token = readToken();
    if (token) {
      headers.set('Authorization', `Bearer ${token}`);
    }
    const response = await fetch(`${API_BASE_URL}${path}`, {
      method: 'POST',
      body,
      headers
    });
    if (!response.ok) {
      let parsed: ApiErrorBody | null = null;
      try {
        parsed = (await response.json()) as ApiErrorBody;
      } catch {
        parsed = null;
      }
      throw new ApiError(response.status, parsed);
    }
    if (response.status === 204) {
      return undefined as T;
    }
    return (await response.json()) as T;
  },
  // streamSSE consumes text/event-stream via fetch instead of EventSource so
  // token-mode dashboards can attach the Authorization header. Native
  // EventSource cannot set custom headers.
  streamSSE
};

export const apiAuth = {
  getToken: readToken,
  getTokenStorageMode: readTokenStorageMode,
  setToken,
  setSessionToken: (token: string) => setToken(token, { sessionOnly: true }),
  clearToken
};
