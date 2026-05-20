import type { Run, RunStatus } from '../api/queries';

type Props = {
  runs: Run[];
  loading?: boolean;
};

type Stage = {
  index: number;
  run: Run;
  state: 'done' | 'running' | 'queued' | 'failed' | 'cancelled' | 'retrying';
  durationMs?: number;
};

function durationOf(run: Run): number | undefined {
  const start = run.started_at || run.claimed_at || run.enqueued_at;
  const end = run.finished_at;
  if (!start || !end) return undefined;
  const ms = new Date(end).getTime() - new Date(start).getTime();
  return Number.isFinite(ms) && ms >= 0 ? ms : undefined;
}

function formatDuration(ms?: number) {
  if (ms === undefined) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m ${s % 60}s`;
}

function stateOf(run: Run): Stage['state'] {
  const s = run.status as RunStatus;
  if (s === 'done') return 'done';
  if (s === 'failed') return 'failed';
  if (s === 'cancelled') return 'cancelled';
  if (s === 'running') return 'running';
  if (s === 'queued') {
    return run.next_retry_at ? 'retrying' : 'queued';
  }
  return 'queued';
}

const stateIcon: Record<Stage['state'], string> = {
  done: '✅',
  running: '🔄',
  queued: '⏸',
  failed: '❌',
  cancelled: '🛑',
  retrying: '⏰'
};

const stateLabel: Record<Stage['state'], string> = {
  done: '완료',
  running: '실행 중',
  queued: '대기',
  failed: '실패',
  cancelled: '취소',
  retrying: '재시도 대기'
};

/**
 * Horizontal stage cards for the issue's run history.
 * Renders runs in chronological order (oldest left → newest right).
 * Each card shows agent name + status icon + duration so operators can see
 * the entire pipeline (Sales → Planner → Designer → ...) without scrolling.
 */
export function AgentPipelineStrip({ runs, loading }: Props) {
  if (loading && !runs.length) {
    return (
      <article className="pipeline-strip empty" role="status">
        <p className="muted-copy">파이프라인 로딩 중…</p>
      </article>
    );
  }
  if (!runs.length) {
    return (
      <article className="pipeline-strip empty">
        <p className="muted-copy">아직 실행된 단계가 없습니다.</p>
      </article>
    );
  }

  const sorted = [...runs].sort((a, b) => {
    const ka = a.enqueued_at || a.claimed_at || a.started_at || '';
    const kb = b.enqueued_at || b.claimed_at || b.started_at || '';
    return ka.localeCompare(kb);
  });
  const stages: Stage[] = sorted.map((run, index) => ({
    index,
    run,
    state: stateOf(run),
    durationMs: durationOf(run)
  }));
  const totalMs = stages.reduce((acc, s) => acc + (s.durationMs ?? 0), 0);
  const doneCount = stages.filter((s) => s.state === 'done').length;
  const failedCount = stages.filter((s) => s.state === 'failed').length;
  const runningCount = stages.filter((s) => s.state === 'running').length;

  return (
    <article className="pipeline-strip" aria-label="에이전트 파이프라인">
      <header className="pipeline-strip-header">
        <strong>파이프라인</strong>
        <small>
          {stages.length}단계 · 완료 {doneCount} · 진행 {runningCount} · 실패 {failedCount} · 누적 {formatDuration(totalMs)}
        </small>
      </header>
      <ol className="pipeline-stages" role="list">
        {stages.map((stage) => {
          const r = stage.run;
          return (
            <li key={r.id} className={`pipeline-stage stage-${stage.state}`} role="listitem">
              <div className="stage-line" aria-hidden="true">
                <span className="stage-icon">{stateIcon[stage.state]}</span>
                {stage.index < stages.length - 1 ? <span className="stage-connector" /> : null}
              </div>
              <div className="stage-card">
                <header className="stage-head">
                  <span className="stage-num">{stage.index + 1}</span>
                  <strong>@{r.agent_name || '에이전트'}</strong>
                </header>
                <small className="stage-state-label">{stateLabel[stage.state]}</small>
                {stage.durationMs !== undefined ? <small className="stage-duration">{formatDuration(stage.durationMs)}</small> : null}
                {r.max_attempts && r.max_attempts > 1 && r.attempt ? (
                  <small className="stage-attempt">
                    attempt {r.attempt}/{r.max_attempts}
                  </small>
                ) : null}
                {r.failure_kind && stage.state === 'failed' ? <small className="stage-failure">{r.failure_kind}</small> : null}
              </div>
            </li>
          );
        })}
      </ol>
    </article>
  );
}
