import { useState } from 'react';
import type { Issue, Run } from '../api/queries';

type Props = {
  issue?: Issue;
  runs: Run[];
  onMarkDone: () => void;
  onCancelIssue: () => void;
  onRetryStage?: (agentName: string, stageIndex: number) => void;
  disabled?: boolean;
};

function shortReason(run: Run): string {
  const r = run.terminal_reason || '';
  const fk = run.failure_kind || '';
  return [r, fk && fk !== r ? fk : ''].filter(Boolean).join(' · ');
}

function shortError(msg?: string): string {
  if (!msg) return '';
  const trimmed = msg.replace(/<br\s*\/?>(?:\n)?/gi, '\n').replace(/&[a-z]+;/gi, ' ').trim();
  return trimmed.length > 220 ? trimmed.slice(0, 220) + '…' : trimmed;
}

function findRecovery(failedRun: Run, all: Run[]): Run | undefined {
  // Same agent later run that succeeded — this is how operators tell that a
  // transient failure was already recovered (RFP-1: Architect/QA failed, then
  // a later run by the same agent succeeded after working_dir fix).
  const failedTime = failedRun.enqueued_at || failedRun.started_at || '';
  return all.find(
    (r) => r.agent_id === failedRun.agent_id && r.status === 'done' && (r.enqueued_at || r.started_at || '') > failedTime
  );
}

function stageIndexOf(run: Run, all: Run[]): number {
  const sorted = [...all].sort((a, b) => (a.enqueued_at || '').localeCompare(b.enqueued_at || ''));
  return sorted.findIndex((r) => r.id === run.id) + 1;
}

/**
 * Calls the operator to action when every agent run finished but the issue is
 * still open. Surfaces the gap between `run.status` (agent did the work) and
 * `issue.status` (human said "we're done with this") that the design
 * principle keeps intentionally separate.
 *
 * Renders nothing unless the conditions match (open issue + done execution + at least one finished run).
 */
export function ReviewBanner({ issue, runs, onMarkDone, onCancelIssue, onRetryStage, disabled }: Props) {
  const [expandFailures, setExpandFailures] = useState(false);

  if (!issue) return null;
  if (issue.status !== 'open') return null;
  if (issue.execution_status !== 'done') return null;
  if (runs.length === 0) return null;

  const finished = runs.filter((r) => r.status === 'done' || r.status === 'failed' || r.status === 'cancelled');
  if (finished.length === 0) return null;
  const lastDone = [...runs].reverse().find((r) => r.status === 'done');
  const lastAgent = lastDone?.agent_name;
  const doneRuns = runs.filter((r) => r.status === 'done');
  const failedRuns = runs.filter((r) => r.status === 'failed');
  const recoveredCount = failedRuns.filter((r) => findRecovery(r, runs)).length;
  const totalDone = doneRuns.length;
  const totalFailed = failedRuns.length;

  return (
    <section className="review-banner" role="status" aria-live="polite">
      <div className="review-banner-icon" aria-hidden="true">
        ✓
      </div>
      <div className="review-banner-copy">
        <strong>에이전트 작업이 끝났습니다. 사용자 확인이 필요합니다.</strong>
        <p>
          마지막 단계 <strong>@{lastAgent || '에이전트'}</strong>의 결과가 댓글로 도착했습니다. 총 {runs.length}단계 (완료 {totalDone}
          {totalFailed > 0 ? (
            <>
              {' · '}
              <button
                type="button"
                className="review-banner-failure-toggle"
                onClick={() => setExpandFailures((v) => !v)}
                aria-expanded={expandFailures}
              >
                실패 {totalFailed}
                {recoveredCount === totalFailed ? ' (모두 복구됨)' : recoveredCount > 0 ? ` (${recoveredCount}건 복구됨)` : ''}
                <span className="review-banner-caret" aria-hidden="true">
                  {expandFailures ? '▾' : '▸'}
                </span>
              </button>
            </>
          ) : null}
          ) — 결과를 검토한 뒤 정식 마감하거나, 멘션 댓글로 추가 보강을 요청하세요.
        </p>
        {expandFailures && totalFailed > 0 ? (
          <ul className="review-banner-failure-list" aria-label="실패한 단계 상세">
            {failedRuns.map((fr) => {
              const recovery = findRecovery(fr, runs);
              const stage = stageIndexOf(fr, runs);
              return (
                <li key={fr.id} className="review-banner-failure-item">
                  <header>
                    <span className="failure-icon" aria-hidden="true">
                      ❌
                    </span>
                    <strong>
                      Stage {stage} · @{fr.agent_name || '에이전트'}
                    </strong>
                    <small className="failure-meta">
                      {shortReason(fr)} · attempt {fr.attempt ?? 1}/{fr.max_attempts ?? 1}
                      {fr.finished_at ? ` · ${fr.finished_at.slice(11, 19)}` : ''}
                    </small>
                  </header>
                  {fr.error_message ? <pre className="failure-error">{shortError(fr.error_message)}</pre> : null}
                  {recovery ? (
                    <p className="failure-recovery">
                      ✅ 이후 <strong>Stage {stageIndexOf(recovery, runs)}</strong>에서 같은 에이전트(@{recovery.agent_name})가
                      <strong> 성공</strong>으로 복구. (재실행)
                    </p>
                  ) : (
                    <p className="failure-recovery unrecovered">⚠ 아직 같은 에이전트의 후속 성공 run이 없습니다. 재시도 또는 멘션 보강이 필요할 수 있습니다.</p>
                  )}
                  <div className="failure-actions">
                    {onRetryStage ? (
                      <button
                        type="button"
                        className={recovery ? 'button micro secondary' : 'button micro'}
                        onClick={() => onRetryStage(fr.agent_name || '에이전트', stage)}
                        disabled={disabled}
                        title={recovery ? '이미 복구됐지만 같은 단계를 한 번 더 돌리고 싶을 때' : '이 단계를 다시 실행 요청 (댓글 멘션으로 자동 작성)'}
                      >
                        {recovery ? '↻ 다시 시도' : '🔄 재시도 요청'}
                      </button>
                    ) : null}
                    <a className="failure-link" href={`#run-${fr.id}`}>
                      ▸ 이벤트 타임라인에서 상세 보기
                    </a>
                  </div>
                </li>
              );
            })}
          </ul>
        ) : null}
        <ul className="review-banner-hints">
          <li>
            정식 마감하면 보드의 <strong>완료</strong> 컬럼으로 이동하고, 이 워크스페이스의 후속 작업이 별도 이슈로 시작됩니다.
          </li>
          <li>
            추가 보강이 필요하면 아래 댓글 폼에 <code>@AgentName 추가 요청 내용</code> 형식으로 멘션해 새 단계를 추가하세요.
          </li>
        </ul>
      </div>
      <div className="review-banner-actions">
        <button className="button" type="button" onClick={onMarkDone} disabled={disabled}>
          ✅ 완료 처리
        </button>
        <button className="button secondary" type="button" onClick={onCancelIssue} disabled={disabled}>
          ✕ 이슈 취소
        </button>
      </div>
    </section>
  );
}
