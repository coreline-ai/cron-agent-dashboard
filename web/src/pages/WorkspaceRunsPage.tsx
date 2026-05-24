import { useEffect, useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { type Run, useAgentsQuery } from '../api/queries';
import { PageHeader } from '../components/PageHeader';
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
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="런 피드"
        description="워크스페이스 전체 run을 최신순으로 확인하고, 실패 원인·agent·chain을 빠르게 좁혀봅니다."
        actions={
          <button className="button micro secondary" type="button" onClick={() => runs.refetch()} disabled={runs.isFetching}>
            {runs.isFetching ? '새로고침 중' : '새로고침'}
          </button>
        }
      />
      <article className="board-toolbar panel">
        <div className="toolbar-main">
          <div>
            <h2>전체 run</h2>
            <p>최근 500건 중 {filtered.length}건 표시 · 클릭하면 해당 이슈 상세로 이동합니다.</p>
          </div>
          <span className="badge">{runs.data?.length ?? 0} total</span>
        </div>
        <div className="toolbar-controls runs-feed-filters">
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
        <article className="panel empty-state compact">
          <p className="muted-copy">불러오는 중…</p>
        </article>
      ) : filtered.length === 0 ? (
        <article className="panel empty-state compact">
          <h2>조건에 해당하는 run 없음</h2>
          <p>필터를 줄이거나 검색어를 비워 다시 확인하세요.</p>
        </article>
      ) : (
        <article className="panel table-panel">
          <div className="section-heading compact">
            <h2>run · {filtered.length}건</h2>
            <span className="muted-copy">최근 500건 중 필터 결과</span>
          </div>
          <div className="runs-feed-table">
            <div className="runs-feed-row runs-feed-head">
              <span>Run</span>
              <span>상태</span>
              <span>Agent</span>
              <span>Chain</span>
              <span>대기 시각</span>
              <span>오류</span>
            </div>
            {filtered.map((r) => (
              <RunRow key={r.id} run={r} slug={slug ?? ''} />
            ))}
          </div>
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
    <Link to={target} className="runs-feed-row">
      <span>
        <code className="runs-feed-row__id">run {run.id.slice(0, 8)}</code>
      </span>
      <span><StatusPill kind="run" status={run.status as never} /></span>
      <span className="runs-feed-cell-muted">{run.agent_name ? `@${run.agent_name}` : '—'}</span>
      <span className="runs-feed-cell-muted">{run.chain_id ? `chain ${run.chain_id.slice(0, 8)}` : '—'}</span>
      <span className="runs-feed-cell-muted">{run.enqueued_at?.slice(0, 19).replace('T', ' ') ?? '—'}</span>
      <span className="runs-feed-row__err" title={run.error_message || undefined}>
        {run.error_message ? run.error_message.replace(/\s+/g, ' ').slice(0, 120) : '—'}
      </span>
    </Link>
  );
}
