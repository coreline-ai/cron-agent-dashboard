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
const TOKEN_STORAGE_KEY = 'corn_agent_dashboard_token';

function readToken() {
  if (typeof window === 'undefined') {
    return '';
  }
  return window.localStorage.getItem(TOKEN_STORAGE_KEY) ?? '';
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

export const apiClient = {
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
    })
};

export const apiAuth = {
  getToken: readToken,
  setToken(token: string) {
    window.localStorage.setItem(TOKEN_STORAGE_KEY, token.trim());
  },
  clearToken() {
    window.localStorage.removeItem(TOKEN_STORAGE_KEY);
  }
};
