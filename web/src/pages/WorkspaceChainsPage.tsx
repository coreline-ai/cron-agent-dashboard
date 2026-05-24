import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { type Run, useWorkspaceQuery } from '../api/queries';
import { ChainSummaryPanel } from '../components/ChainSummaryPanel';
import { PageHeader } from '../components/PageHeader';

// Track G of dev-plan/implement_20260522_220446.md.
//
// WorkspaceChainsPage surfaces every recent run across the workspace
// grouped into chain summaries. We deliberately reuse ChainSummaryPanel /
// summarizeChains so the depth-cost guard rendering, the cancel button,
// and the retry button stay in lock-step with the per-issue view.
export function WorkspaceChainsPage() {
  const { slug } = useParams<{ slug: string }>();
  const queryClient = useQueryClient();
  const workspace = useWorkspaceQuery(slug);
  const runs = useQuery({
    enabled: Boolean(slug),
    queryKey: ['workspace-runs', slug],
    refetchInterval: 8_000,
    queryFn: async () => (await apiClient.get<{ runs: Run[] | null }>(`/workspaces/${slug}/runs?limit=500`)).runs ?? []
  });

  // Workspace SSE wake stream: invalidate the chain list whenever any
  // issue in the workspace fires a run_event. The API helper uses fetch
  // streaming so token mode can attach Authorization; the 8s poll stays
  // as a fallback.
  useEffect(() => {
    if (!slug) return undefined;
    return apiClient.streamSSE(`/workspaces/${slug}/runs/stream`, {
      reconnect: true,
      onEvent: (event) => {
        if (event === 'wake') {
          queryClient.invalidateQueries({ queryKey: ['workspace-runs', slug] });
        }
      }
    });
  }, [slug, queryClient]);

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="체인 대시보드"
        description="워크스페이스 전체 chain의 진행 상태, guard 사용량, 마지막 활동을 보드와 같은 밀도로 확인합니다."
        actions={
          <button className="button micro secondary" type="button" onClick={() => runs.refetch()} disabled={runs.isFetching}>
            {runs.isFetching ? '새로고침 중' : '새로고침'}
          </button>
        }
      />
      <article className="board-toolbar panel">
        <div className="toolbar-main">
          <div>
            <h2>체인 실행 현황</h2>
            <p>
              최근 500개 run 기준 · chain 취소/재시작 액션은 이슈 상세와 동일한 API를 사용합니다.
            </p>
          </div>
          <span className="badge">{runs.data?.length ?? 0} runs</span>
        </div>
      </article>
      {runs.isLoading ? (
        <article className="panel empty-state compact">
          <p className="muted-copy">불러오는 중…</p>
        </article>
      ) : (runs.data ?? []).length === 0 ? (
        <article className="panel empty-state compact">
          <h2>실행 이력 없음</h2>
          <p>아직 이 워크스페이스에 run이 없습니다.</p>
        </article>
      ) : (
        <ChainSummaryPanel runs={runs.data ?? []} workspace={workspace.data} />
      )}
    </section>
  );
}
