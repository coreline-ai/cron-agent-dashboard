import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, apiAuth } from '../api/client';

type AuthTokenPanelProps = {
  error?: unknown;
};

export function isUnauthorizedError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.status === 401;
}

export function AuthTokenPanel({ error }: AuthTokenPanelProps) {
  const queryClient = useQueryClient();
  const [token, setToken] = useState(() => apiAuth.getToken());
  const [sessionOnly, setSessionOnly] = useState(() => apiAuth.getTokenStorageMode() === 'session');
  const visible = error === undefined || isUnauthorizedError(error);

  if (!visible) {
    return null;
  }

  return (
    <article className="panel form-grid auth-panel">
      <h2>API 토큰 필요</h2>
      <p>서버가 token mode로 실행 중입니다. `--token` 값 또는 `CORN_AGENT_DASHBOARD_TOKEN` 값을 입력하면 UI 요청에 자동으로 Bearer 토큰을 붙입니다.</p>
      <p>기본값은 브라우저 localStorage 저장이며, “이번 세션만 저장”을 선택하면 sessionStorage에만 저장됩니다.</p>
      <input
        aria-label="API token"
        placeholder="Bearer token"
        type="password"
        value={token}
        onChange={(event) => setToken(event.target.value)}
      />
      <label className="checkbox-row">
        <input type="checkbox" checked={sessionOnly} onChange={(event) => setSessionOnly(event.target.checked)} />
        이번 세션만 저장
      </label>
      <div className="button-row">
        <button
          className="button"
          type="button"
          onClick={() => {
            apiAuth.setToken(token, { sessionOnly });
            queryClient.invalidateQueries();
          }}
        >
          토큰 저장 후 다시 시도
        </button>
        <button
          className="button secondary"
          type="button"
          onClick={() => {
            apiAuth.clearToken();
            setToken('');
            setSessionOnly(false);
            queryClient.invalidateQueries();
          }}
        >
          토큰 삭제
        </button>
      </div>
    </article>
  );
}
