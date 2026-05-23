import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiAuth, apiClient } from '../api/client';
import { AuthTokenPanel, isUnauthorizedError } from '../components/AuthTokenPanel';
import { PageHeader } from '../components/PageHeader';
import { WorkspaceWebhookSection } from '../components/WorkspaceWebhookSection';
import { WorkspaceSummary, useSettingsQuery, useUsageSummaryQuery, useWorkspacesQuery } from '../api/queries';

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

function formatDurationSeconds(value?: number) {
  const seconds = value ?? 0;
  if (seconds <= 0) {
    return 'OFF';
  }
  const days = Math.floor(seconds / 86_400);
  if (days >= 1 && seconds % 86_400 === 0) {
    return `${days}일`;
  }
  const hours = Math.floor(seconds / 3_600);
  if (hours >= 1 && seconds % 3_600 === 0) {
    return `${hours}시간`;
  }
  return `${seconds}초`;
}

function formatWorktreeUsage(m?: { worktree_bytes?: string; worktree_dir_count?: string; worktree_pruned_last_pass?: string; worktree_measured_at?: string }) {
  const at = m?.worktree_measured_at?.trim();
  if (!at) {
    return '아직 측정되지 않음';
  }
  const dirs = Number(m?.worktree_dir_count ?? 0);
  if (dirs === 0) {
    return '디렉터리 없음 (모두 정리됨)';
  }
  const bytes = Number(m?.worktree_bytes ?? 0);
  const pruned = Number(m?.worktree_pruned_last_pass ?? 0);
  const prunedSuffix = pruned > 0 ? ` · 직전 GC ${pruned}개 정리` : '';
  return `${dirs}개 디렉터리 · ${formatBytes(bytes)}${prunedSuffix}`;
}

function formatLastLogCleanup(m?: { last_log_cleanup_at?: string; last_log_cleanup_files?: string; last_log_cleanup_bytes?: string }) {
  const at = m?.last_log_cleanup_at?.trim();
  if (!at) {
    return '아직 실행되지 않음';
  }
  const stamp = at.slice(0, 19).replace('T', ' ');
  const files = Number(m?.last_log_cleanup_files ?? 0);
  const bytes = Number(m?.last_log_cleanup_bytes ?? 0);
  return `${stamp} UTC — 파일 ${files}개 / ${formatBytes(bytes)}`;
}

