import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { type Run, useWorkspaceQuery } from '../api/queries';
import { ChainSummaryPanel } from '../components/ChainSummaryPanel';

// Track G of dev-plan/implement_20260522_220446.md.
//
// WorkspaceChainsPage surfaces every recent run across the workspace
// grouped into chain summaries. We deliberately reuse ChainSummaryPanel /
// summarizeChains so the depth-cost guard rendering, the cancel button,
// and the retry button stay in lock-step with the per-issue view.
export function WorkspaceChainsPage() {
  const { slug } = useParams<{ slug: string }>();
  const workspace = useWorkspaceQuery(slug);
  const runs = useQuery({
    enabled: Boolean(slug),
    queryKey: ['workspace-runs', slug],
    refetchInterval: 8_000,
    queryFn: async () => (await apiClient.get<{ runs: Run[] | null }>(`/workspaces/${slug}/runs?limit=500`)).runs ?? []
  });

  return (
    <section className="content-grid">
      <header className="content-header">
        <div>
          <h1>체인 대시보드</h1>
          <p className="muted-copy">
            워크스페이스 전체에서 진행 중이거나 마무리된 chain을 한 번에 봅니다. 각 row의 체인 취소 / 재시작 버튼은
            이슈 상세 페이지의 동작과 동일합니다.
          </p>
        </div>
      </header>
      {runs.isLoading ? (
        <p className="muted-copy">불러오는 중…</p>
      ) : (runs.data ?? []).length === 0 ? (
        <p className="muted-copy">아직 실행 이력이 없습니다.</p>
      ) : (
        <ChainSummaryPanel runs={runs.data ?? []} workspace={workspace.data} />
      )}
    </section>
  );
}
