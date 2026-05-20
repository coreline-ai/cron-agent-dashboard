import type { Run } from '../api/queries';

type Props = {
  run?: Run;
  onCancel?: () => void;
  cancelDisabled?: boolean;
};

function secondsBetween(from?: string, to?: string): number | undefined {
  if (!from) return undefined;
  const start = new Date(from).getTime();
  const end = to ? new Date(to).getTime() : Date.now();
  if (Number.isNaN(start) || Number.isNaN(end)) return undefined;
  return Math.max(0, Math.round((end - start) / 1000));
}

function formatElapsed(s?: number) {
  if (s === undefined) return '—';
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${s % 60}s`;
}

function heartbeatHealth(heartbeatAt?: string): { label: string; tone: 'healthy' | 'stale' | 'unknown' } {
  if (!heartbeatAt) return { label: '—', tone: 'unknown' };
  const ago = secondsBetween(heartbeatAt);
  if (ago === undefined) return { label: '—', tone: 'unknown' };
  if (ago <= 30) return { label: `${ago}초 전 (healthy)`, tone: 'healthy' };
  if (ago <= 120) return { label: `${ago}초 전`, tone: 'healthy' };
  return { label: `${Math.floor(ago / 60)}분 ${ago % 60}초 전 (stale)`, tone: 'stale' };
}

/**
 * Live heartbeat card for the currently active (queued/running) run. Surfaces
 * the heartbeat freshness, attempt count, token/cost so operators can tell
 * at a glance whether the run is healthy or hung.
 *
 * Renders nothing when no run is queued/running.
 */
export function LiveRunCard({ run, onCancel, cancelDisabled }: Props) {
  if (!run) return null;
  if (run.status !== 'running' && run.status !== 'queued') return null;

  const elapsed = secondsBetween(run.started_at || run.claimed_at || run.enqueued_at);
  const hb = heartbeatHealth(run.heartbeat_at);
  const tokens = (run.input_tokens ?? 0) + (run.output_tokens ?? 0);
  const cost = run.total_cost_micros ? `$${(run.total_cost_micros / 1_000_000).toFixed(4)}` : '$0.0000';
  const attempt = run.attempt && run.max_attempts ? `${run.attempt}/${run.max_attempts}` : '—';

  return (
    <section className={`live-run-card live-run-${run.status}`} aria-live="polite">
      <header className="live-run-head">
        <span className="live-run-dot" aria-hidden="true" />
        <strong>{run.status === 'running' ? '🔄 LIVE' : '⏳ QUEUED'}</strong>
        <span className="live-run-agent">@{run.agent_name || '에이전트'}</span>
      </header>
      <dl className="live-run-stats">
        <div>
          <dt>경과</dt>
          <dd>{formatElapsed(elapsed)}</dd>
        </div>
        <div>
          <dt>heartbeat</dt>
          <dd className={`hb-${hb.tone}`}>{hb.label}</dd>
        </div>
        <div>
          <dt>시도</dt>
          <dd>{attempt}</dd>
        </div>
        <div>
          <dt>토큰</dt>
          <dd>{tokens.toLocaleString()}</dd>
        </div>
        <div>
          <dt>비용</dt>
          <dd>{cost}</dd>
        </div>
        {run.model_resolved ? (
          <div>
            <dt>모델</dt>
            <dd className="muted">{run.model_resolved}</dd>
          </div>
        ) : null}
      </dl>
      {onCancel ? (
        <button className="button micro danger live-run-cancel" type="button" onClick={onCancel} disabled={cancelDisabled}>
          🛑 실행 취소
        </button>
      ) : null}
    </section>
  );
}
