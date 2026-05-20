import { Link } from 'react-router-dom';
import { useIssuesQuery } from '../api/queries';

type Props = {
  slug: string | undefined;
};

function formatElapsed(iso?: string) {
  if (!iso) return '';
  const start = new Date(iso).getTime();
  if (Number.isNaN(start)) return '';
  const elapsed = Math.max(0, Date.now() - start);
  const s = Math.floor(elapsed / 1000);
  if (s < 60) return `${s}초`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}분 ${s % 60}초`;
  const h = Math.floor(m / 60);
  return `${h}시간 ${m % 60}분`;
}

/**
 * Sidebar live activity card. Shows the single most relevant active run in
 * the current workspace so operators always know which agent is busy.
 *
 * Workspaces are serialized on working_dir, so at most one run is "running"
 * at a time; queued runs surface as the "next" hint.
 */
export function WorkspaceActivityStrip({ slug }: Props) {
  const runningQuery = useIssuesQuery(slug, { execution: 'running', limit: 1 });
  const queuedQuery = useIssuesQuery(slug, { execution: 'queued', limit: 1 });
  // "검토 대기" — 모든 run이 done인데 issue는 아직 open인 상태. 보드에서 사용자 확인을 기다리는 케이스.
  const reviewPendingQuery = useIssuesQuery(slug, { status: 'open', execution: 'done', limit: 5 });

  if (!slug) {
    return null;
  }

  const running = runningQuery.data?.[0];
  const queued = queuedQuery.data?.[0];
  const reviewPending = (reviewPendingQuery.data ?? []).filter((i) => (i.comment_count ?? 0) > 0);

  if (!running && !queued && reviewPending.length === 0) {
    return (
      <div className="activity-strip idle" aria-live="polite">
        <span className="activity-dot idle" aria-hidden="true" />
        <div className="activity-copy">
          <strong>Idle</strong>
          <small>실행 중 작업 없음</small>
        </div>
      </div>
    );
  }

  if (!running && !queued && reviewPending.length > 0) {
    const first = reviewPending[0];
    return (
      <Link className="activity-strip review-pending" to={`/w/${slug}/issues/${first.identifier}`} aria-live="polite">
        <span className="activity-dot review" aria-hidden="true" />
        <div className="activity-copy">
          <strong>✓ 검토 대기 {reviewPending.length > 1 ? `${reviewPending.length}건` : ''}</strong>
          <small>
            {first.identifier} · @{first.last_run_agent_name || '에이전트'} 결과 확인 필요
          </small>
        </div>
      </Link>
    );
  }

  if (running) {
    return (
      <Link className="activity-strip running" to={`/w/${slug}/issues/${running.identifier}`} aria-live="polite">
        <span className="activity-dot running" aria-hidden="true" />
        <div className="activity-copy">
          <strong>🔄 @{running.last_run_agent_name || running.assignee_agent_name || '에이전트'} 실행 중</strong>
          <small>
            {running.identifier} · {formatElapsed(running.updated_at)} 경과
          </small>
        </div>
        {queued ? <span className="activity-next">다음 {queued.identifier}</span> : null}
      </Link>
    );
  }

  return (
    <Link className="activity-strip queued" to={`/w/${slug}/issues/${queued!.identifier}`} aria-live="polite">
      <span className="activity-dot queued" aria-hidden="true" />
      <div className="activity-copy">
        <strong>⏳ 대기 중</strong>
        <small>
          {queued!.identifier} · @{queued!.assignee_agent_name || '에이전트'} 대기
        </small>
      </div>
    </Link>
  );
}
