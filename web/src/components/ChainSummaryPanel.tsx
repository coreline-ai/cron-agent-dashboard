import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import type { Run, WorkspaceSummary } from '../api/queries';
import { summarizeChains, type ChainSummary } from '../lib/chainSummary';
import { StatusPill } from './StatusPill';

type GuardStatus = 'ok' | 'warn' | 'over';

function classifyGuard(used: number, limit?: number, warnAt = 0.75): GuardStatus {
  if (!limit || limit <= 0) return 'ok';
  const ratio = used / limit;
  if (ratio >= 1) return 'over';
  if (ratio >= warnAt) return 'warn';
  return 'ok';
}

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

export function ChainSummaryPanel({
  runs,
  issueID,
  workspace
}: {
  runs: Run[];
  issueID?: string;
  workspace?: WorkspaceSummary;
}) {
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
            <ChainSummaryRow summary={s} issueID={issueID} workspace={workspace} />
          </li>
        ))}
      </ul>
    </article>
  );
}

function ChainSummaryRow({
  summary,
  issueID,
  workspace
}: {
  summary: ChainSummary;
  issueID?: string;
  workspace?: WorkspaceSummary;
}) {
  const queryClient = useQueryClient();
  const invalidateIssue = () => {
    if (!issueID) return;
    queryClient.invalidateQueries({ queryKey: ['issue', issueID] });
    queryClient.invalidateQueries({ queryKey: ['runs', issueID] });
    queryClient.invalidateQueries({ queryKey: ['comments', issueID] });
  };
  const cancelChain = useMutation({
    mutationFn: () => apiClient.post(`/runs/chain/${summary.chainID}/cancel`, {}),
    onSuccess: invalidateIssue
  });
  const retryChain = useMutation({
    mutationFn: () => apiClient.post(`/runs/chain/${summary.chainID}/retry`, {}),
    onSuccess: invalidateIssue
  });
  const isCancellable =
    summary.lastStatus === 'queued' || summary.lastStatus === 'running';
  const isRetryable = summary.lastStatus === 'failed';
  const depthLimit = workspace?.auto_chain_max_depth;
  const costLimitMicros = workspace?.auto_chain_daily_cost_micros;
  const depthGuard = classifyGuard(summary.maxChainDepth, depthLimit);
  const costGuard = classifyGuard(summary.totalCostMicros, costLimitMicros);
  return (
    <div className="chain-summary-row" data-guard={depthGuard === 'over' || costGuard === 'over' ? 'over' : depthGuard === 'warn' || costGuard === 'warn' ? 'warn' : 'ok'}>
      <div className="chain-summary-row__head">
        <code className="chain-summary-row__id">chain {summary.chainID.slice(0, 8)}</code>
        {summary.lastStatus ? <StatusPill kind="run" status={summary.lastStatus as never} /> : null}
        {summary.lastAgentName ? <span className="muted-copy">@{summary.lastAgentName}</span> : null}
        {isCancellable ? (
          <button
            type="button"
            className="button danger ghost chain-summary-row__cancel"
            onClick={() => {
              if (window.confirm('이 체인의 모든 queued/running run을 취소할까요?')) {
                cancelChain.mutate();
              }
            }}
            disabled={cancelChain.isPending}
          >
            {cancelChain.isPending ? '취소 중' : '체인 취소'}
          </button>
        ) : null}
        {isRetryable ? (
          <button
            type="button"
            className="button secondary ghost chain-summary-row__retry"
            onClick={() => retryChain.mutate()}
            disabled={retryChain.isPending}
          >
            {retryChain.isPending ? '재시작 중' : '체인 재시작'}
          </button>
        ) : null}
      </div>
      <dl className="chain-summary-row__stats">
        <div>
          <dt>run</dt>
          <dd>{summary.totalRuns}</dd>
        </div>
        <div data-guard={depthGuard}>
          <dt>max depth{depthLimit ? ` / ${depthLimit}` : ''}</dt>
          <dd>
            {summary.maxChainDepth}
            {depthLimit && depthGuard !== 'ok' ? (
              <span className="muted-copy"> ({depthGuard === 'over' ? '한도 초과' : '한도 근접'})</span>
            ) : null}
          </dd>
        </div>
        <div>
          <dt>token (in / out)</dt>
          <dd>
            {formatTokens(summary.totalInputTokens)} / {formatTokens(summary.totalOutputTokens)}
          </dd>
        </div>
        <div data-guard={costGuard}>
          <dt>cost{costLimitMicros ? ` / ${formatCostMicros(costLimitMicros)}/일` : ''}</dt>
          <dd>
            {formatCostMicros(summary.totalCostMicros)}
            {costLimitMicros && costGuard !== 'ok' ? (
              <span className="muted-copy"> ({costGuard === 'over' ? '한도 초과' : '한도 근접'})</span>
            ) : null}
          </dd>
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
