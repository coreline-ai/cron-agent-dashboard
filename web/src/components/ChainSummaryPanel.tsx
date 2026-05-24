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
    <article className="panel chain-summary-panel">
      <div className="section-heading compact">
        <h2>체인 요약</h2>
        <span className="badge">{summaries.length}</span>
      </div>
      <p className="muted-copy">
        같은 <code>chain_id</code>의 run을 묶어 시작/마지막 활동, 최대 chain_depth, 누적 token/cost, 마지막 상태를 보여줍니다.
        main agent 재진입은 depth 카운트에서 제외되므로 max_depth가 작아도 hub-PM 워크플로우가 끝까지 진행됩니다.
      </p>
      <div className="chain-summary-table">
        <div className="chain-summary-head">
          <span>Chain</span>
          <span>상태</span>
          <span>Agent</span>
          <span>Run</span>
          <span>Depth</span>
          <span>Token</span>
          <span>Cost</span>
          <span>시작</span>
          <span>마지막 활동</span>
          <span>Action</span>
        </div>
        {summaries.map((s) => (
          <ChainSummaryRow key={s.chainID} summary={s} issueID={issueID} workspace={workspace} />
        ))}
      </div>
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
  const invalidateAfterAction = () => {
    if (issueID) {
      queryClient.invalidateQueries({ queryKey: ['issue', issueID] });
      queryClient.invalidateQueries({ queryKey: ['runs', issueID] });
      queryClient.invalidateQueries({ queryKey: ['comments', issueID] });
    }
    if (workspace?.slug) {
      queryClient.invalidateQueries({ queryKey: ['workspace-runs', workspace.slug] });
      queryClient.invalidateQueries({ queryKey: ['workspace-runs-feed', workspace.slug] });
    }
  };
  const cancelChain = useMutation({
    mutationFn: () => apiClient.post(`/runs/chain/${summary.chainID}/cancel`, {}),
    onSuccess: invalidateAfterAction
  });
  const retryChain = useMutation({
    mutationFn: () => apiClient.post(`/runs/chain/${summary.chainID}/retry`, {}),
    onSuccess: invalidateAfterAction
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
      <span><code className="chain-summary-row__id">chain {summary.chainID.slice(0, 8)}</code></span>
      <span>{summary.lastStatus ? <StatusPill kind="run" status={summary.lastStatus as never} /> : '—'}</span>
      <span className="chain-summary-cell-muted">{summary.lastAgentName ? `@${summary.lastAgentName}` : '—'}</span>
      <span>{summary.totalRuns}</span>
      <span data-guard={depthGuard}>
        {summary.maxChainDepth}{depthLimit ? ` / ${depthLimit}` : ''}
      </span>
      <span>{formatTokens(summary.totalInputTokens)} / {formatTokens(summary.totalOutputTokens)}</span>
      <span data-guard={costGuard}>
        {formatCostMicros(summary.totalCostMicros)}
        {costLimitMicros && costGuard !== 'ok' ? ` · ${costGuard === 'over' ? '초과' : '근접'}` : ''}
      </span>
      <span className="chain-summary-cell-muted">{shortDate(summary.firstEnqueuedAt)}</span>
      <span className="chain-summary-cell-muted">{shortDate(summary.lastActivityAt)}</span>
      <span className="chain-summary-row__actions">
        {isCancellable ? (
          <button
            type="button"
            aria-label="체인 취소"
            className="button danger ghost chain-summary-row__cancel"
            onClick={() => {
              if (window.confirm('이 체인의 모든 queued/running run을 취소할까요?')) {
                cancelChain.mutate();
              }
            }}
            disabled={cancelChain.isPending}
          >
            {cancelChain.isPending ? '취소 중' : '취소'}
          </button>
        ) : null}
        {isRetryable ? (
          <button
            type="button"
            aria-label="체인 재시작"
            className="button secondary ghost chain-summary-row__retry"
            onClick={() => retryChain.mutate()}
            disabled={retryChain.isPending}
          >
            {retryChain.isPending ? '재시작 중' : '재시작'}
          </button>
        ) : null}
        {!isCancellable && !isRetryable ? <span className="chain-summary-cell-muted">—</span> : null}
      </span>
    </div>
  );
}
