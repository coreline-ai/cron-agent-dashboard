import type { Issue, IssueStatus, Run } from '../api/queries';
import { DateTimeText } from './DateTimeText';
import { StatusPill } from './StatusPill';

type IssueSummaryRailProps = {
  issue?: Issue;
  runs: Run[];
  busy: boolean;
  actionPending?: boolean;
  onRerun: () => void;
  onCancelRun: () => void;
  onMarkDone: () => void;
  onCancelIssue: () => void;
  onReopen: () => void;
};

export function IssueSummaryRail({
  issue,
  runs,
  busy,
  actionPending = false,
  onRerun,
  onCancelRun,
  onMarkDone,
  onCancelIssue,
  onReopen
}: IssueSummaryRailProps) {
  const latestRun = runs.length > 0 ? runs[runs.length - 1] : undefined;
  const latestProblemRun = [...runs].reverse().find((run) => run.status === 'failed' || run.status === 'cancelled');
  const canOperate = Boolean(issue) && !actionPending;
  const usage = summarizeRunUsage(runs);
  const issueStatus = issue?.status ?? 'open';
  const executionStatus = issue?.execution_status ?? 'idle';

  return (
    <aside className="issue-summary-rail panel" aria-label="이슈 작업 콘솔">
      <div className="section-heading compact">
        <h2>작업 콘솔</h2>
        <span className="badge">live</span>
      </div>

      <div className="summary-status-grid">
        <div>
          <span>이슈 상태</span>
          <StatusPill kind="issue" status={issueStatus} />
        </div>
        <div>
          <span>실행 상태</span>
          <StatusPill kind="execution" status={executionStatus} pulse={busy} />
        </div>
        <div>
          <span>담당</span>
          <strong>@{issue?.last_run_agent_name || issue?.assignee_agent_name || '메인 에이전트'}</strong>
        </div>
        <div>
          <span>댓글</span>
          <strong>{issue?.comment_count ?? 0}개</strong>
        </div>
        <div>
          <span>토큰</span>
          <strong>{formatTokens(usage.totalTokens)}</strong>
        </div>
        <div>
          <span>비용</span>
          <strong>{formatCostMicros(usage.totalCostMicros)}</strong>
        </div>
      </div>

      <div className="summary-actions">
        <button className="button" type="button" onClick={onRerun} disabled={!canOperate || busy}>
          재실행
        </button>
        {busy ? (
          <button className="button danger" type="button" onClick={onCancelRun} disabled={!canOperate}>
            {executionStatus === 'queued' ? '대기 취소' : '실행 취소'}
          </button>
        ) : (
          <button className="button danger" type="button" onClick={onCancelIssue} disabled={!canOperate || issueStatus === 'cancelled'}>
            이슈 취소
          </button>
        )}
        <button className="button secondary" type="button" onClick={onMarkDone} disabled={!canOperate || busy || issueStatus === 'done'}>
          완료 처리
        </button>
        <button className="button secondary" type="button" onClick={onReopen} disabled={!canOperate || issueStatus === 'open'}>
          다시 열기
        </button>
      </div>

      <div className="summary-block">
        <h3>최근 실행</h3>
        {latestRun ? (
          <div className="latest-run-card">
            <StatusPill kind="run" status={latestRun.status} pulse={latestRun.status === 'running'} />
            <strong>@{latestRun.agent_name || '-'}</strong>
            <span>{triggerLabel(latestRun.trigger_type)}</span>
            <small>
              <DateTimeText value={latestRun.finished_at || latestRun.started_at || latestRun.enqueued_at} mode="both" />
            </small>
            {runHasUsage(latestRun) ? (
              <small>{formatTokens((latestRun.input_tokens ?? 0) + (latestRun.output_tokens ?? 0))} · {formatCostMicros(latestRun.total_cost_micros)}</small>
            ) : null}
          </div>
        ) : (
          <p className="muted-copy">아직 실행 이력이 없습니다.</p>
        )}
      </div>

      {latestProblemRun ? (
        <div className="summary-block warning">
          <h3>최근 문제</h3>
          <p>
            {latestProblemRun.terminal_reason || latestProblemRun.failure_kind || latestProblemRun.cancel_reason || latestProblemRun.status}
          </p>
          {latestProblemRun.error_message ? <pre>{normalizeRunMessage(latestProblemRun.error_message)}</pre> : null}
        </div>
      ) : null}
    </aside>
  );
}

function triggerLabel(trigger: string) {
  const labels: Record<string, string> = {
    issue_created: '이슈 생성',
    rerun: '재실행',
    mention: '멘션',
    autopilot: '오토파일럿'
  };
  return labels[trigger] ?? trigger;
}

function normalizeRunMessage(value?: string) {
  if (!value) {
    return '';
  }
  return value
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&#39;/gi, "'")
    .replace(/&amp;/gi, '&')
    .replace(/^stderr:\s*/i, 'stderr:\n')
    .trim();
}


export function summarizeRunUsage(runs: Run[]) {
  return runs.reduce(
    (acc, run) => {
      const input = run.input_tokens ?? 0;
      const output = run.output_tokens ?? 0;
      acc.inputTokens += input;
      acc.outputTokens += output;
      acc.totalTokens += input + output;
      acc.totalCostMicros += run.total_cost_micros ?? 0;
      if (input > 0 || output > 0 || (run.total_cost_micros ?? 0) > 0) {
        acc.measuredRuns += 1;
      }
      return acc;
    },
    { inputTokens: 0, outputTokens: 0, totalTokens: 0, totalCostMicros: 0, measuredRuns: 0 }
  );
}

export function formatTokens(value?: number) {
  const n = Math.max(0, value ?? 0);
  if (n >= 1_000_000) {
    return `${(n / 1_000_000).toFixed(2)}M`;
  }
  if (n >= 1_000) {
    return `${(n / 1_000).toFixed(1)}k`;
  }
  return String(n);
}

export function formatCostMicros(value?: number) {
  return `$${((Math.max(0, value ?? 0)) / 1_000_000).toFixed(4)}`;
}

function runHasUsage(run: Run) {
  return (run.input_tokens ?? 0) > 0 || (run.output_tokens ?? 0) > 0 || (run.total_cost_micros ?? 0) > 0;
}
