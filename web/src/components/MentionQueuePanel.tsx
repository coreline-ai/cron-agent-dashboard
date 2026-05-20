import type { Run } from '../api/queries';

type Props = {
  runs: Run[];
};

/**
 * Lists queued runs in chronological order so the operator can see which
 * agent is up next on the issue. Workspaces are serialized on working_dir,
 * so queued runs execute one-by-one — this panel makes that order explicit
 * before the user posts another mention comment.
 */
export function MentionQueuePanel({ runs }: Props) {
  const queued = runs
    .filter((r) => r.status === 'queued')
    .sort((a, b) => (a.enqueued_at || '').localeCompare(b.enqueued_at || ''));

  if (queued.length === 0) return null;

  return (
    <section className="mention-queue-panel" aria-label="대기 중인 멘션 큐">
      <header className="mention-queue-head">
        <span className="mention-queue-icon" aria-hidden="true">
          📨
        </span>
        <strong>다음 차례 ({queued.length})</strong>
        <small>워크스페이스 동시 실행 1건 제약 — 순차로 실행됩니다.</small>
      </header>
      <ol className="mention-queue-list">
        {queued.map((r, i) => (
          <li key={r.id} className="mention-queue-item">
            <span className="queue-num">{i + 1}</span>
            <div className="queue-copy">
              <strong>@{r.agent_name || '에이전트'}</strong>
              <small>
                {r.next_retry_at ? '재시도 대기' : r.trigger_type === 'mention' ? '멘션으로 큐잉' : r.trigger_type}
                {r.chain_depth ? ` · chain depth ${r.chain_depth}` : ''}
              </small>
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}