function WorkspaceTimeoutRow({ workspace }: { workspace: WorkspaceSummary }) {
  const queryClient = useQueryClient();
  const [timeoutSeconds, setTimeoutSeconds] = useState(String(workspace.default_timeout_seconds ?? 600));
  const [autoChainEnabled, setAutoChainEnabled] = useState(Boolean(workspace.auto_chain_enabled));
  const [autoChainMaxDepth, setAutoChainMaxDepth] = useState(String(workspace.auto_chain_max_depth ?? 5));
  const [autoChainDailyRunLimit, setAutoChainDailyRunLimit] = useState(String(workspace.auto_chain_daily_run_limit ?? 20));
  const [autoChainDailyCostDollars, setAutoChainDailyCostDollars] = useState(String(((workspace.auto_chain_daily_cost_micros ?? 0) / 1_000_000).toFixed(4)));
  const [autoChainDryRun, setAutoChainDryRun] = useState(Boolean(workspace.auto_chain_dry_run));
  const [autoCloseOnRunDone, setAutoCloseOnRunDone] = useState(Boolean(workspace.auto_close_on_run_done));
  const [perRunWorktree, setPerRunWorktree] = useState(Boolean(workspace.per_run_worktree));
  const save = useMutation({
    mutationFn: () =>
      apiClient.put(`/workspaces/${workspace.slug}`, {
        name: workspace.name,
        description: workspace.description ?? '',
        working_dir: workspace.working_dir ?? '',
        output_dir: workspace.output_dir ?? '',
        default_timeout_seconds: Number(timeoutSeconds) || 600,
        auto_chain_enabled: autoChainEnabled,
        auto_chain_max_depth: Number(autoChainMaxDepth) || 5,
        auto_chain_daily_run_limit: Number(autoChainDailyRunLimit) || 0,
        auto_chain_daily_cost_micros: Math.max(0, Math.round((Number(autoChainDailyCostDollars) || 0) * 1_000_000)),
        auto_chain_dry_run: autoChainDryRun,
        auto_close_on_run_done: autoCloseOnRunDone,
        per_run_worktree: perRunWorktree
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['workspace', workspace.slug] });
    }
  });

  return (
    <section className="setting-action workspace-setting-action">
      <div className="setting-copy">
        <strong>{workspace.name}</strong>
        <p>{workspace.slug} · 기본 run timeout을 초 단위로 설정합니다.</p>
      </div>
      <label className="field-label">
        기본 timeout (초)
        <input min="1" max="86400" type="number" value={timeoutSeconds} onChange={(e) => setTimeoutSeconds(e.target.value)} />
      </label>
      <label className="checkbox-row">
        <input type="checkbox" checked={autoCloseOnRunDone} onChange={(e) => setAutoCloseOnRunDone(e.target.checked)} />
        run 성공 시 이슈 자동 완료 처리
      </label>
      <label className="checkbox-row">
        <input type="checkbox" checked={perRunWorktree} onChange={(e) => setPerRunWorktree(e.target.checked)} />
        run마다 별도 worktree 사용 (워크스페이스 동시 실행 허용)
      </label>
      <label className="checkbox-row">
        <input type="checkbox" checked={autoChainEnabled} onChange={(e) => setAutoChainEnabled(e.target.checked)} />
        agent 결과 @mention 자동 체이닝 허용
      </label>
      <div className="settings-grid two">
        <label className="field-label">
          최대 chain depth
          <input min="1" max="20" type="number" value={autoChainMaxDepth} onChange={(e) => setAutoChainMaxDepth(e.target.value)} />
        </label>
        <label className="field-label">
          24시간 자동 chain run 제한
          <input min="0" type="number" value={autoChainDailyRunLimit} onChange={(e) => setAutoChainDailyRunLimit(e.target.value)} />
        </label>
        <label className="field-label">
          24시간 자동 chain 비용 제한($, 0=무제한)
          <input min="0" step="0.0001" type="number" value={autoChainDailyCostDollars} onChange={(e) => setAutoChainDailyCostDollars(e.target.value)} />
        </label>
        <label className="checkbox-row">
          <input type="checkbox" checked={autoChainDryRun} onChange={(e) => setAutoChainDryRun(e.target.checked)} />
          dry-run: 감지하되 실행 등록 안 함
        </label>
      </div>
      <button className="button secondary" type="button" onClick={() => save.mutate()} disabled={save.isPending}>
        {save.isPending ? '저장 중' : '저장'}
      </button>
      {save.isError && <p className="error-text">저장 실패: {save.error instanceof Error ? save.error.message : '알 수 없는 오류'}</p>}
      <WorkspaceWebhookSection workspace={workspace} />
    </section>
  );
}

