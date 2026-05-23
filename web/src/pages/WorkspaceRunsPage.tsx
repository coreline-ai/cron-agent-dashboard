import { useEffect, useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { type Run, useAgentsQuery } from '../api/queries';
import { StatusPill } from '../components/StatusPill';

// Track B of dev-plan/implement_20260523_201535.md.
//
// WorkspaceRunsPage surfaces every run across the workspace as a flat
// newest-first list with status / agent / search filters. The chain
// dashboard (/w/:slug/chains) groups by chain_id and exposes
// cancel/retry; this view is read-only and exists for run-level
// inspection (debugging a specific failure, reviewing what an agent
// worked on this week, etc.).
export function WorkspaceRunsPage() {
  const { slug } = useParams<{ slug: string }>();
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [agentFilter, setAgentFilter] = useState<string>('');
  const [search, setSearch] = useState<string>('');

  const runs = useQuery({
    enabled: Boolean(slug),
    queryKey: ['workspace-runs-feed', slug],
    refetchInterval: 8_000,
    queryFn: async () =>
      (await apiClient.get<{ runs: Run[] | null }>(`/workspaces/${slug}/runs?limit=500`)).runs ?? []
  });
  const agents = useAgentsQuery(slug);

  // Subscribe to the workspace SSE wake stream so any issue's run_event
  // in this workspace refreshes the feed without waiting for the 8s
  // polling tick. The API helper uses fetch streaming so token mode can
  // attach Authorization.
  useEffect(() => {
    if (!slug) return undefined;
    return apiClient.streamSSE(`/workspaces/${slug}/runs/stream`, {
      reconnect: true,
      onEvent: (event) => {
        if (event === 'wake') {
          queryClient.invalidateQueries({ queryKey: ['workspace-runs-feed', slug] });
        }
      }
    });
  }, [slug, queryClient]);

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return (runs.data ?? []).filter((r) => {
      if (statusFilter && r.status !== statusFilter) return false;
      if (agentFilter && r.agent_id !== agentFilter) return false;
      if (!needle) return true;
      const haystack = `${r.id} ${r.chain_id ?? ''} ${r.agent_name ?? ''} ${r.error_message ?? ''}`.toLowerCase();
      return haystack.includes(needle);
    });
  }, [runs.data, statusFilter, agentFilter, search]);

  return (
    <section className="content-grid">
      <header className="content-header">
        <div>
          <h1>Run feed</h1>
          <p className="muted-copy">
            워크스페이스 전체 run을 한 줄씩 newest-first로 봅니다. 클릭하면 해당 이슈 상세로 이동합니다.
            취소/재시작 액션은 이슈 상세 또는 체인 대시보드에서 진행하세요.
          </p>
        </div>
      </header>
      <article className="panel">
        <div className="form-grid runs-feed-filters">
          <label className="field-label">
            상태
            <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
              <option value="">전체</option>
              <option value="queued">queued</option>
              <option value="running">running</option>
              <option value="done">done</option>
              <option value="failed">failed</option>
              <option value="cancelled">cancelled</option>
            </select>
          </label>
          <label className="field-label">
            에이전트
            <select value={agentFilter} onChange={(e) => setAgentFilter(e.target.value)}>
              <option value="">전체</option>
              {(agents.data ?? []).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </label>
          <label className="field-label">
            검색 (chain_id / agent / error_message)
            <input
              type="search"
              placeholder="chain-abc12345 / Lead / timeout..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </label>
        </div>
      </article>
      {runs.isLoading ? (
        <p className="muted-copy">불러오는 중…</p>
      ) : filtered.length === 0 ? (
        <p className="muted-copy">조건에 해당하는 run이 없습니다.</p>
      ) : (
        <article className="panel">
          <div className="section-heading compact">
            <h2>run · {filtered.length}건</h2>
            <span className="muted-copy">최근 500건 중 필터 결과</span>
          </div>
          <ul className="runs-feed-list">
            {filtered.map((r) => (
              <RunRow key={r.id} run={r} slug={slug ?? ''} />
            ))}
          </ul>
        </article>
      )}
    </section>
  );
}

function RunRow({ run, slug }: { run: Run; slug: string }) {
  // run.issue_id is always present in the workspace feed payload (joined
  // through issue). The detail page uses issue identifier; the API does
  // not return identifier on the run row, so we link via issue id and
  // let the IssueDetailPage route resolve it.
  const target = run.issue_id ? `/w/${slug}/issues/${run.issue_id}` : '#';
  return (
    <li className="runs-feed-row">
      <Link to={target} className="runs-feed-row__link">
        <code className="runs-feed-row__id">run {run.id.slice(0, 8)}</code>
        <StatusPill kind="run" status={run.status as never} />
        {run.agent_name ? <span className="muted-copy">@{run.agent_name}</span> : null}
        {run.chain_id ? <span className="muted-copy">chain {run.chain_id.slice(0, 8)}</span> : null}
        <span className="runs-feed-row__time">
          {run.enqueued_at?.slice(0, 19).replace('T', ' ') ?? '—'}
        </span>
        {run.error_message ? <span className="runs-feed-row__err">{run.error_message.slice(0, 80)}</span> : null}
      </Link>
    </li>
  );
}
