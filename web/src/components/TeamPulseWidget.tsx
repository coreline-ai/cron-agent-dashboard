import { Link } from 'react-router-dom';
import { useAgentActivityQuery, type AgentActivity } from '../api/queries';

type Props = {
  slug: string | undefined;
};

function dotClass(status?: string): string {
  switch (status) {
    case 'running':
      return 'running';
    case 'queued':
      return 'queued';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    case 'done':
      return 'done';
    default:
      return 'idle';
  }
}

function statusLabel(status?: string): string {
  switch (status) {
    case 'running':
      return '실행 중';
    case 'queued':
      return '대기';
    case 'done':
      return '완료';
    case 'failed':
      return '실패';
    case 'cancelled':
      return '취소';
    default:
      return '대기 없음';
  }
}

function relativeTime(iso?: string): string {
  if (!iso) return '';
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return '';
  const s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (s < 60) return `${s}초 전`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}분 전`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}시간 전`;
  return `${Math.floor(h / 24)}일 전`;
}

function sortPriority(a: AgentActivity, b: AgentActivity): number {
  // running > queued > failed > done > idle, then by recency
  const order = (s?: string) => {
    if (s === 'running') return 0;
    if (s === 'queued') return 1;
    if (s === 'failed') return 2;
    if (s === 'done') return 3;
    return 4;
  };
  const ord = order(a.latest_run_status) - order(b.latest_run_status);
  if (ord !== 0) return ord;
  return (b.latest_run_enqueued_at || '').localeCompare(a.latest_run_enqueued_at || '');
}

/**
 * Compact at-a-glance roster: each agent in the workspace with its most recent
 * run status. Renders on the HomePage right sidebar so operators can see "who
 * is busy" without opening individual issues.
 */
export function TeamPulseWidget({ slug }: Props) {
  const activityQuery = useAgentActivityQuery(slug);
  const items = (activityQuery.data ?? []).slice().sort(sortPriority);

  if (!slug) return null;

  return (
    <article className="panel team-pulse-card">
      <div className="section-heading compact">
        <div>
          <h2>👥 팀 활동</h2>
          <p>{activityQuery.isLoading ? '집계 중…' : `${items.length}명 · 5초 polling`}</p>
        </div>
      </div>
      {!items.length ? (
        <p className="muted-copy">에이전트가 없습니다.</p>
      ) : (
        <ul className="team-pulse-list">
          {items.map((a) => (
            <li key={a.agent_id} className={`team-pulse-row pulse-${dotClass(a.latest_run_status)}`}>
              <span className={`team-pulse-dot pulse-${dotClass(a.latest_run_status)}`} aria-hidden="true" />
              <span className="team-pulse-name">
                <strong>@{a.agent_name}</strong>
                {a.is_main ? <span className="badge muted">main</span> : null}
              </span>
              <span className="team-pulse-state">
                {statusLabel(a.latest_run_status)}
                {a.latest_issue_identifier ? (
                  <Link className="team-pulse-issue" to={`/w/${slug}/issues/${a.latest_issue_identifier}`}>
                    {a.latest_issue_identifier}
                  </Link>
                ) : null}
              </span>
              <span className="team-pulse-time">{relativeTime(a.latest_run_finished_at || a.latest_run_enqueued_at)}</span>
            </li>
          ))}
        </ul>
      )}
    </article>
  );
}