export function SettingsPage() {
  const settings = useSettingsQuery();
  const workspaces = useWorkspacesQuery();
  const usage30d = useUsageSummaryQuery(30);
  const queryClient = useQueryClient();
  const data = settings.data;
  const [message, setMessage] = useState('');
  const [token, setToken] = useState(() => apiAuth.getToken());
  const [tokenSessionOnly, setTokenSessionOnly] = useState(() => apiAuth.getTokenStorageMode() === 'session');
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
              ? data.available_runtimes.map((runtime) => `${runtime.name}${runtime.version ? ` (${runtime.version})` : ''}${runtime.warning ? ` · ${runtime.warning}` : ''}`).join(', ')
              : 'PATH에서 탐지된 런타임 없음'}
          </dd>
          <dt>7일 토큰</dt>
          <dd>
            {formatTokens(data?.usage_7d?.total_tokens)}
            {data?.usage_7d?.measured_run_count ? ` · 측정 run ${data.usage_7d.measured_run_count}/${data.usage_7d.run_count}` : ' · 측정된 run 없음'}
          </dd>
          <dt>7일 비용</dt>
          <dd>{formatCostMicros(data?.usage_7d?.total_cost_micros)}</dd>
          <dt>자동 백업</dt>
          <dd>
            {data?.maintenance?.auto_backup ? `ON · 최근 ${data.maintenance.auto_backup_keep}개 보존` : 'OFF'}
          </dd>
          <dt>자동 로그 정리</dt>
          <dd>{data?.maintenance?.auto_cleanup_log_days ? `${data.maintenance.auto_cleanup_log_days}일 초과 run 로그 자동 삭제` : 'OFF'}</dd>
          <dt>마지막 로그 정리</dt>
          <dd>{formatLastLogCleanup(data?.maintenance)}</dd>
          <dt>worktree 디스크</dt>
          <dd>{formatWorktreeUsage(data?.maintenance)}</dd>
          <dt>worktree GC</dt>
          <dd>{data?.maintenance?.worktree_gc_after_seconds ? `${formatDurationSeconds(data.maintenance.worktree_gc_after_seconds)} 이상 미사용 디렉터리 자동 정리` : 'OFF'}</dd>
          <dt>마이그레이션 실패</dt>
          <dd>{data?.migration_fail_count ? `${data.migration_fail_count}건 이력 있음 · 로그/DB 확인 권장` : '없음'}</dd>
        </dl>
      </article>

      <article className="panel settings-card read-only-note">
        <div>
          <h2>사용량 대시보드</h2>
          <p>런타임이 보고한 token/cost metric 기준입니다. 값이 0이면 CLI가 사용량을 출력하지 않았거나 아직 측정 run이 없는 상태입니다.</p>
        </div>
        <div className="settings-grid two">
          <UsageCard title="최근 7일" usage={data?.usage_7d} />
          <UsageCard title="최근 30일" usage={usage30d.data} loading={usage30d.isLoading} />
        </div>
      </article>

      {Boolean(data?.migration_failures?.length) && (
        <article className="panel settings-card read-only-note">
          <div>
            <h2>최근 마이그레이션 실패 이력</h2>
            <p>서버는 실패한 migration을 트랜잭션 rollback 후 기록합니다. 같은 실패가 반복되면 DB 백업 후 로그를 확인하세요.</p>
          </div>
          <div className="settings-grid">
            {data?.migration_failures?.map((failure) => (
              <section className="setting-action" key={failure.id}>
                <div className="setting-copy">
                  <strong>{failure.version} · {failure.name}</strong>
                  <p>{failure.failed_at}</p>
                </div>
                <code className="settings-code">{failure.error}</code>
              </section>
            ))}
          </div>
        </article>
      )}

      <article className="panel settings-card">
        <div className="section-heading">
          <div>
            <h2>워크스페이스 실행 기본값</h2>
            <p>에이전트별 override가 없을 때 적용되는 기본 run timeout입니다.</p>
          </div>
        </div>
        <div className="settings-grid two">
          {workspaces.data?.length ? workspaces.data.map((workspace) => <WorkspaceTimeoutRow key={workspace.id} workspace={workspace} />) : <p>워크스페이스가 아직 없습니다.</p>}
        </div>
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
            <p>서버가 token mode로 실행 중일 때 사용하는 브라우저 로컬 설정입니다. 토큰은 서버가 아니라 이 브라우저의 localStorage 또는 sessionStorage에 저장됩니다.</p>
          </div>
          <label className="field-label">
            Bearer token
            <input placeholder="Bearer token" type="password" value={token} onChange={(e) => setToken(e.target.value)} />
          </label>
          <label className="checkbox-row">
            <input type="checkbox" checked={tokenSessionOnly} onChange={(e) => setTokenSessionOnly(e.target.checked)} />
            이번 세션만 저장(sessionStorage)
          </label>
          <div className="settings-grid two">
            <section className="setting-action compact-action">
              <div className="setting-copy">
                <strong>토큰 저장</strong>
                <p>저장 위치를 선택하면 이후 UI API 요청에 Authorization Bearer 헤더를 자동으로 붙입니다.</p>
              </div>
              <button
                className="button"
                type="button"
                onClick={() => {
                  apiAuth.setToken(token, { sessionOnly: tokenSessionOnly });
                  queryClient.invalidateQueries();
                  setMessage(tokenSessionOnly ? '토큰을 이번 브라우저 세션(sessionStorage)에 저장했습니다.' : '토큰을 브라우저 localStorage에 저장했습니다.');
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
                  setTokenSessionOnly(false);
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


function UsageCard({ title, usage, loading = false }: { title: string; usage?: { run_count: number; measured_run_count: number; total_tokens: number; total_cost_micros: number; input_tokens: number; output_tokens: number }; loading?: boolean }) {
  return (
    <section className="setting-action">
      <div className="setting-copy">
        <strong>{title}</strong>
        <p>{loading ? '집계 중' : `측정 run ${usage?.measured_run_count ?? 0}/${usage?.run_count ?? 0}`}</p>
      </div>
      <dl className="detail-list settings-detail-list">
        <dt>Input</dt>
        <dd>{formatTokens(usage?.input_tokens)}</dd>
        <dt>Output</dt>
        <dd>{formatTokens(usage?.output_tokens)}</dd>
        <dt>Total</dt>
        <dd>{formatTokens(usage?.total_tokens)}</dd>
        <dt>Cost</dt>
        <dd>{formatCostMicros(usage?.total_cost_micros)}</dd>
      </dl>
    </section>
  );
}
