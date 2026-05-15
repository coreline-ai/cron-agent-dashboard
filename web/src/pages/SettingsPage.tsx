import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiAuth, apiClient } from '../api/client';
import { AuthTokenPanel, isUnauthorizedError } from '../components/AuthTokenPanel';
import { PageHeader } from '../components/PageHeader';
import { useSettingsQuery } from '../api/queries';

function formatTokens(value?: number) {
  const n = value ?? 0;
  if (n >= 1_000_000) {
    return `${(n / 1_000_000).toFixed(2)}M`;
  }
  if (n >= 1_000) {
    return `${(n / 1_000).toFixed(1)}k`;
  }
  return String(n);
}

function formatCostMicros(value?: number) {
  return `$${((value ?? 0) / 1_000_000).toFixed(4)}`;
}

function formatBytes(value?: number) {
  if (!value) {
    return '0 B';
  }
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

export function SettingsPage() {
  const settings = useSettingsQuery();
  const queryClient = useQueryClient();
  const data = settings.data;
  const [message, setMessage] = useState('');
  const [token, setToken] = useState(() => apiAuth.getToken());
  const [backupPath, setBackupPath] = useState('');
  const [cleanupDays, setCleanupDays] = useState('30');
  const backup = useMutation({
    mutationFn: () => apiClient.post<{ backup_path: string; size_bytes: number }>('/system/backup', backupPath.trim() ? { to: backupPath.trim() } : {}),
    onSuccess: (res) => setMessage(`백업 완료: ${res.backup_path} (${formatBytes(res.size_bytes)})`)
  });
  const vacuum = useMutation({
    mutationFn: () => apiClient.post<{ reclaimed_bytes: number }>('/system/vacuum'),
    onSuccess: (res) => setMessage(`Vacuum 완료: ${formatBytes(res.reclaimed_bytes)} 회수`)
  });
  const cleanup = useMutation({
    mutationFn: () => apiClient.post<{ deleted_files: number; freed_bytes: number }>('/system/cleanup-logs', { days: Number(cleanupDays) || 30 }),
    onSuccess: (res) => setMessage(`로그 정리 완료: ${res.deleted_files}개 삭제, ${formatBytes(res.freed_bytes)} 회수`)
  });

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="전역"
        title="설정"
        description="서버 설정은 확인 중심이고, 브라우저 토큰/운영 유지보수 작업은 여기서 직접 실행합니다."
      />

      <article className="panel settings-card read-only-note">
        <div>
          <h2>서버 설정</h2>
          <p>아래 값은 현재 실행 중인 서버 상태입니다. 데이터 디렉토리, 타임존, worker 수, 인증 모드는 CLI 플래그나 환경변수로 바꾸고 서버를 재시작해야 합니다.</p>
        </div>
        <dl className="detail-list settings-detail-list">
          <dt>상태</dt>
          <dd>{settings.isLoading ? '확인 중' : settings.isError ? 'API 연결 대기' : '연결됨'}</dd>
          <dt>버전</dt>
          <dd>{data?.version ?? '-'}</dd>
          <dt>데이터 디렉토리</dt>
          <dd>{data?.data_dir ?? '-'}</dd>
          <dt>테마</dt>
          <dd>좌측 하단 Light/Dark 버튼에서 변경</dd>
          <dt>타임존</dt>
          <dd>{data?.timezone ?? 'Asia/Seoul'}</dd>
          <dt>Worker</dt>
          <dd>{data?.worker_pool_size ?? 3}</dd>
          <dt>인증</dt>
          <dd>{data?.auth_mode ?? 'none'}</dd>
          <dt>런타임</dt>
          <dd>
            {data?.available_runtimes?.length
              ? data.available_runtimes.map((runtime) => `${runtime.name}${runtime.version ? ` (${runtime.version})` : ''}`).join(', ')
              : 'PATH에서 탐지된 런타임 없음'}
          </dd>
          <dt>7일 토큰</dt>
          <dd>
            {formatTokens(data?.usage_7d?.total_tokens)}
            {data?.usage_7d?.measured_run_count ? ` · 측정 run ${data.usage_7d.measured_run_count}/${data.usage_7d.run_count}` : ' · 측정된 run 없음'}
          </dd>
          <dt>7일 비용</dt>
          <dd>{formatCostMicros(data?.usage_7d?.total_cost_micros)}</dd>
        </dl>
      </article>

      <article className="panel settings-card">
        <div className="section-heading">
          <div>
            <h2>운영 작업</h2>
            <p>DB와 run 로그를 관리하는 즉시 실행 작업입니다.</p>
          </div>
        </div>
        <div className="settings-grid">
          <section className="setting-action">
            <div className="setting-copy">
              <strong>DB 백업</strong>
              <p>현재 SQLite DB를 안전하게 복사합니다. 경로를 비우면 서버가 자동으로 `.bak` 파일명을 만듭니다.</p>
            </div>
            <input placeholder="백업 경로 선택 입력 (선택)" value={backupPath} onChange={(e) => setBackupPath(e.target.value)} />
            <button className="button secondary" type="button" onClick={() => backup.mutate()} disabled={backup.isPending}>
              {backup.isPending ? '백업 중' : 'DB 백업'}
            </button>
          </section>

          <section className="setting-action">
            <div className="setting-copy">
              <strong>DB Vacuum</strong>
              <p>삭제 후 남은 SQLite 빈 공간을 회수해 DB 파일을 정리합니다. 데이터는 삭제하지 않습니다.</p>
            </div>
            <button className="button secondary" type="button" onClick={() => vacuum.mutate()} disabled={vacuum.isPending}>
              {vacuum.isPending ? '정리 중' : 'Vacuum'}
            </button>
          </section>

          <section className="setting-action">
            <div className="setting-copy">
              <strong>Run 로그 정리</strong>
              <p>지정 일수보다 오래된 run 로그 파일을 삭제합니다. DB 이슈/댓글 기록은 유지됩니다.</p>
            </div>
            <label className="field-label">
              보존 일수
              <input min="1" type="number" value={cleanupDays} onChange={(e) => setCleanupDays(e.target.value)} />
            </label>
            <button className="button secondary" type="button" onClick={() => cleanup.mutate()} disabled={cleanup.isPending}>
              {cleanup.isPending ? '정리 중' : '로그 정리'}
            </button>
          </section>
        </div>
        {message && <p className="settings-message">{message}</p>}
      </article>

      {isUnauthorizedError(settings.error) && <AuthTokenPanel error={settings.error} />}
      {!isUnauthorizedError(settings.error) && (
        <article className="panel settings-card form-grid">
          <div>
            <h2>API 토큰</h2>
            <p>서버가 token mode로 실행 중일 때 사용하는 브라우저 로컬 설정입니다. 토큰은 서버가 아니라 이 브라우저의 localStorage에 저장됩니다.</p>
          </div>
          <label className="field-label">
            Bearer token
            <input placeholder="Bearer token" type="password" value={token} onChange={(e) => setToken(e.target.value)} />
          </label>
          <div className="settings-grid two">
            <section className="setting-action compact-action">
              <div className="setting-copy">
                <strong>토큰 저장</strong>
                <p>이후 UI API 요청에 Authorization Bearer 헤더를 자동으로 붙입니다.</p>
              </div>
              <button
                className="button"
                type="button"
                onClick={() => {
                  apiAuth.setToken(token);
                  queryClient.invalidateQueries();
                  setMessage('토큰을 브라우저 localStorage에 저장했습니다.');
                }}
              >
                토큰 저장
              </button>
            </section>
            <section className="setting-action compact-action">
              <div className="setting-copy">
                <strong>토큰 삭제</strong>
                <p>브라우저에 저장된 토큰을 제거합니다. token mode 서버에서는 다시 인증이 필요합니다.</p>
              </div>
              <button
                className="button secondary"
                type="button"
                onClick={() => {
                  apiAuth.clearToken();
                  setToken('');
                  queryClient.invalidateQueries();
                  setMessage('토큰을 삭제했습니다.');
                }}
              >
                토큰 삭제
              </button>
            </section>
          </div>
        </article>
      )}
    </section>
  );
}
