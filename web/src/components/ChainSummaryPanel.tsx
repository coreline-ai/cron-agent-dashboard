import type { Run } from '../api/queries';
import { summarizeChains, type ChainSummary } from '../lib/chainSummary';
import { StatusPill } from './StatusPill';

function formatCostMicros(value: number): string {
  if (!value) return '$0.0000';
  return `$${(value / 1_000_000).toFixed(4)}`;
}

function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
  return String(value);
}

function shortDate(value?: string): string {
  if (!value) return '—';
  return value.slice(0, 19).replace('T', ' ');
}

export function ChainSummaryPanel({ runs }: { runs: Run[] }) {
  if (!runs.length) {
    return null;
  }
  const summaries = summarizeChains(runs);
  if (summaries.length === 0) {
    return null;
  }
  return (
    <article className="panel">
      <div className="section-heading compact">
        <h2>체인 요약</h2>
        <span className="badge">{summaries.length}</span>
      </div>
      <p className="muted-copy">
        같은 <code>chain_id</code>의 run을 묶어 시작/마지막 활동, 최대 chain_depth, 누적 token/cost, 마지막 상태를 보여줍니다.
        main agent 재진입은 depth 카운트에서 제외되므로 max_depth가 작아도 hub-PM 워크플로우가 끝까지 진행됩니다.
      </p>
      <ul className="chain-summary-list">
        {summaries.map((s) => (
          <li key={s.chainID}>
            <ChainSummaryRow summary={s} />
          </li>
        ))}
      </ul>
    </article>
  );
}

function ChainSummaryRow({ summary }: { summary: ChainSummary }) {
  return (
    <div className="chain-summary-row">
      <div className="chain-summary-row__head">
        <code className="chain-summary-row__id">chain {summary.chainID.slice(0, 8)}</code>
        {summary.lastStatus ? <StatusPill kind="run" status={summary.lastStatus as never} /> : null}
        {summary.lastAgentName ? <span className="muted-copy">@{summary.lastAgentName}</span> : null}
      </div>
      <dl className="chain-summary-row__stats">
        <div>
          <dt>run</dt>
          <dd>{summary.totalRuns}</dd>
        </div>
        <div>
          <dt>max depth</dt>
          <dd>{summary.maxChainDepth}</dd>
        </div>
        <div>
          <dt>token (in / out)</dt>
          <dd>
            {formatTokens(summary.totalInputTokens)} / {formatTokens(summary.totalOutputTokens)}
          </dd>
        </div>
        <div>
          <dt>cost</dt>
          <dd>{formatCostMicros(summary.totalCostMicros)}</dd>
        </div>
        <div>
          <dt>시작</dt>
          <dd>{shortDate(summary.firstEnqueuedAt)}</dd>
        </div>
        <div>
          <dt>마지막 활동</dt>
          <dd>{shortDate(summary.lastActivityAt)}</dd>
        </div>
      </dl>
    </div>
  );
}
