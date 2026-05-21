import { FormEvent, Suspense, lazy, useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { useAgentsQuery, useCommentsQuery, useRunEventsQuery, useRunsQuery, useSubIssuesQuery, useWorkspaceIssueQuery } from '../api/queries';
import type { Comment, IssueStatus, Run, RunEvent } from '../api/queries';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DateTimeText } from '../components/DateTimeText';
import { IssueSummaryRail } from '../components/IssueSummaryRail';
import { MarkdownText } from '../components/MarkdownText';
import { MentionAutocomplete } from '../components/MentionAutocomplete';
import { MutationErrorAlert } from '../components/MutationErrorAlert';
import { AgentPipelineStrip } from '../components/AgentPipelineStrip';
import { LiveRunCard } from '../components/LiveRunCard';
import { MentionQueuePanel } from '../components/MentionQueuePanel';
import { PageHeader } from '../components/PageHeader';
import { ReviewBanner } from '../components/ReviewBanner';
import { StatusPill } from '../components/StatusPill';
import { useToast } from '../components/ToastProvider';
import {
  getCancelReasonLabel,
  getFailureKindLabel,
  getRunEventLabel,
  getTerminalReasonLabel,
  getTriggerLabel
} from '../lib/runLabels';

type ConfirmAction = 'rerun' | 'cancelRun' | 'markDone' | 'cancelIssue' | 'reopen';

const IssueFlowGraph = lazy(() => import('../components/IssueFlowGraph'));

type CommentResponse = {
  comment: Comment;
  mention_warnings?: string[];
  dispatched_run?: Run;
};

export function IssueDetailPage() {
  const { identifier, slug } = useParams();
  const issue = useWorkspaceIssueQuery(slug, identifier);
  const comments = useCommentsQuery(issue.data?.id, issue.data?.execution_status);
  const runs = useRunsQuery(issue.data?.id, issue.data?.execution_status);
  const subIssues = useSubIssuesQuery(issue.data?.id);
  const agents = useAgentsQuery(slug);
  const queryClient = useQueryClient();
  const toast = useToast();
  const [content, setContent] = useState('');
  const [subIssueForm, setSubIssueForm] = useState({ title: '', body: '', assignee_agent_id: '' });
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const runList = useMemo(() => runs.data ?? [], [runs.data]);
  const busy = issue.data?.execution_status === 'queued' || issue.data?.execution_status === 'running';

  const invalidateIssue = () => {
    queryClient.invalidateQueries({ queryKey: ['issue', slug, identifier] });
    queryClient.invalidateQueries({ queryKey: ['comments', issue.data?.id] });
    queryClient.invalidateQueries({ queryKey: ['runs', issue.data?.id] });
    queryClient.invalidateQueries({ queryKey: ['issues', slug] });
    queryClient.invalidateQueries({ queryKey: ['subissues', issue.data?.id] });
    for (const run of runList) {
      queryClient.invalidateQueries({ queryKey: ['run-events', run.id] });
    }
  };

  const addComment = useMutation({
    mutationFn: () => apiClient.post<CommentResponse>(`/issues/${issue.data?.id}/comments`, { content }),
    onSuccess: (data) => {
      setContent('');
      invalidateIssue();
      if (data.dispatched_run) {
        toast.success('멘션 실행을 큐에 등록했습니다.', { description: `run ${data.dispatched_run.id.slice(0, 8)}` });
      } else {
        toast.success('댓글을 등록했습니다.');
      }
      for (const warning of data.mention_warnings ?? []) {
        toast.warning('멘션 경고', { description: warning });
      }
    },
    onError: (error) => toast.error('댓글 등록 실패', { description: errorMessage(error) })
  });

  const rerun = useMutation({
    mutationFn: () => apiClient.post(`/issues/${issue.data?.id}/rerun`),
    onSuccess: () => {
      invalidateIssue();
      toast.success('재실행을 큐에 등록했습니다.');
    },
    onError: (error) => toast.error('재실행 실패', { description: errorMessage(error) }),
    onSettled: () => setConfirmAction(null)
  });
  const cancelRun = useMutation({
    mutationFn: () => apiClient.post(`/issues/${issue.data?.id}/cancel`),
    onSuccess: () => {
      invalidateIssue();
      toast.success('실행 취소를 요청했습니다.');
    },
    onError: (error) => toast.error('실행 취소 실패', { description: errorMessage(error) }),
    onSettled: () => setConfirmAction(null)
  });
  const updateStatus = useMutation({
    mutationFn: (status: IssueStatus) => apiClient.put(`/issues/${issue.data?.id}`, { status }),
    onSuccess: (_, status) => {
      invalidateIssue();
      toast.success(statusToast(status));
    },
    onError: (error) => toast.error('상태 변경 실패', { description: errorMessage(error) }),
    onSettled: () => setConfirmAction(null)
  });

  const createSubIssue = useMutation({
    mutationFn: () => apiClient.post(`/issues/${issue.data?.id}/subissues`, subIssueForm),
    onSuccess: () => {
      setSubIssueForm({ title: '', body: '', assignee_agent_id: '' });
      invalidateIssue();
      toast.success('하위 이슈를 생성했습니다.');
    },
    onError: (error) => toast.error('하위 이슈 생성 실패', { description: errorMessage(error) })
  });

  const actionPending = rerun.isPending || cancelRun.isPending || updateStatus.isPending;
  const refreshPending = issue.isFetching || comments.isFetching || runs.isFetching || agents.isFetching;
  const refreshIssue = () => {
    const tasks: Array<Promise<unknown>> = [issue.refetch(), agents.refetch(), subIssues.refetch()];
    if (issue.data?.id) {
      tasks.push(comments.refetch(), runs.refetch());
      for (const run of runList) {
        queryClient.invalidateQueries({ queryKey: ['run-events', run.id] });
      }
    }
    void Promise.all(tasks);
  };

  const onCommentSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!issue.data || !content.trim()) {
      return;
    }
    addComment.mutate();
  };

  const onSubIssueSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!issue.data || !subIssueForm.title.trim()) {
      return;
    }
    createSubIssue.mutate();
  };

  const runConfirmedAction = () => {
    if (!issue.data || !confirmAction) {
      return;
    }
    if (confirmAction === 'rerun') {
      rerun.mutate();
      return;
    }
    if (confirmAction === 'cancelRun') {
      cancelRun.mutate();
      return;
    }
    if (confirmAction === 'markDone') {
      updateStatus.mutate('done');
      return;
    }
    if (confirmAction === 'cancelIssue') {
      updateStatus.mutate('cancelled');
      return;
    }
    updateStatus.mutate('open');
  };

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`${slug} / ${identifier}`}
        title={issue.data?.title ?? '이슈 상세'}
        description={
          issue.data
            ? `${issue.data.status} · ${issue.data.execution_status} · 댓글 ${issue.data.comment_count}개`
            : '실행 로그, 댓글, 멘션 위임 흐름을 안전한 텍스트 렌더링으로 표시합니다.'
        }
        actions={
          <button className="button micro secondary" type="button" onClick={refreshIssue} disabled={refreshPending}>
            {refreshPending ? '새로고침 중' : '새로고침'}
          </button>
        }
      />

      {issue.isError ? <MutationErrorAlert error={issue.error} title="이슈 로드 실패" /> : null}

      <ReviewBanner
        issue={issue.data}
        runs={runList}
        onMarkDone={() => setConfirmAction('markDone')}
        onCancelIssue={() => setConfirmAction('cancelIssue')}
        onRetryStage={(agentName, stage) => {
          const template = `@${agentName} 이전 시도(Stage ${stage})가 실패했습니다. 다시 진행해주세요.\n\n(필요한 추가 컨텍스트나 변경된 가정이 있으면 여기에 적은 후 [댓글 등록]을 누르세요)`;
          setContent(template);
          // scroll the comment textarea into view so the operator can review + submit
          setTimeout(() => {
            const form = document.querySelector('.comment-form');
            if (form) form.scrollIntoView({ behavior: 'smooth', block: 'center' });
            const textarea = document.querySelector<HTMLTextAreaElement>('.comment-form textarea');
            if (textarea) {
              textarea.focus();
              textarea.setSelectionRange(textarea.value.length, textarea.value.length);
            }
          }, 50);
        }}
        disabled={actionPending}
      />

      <LiveRunCard run={runList.find((r) => r.status === 'running' || r.status === 'queued')} onCancel={() => setConfirmAction('cancelRun')} cancelDisabled={actionPending} />

      <AgentPipelineStrip runs={runList} loading={runs.isLoading} />

      <div className="issue-detail-layout">
        <div className="issue-detail-main">
          <article className="panel">
            <div className="section-heading compact">
              <h2>본문</h2>
              {issue.data ? <StatusPill kind="issue" status={issue.data.status} /> : null}
            </div>
            <MarkdownText value={issue.data?.body || '(본문 없음)'} />
          </article>

          {issue.data ? (
            <article className="panel">
              <div className="section-heading compact">
                <h2>흐름 그래프</h2>
                <span className="badge">lineage</span>
              </div>
              <Suspense fallback={<p className="muted-copy">그래프를 불러오는 중입니다.</p>}>
                <IssueFlowGraph
                  issue={issue.data}
                  subIssues={subIssues.data ?? []}
                  runs={runList}
                  mainAgentName={(agents.data ?? []).find((agent) => agent.is_main)?.name}
                />
              </Suspense>
            </article>
          ) : null}

          <article className="panel">
            <div className="section-heading compact">
              <h2>하위 이슈</h2>
              <span className="badge">{subIssues.data?.length ?? 0}</span>
            </div>
            <div className="subissue-list">
              {subIssues.data?.map((subIssue) => (
                <a className="subissue-card" href={`/w/${slug}/issues/${subIssue.identifier}`} key={subIssue.id}>
                  <strong>{subIssue.identifier}</strong>
                  <span>{subIssue.title}</span>
                  <StatusPill kind="issue" status={subIssue.status} />
                </a>
              ))}
            </div>
            {!subIssues.isLoading && !subIssues.data?.length ? <p className="muted-copy">아직 하위 이슈가 없습니다.</p> : null}
            <form className="form-grid subissue-form" onSubmit={onSubIssueSubmit}>
              <input placeholder="하위 이슈 제목" value={subIssueForm.title} onChange={(e) => setSubIssueForm({ ...subIssueForm, title: e.target.value })} />
              <select value={subIssueForm.assignee_agent_id} onChange={(e) => setSubIssueForm({ ...subIssueForm, assignee_agent_id: e.target.value })}>
                <option value="">메인 에이전트 자동 선택</option>
                {(agents.data ?? []).map((agent) => (
                  <option key={agent.id} value={agent.id}>@{agent.name} · {agent.runtime}</option>
                ))}
              </select>
              <textarea placeholder="하위 이슈 본문" value={subIssueForm.body} onChange={(e) => setSubIssueForm({ ...subIssueForm, body: e.target.value })} />
              <button className="button secondary" type="submit" disabled={!issue.data || createSubIssue.isPending || !subIssueForm.title.trim()}>
                {createSubIssue.isPending ? '생성 중' : '하위 이슈 생성'}
              </button>
              {createSubIssue.isError ? <MutationErrorAlert error={createSubIssue.error} title="하위 이슈 생성 실패" /> : null}
            </form>
          </article>

          <article className="panel">
            <div className="section-heading compact">
              <h2>댓글 스레드</h2>
              <span className="badge">{comments.data?.length ?? 0}</span>
            </div>
            {comments.data?.map((comment) => (
              <div className="comment-block" key={comment.id}>
                <div className="comment-meta-row">
                  <strong>{comment.author_agent_name || comment.author_type}</strong>
                  <DateTimeText value={comment.created_at} mode="both" />
                  {comment.truncated ? <span className="badge warning">truncated</span> : null}
                </div>
                <MarkdownText value={comment.content} />
                {comment.log_url && (
                  <a className="inline-link" href={comment.log_url}>
                    로그 보기
                  </a>
                )}
              </div>
            ))}
            {!comments.isLoading && !comments.data?.length && <p>아직 댓글이 없습니다.</p>}
            <MentionQueuePanel runs={runList} />
            <form className="form-grid comment-form" onSubmit={onCommentSubmit}>
              <MentionAutocomplete
                placeholder="@AgentName 멘션으로 위임할 수 있습니다."
                value={content}
                agents={agents.data ?? []}
                onChange={setContent}
                required
              />
              <button className="button" type="submit" disabled={!issue.data || addComment.isPending || !content.trim()}>
                {addComment.isPending ? '등록 중' : '댓글 등록'}
              </button>
              {addComment.isError ? <MutationErrorAlert error={addComment.error} title="댓글 등록 실패" /> : null}
            </form>
          </article>

          <article className="panel">
            <div className="section-heading compact">
              <h2>Run 이력</h2>
              <span className="badge">{runList.length}</span>
            </div>
            <div className="run-history-list">
              {runList.map((run) => (
                <RunHistoryCard key={run.id} run={run} />
              ))}
            </div>
            {!runs.isLoading && !runList.length && <p>아직 실행 이력이 없습니다.</p>}
          </article>
        </div>

        <IssueSummaryRail
          issue={issue.data}
          runs={runList}
          busy={Boolean(busy)}
          actionPending={actionPending}
          onRerun={() => setConfirmAction('rerun')}
          onCancelRun={() => setConfirmAction('cancelRun')}
          onMarkDone={() => setConfirmAction('markDone')}
          onCancelIssue={() => setConfirmAction('cancelIssue')}
          onReopen={() => setConfirmAction('reopen')}
        />
      </div>

      <ConfirmDialog
        open={Boolean(confirmAction)}
        title={confirmTitle(confirmAction)}
        description={confirmDescription(confirmAction, issue.data?.title)}
        confirmLabel={confirmLabel(confirmAction)}
        tone={confirmAction === 'cancelIssue' || confirmAction === 'cancelRun' ? 'danger' : 'default'}
        pending={actionPending}
        onConfirm={runConfirmedAction}
        onClose={() => setConfirmAction(null)}
      />
    </section>
  );
}

function RunHistoryCard({ run }: { run: Run }) {
  const message = normalizeRunMessage(run.error_message);
  const showError = run.status !== 'done' && Boolean(message);

  return (
    <section id={`run-${run.id}`} className={`run-history-card run-status-${run.status}`}>
      <header className="run-history-header">
        <StatusPill kind="run" status={run.status} pulse={run.status === 'running'} />
        <strong>@{run.agent_name || '-'}</strong>
        <span>{getTriggerLabel(run.trigger_type)}</span>
      </header>
      <div className="run-history-meta">
        <span>
          실행 시각: <DateTimeText value={run.finished_at || run.started_at || run.enqueued_at} mode="both" />
        </span>
        {run.heartbeat_at && run.status === 'running' ? (
          <span>
            heartbeat <DateTimeText value={run.heartbeat_at} mode="relative" />
          </span>
        ) : null}
        {typeof run.exit_code === 'number' && <span>exit {run.exit_code}</span>}
        {run.terminal_reason ? <span>{getTerminalReasonLabel(run.terminal_reason)}</span> : null}
        {run.failure_kind ? <span>{getFailureKindLabel(run.failure_kind)}</span> : null}
        {run.cancel_reason ? <span>{getCancelReasonLabel(run.cancel_reason)}</span> : null}
        {typeof run.stdout_size_bytes === 'number' && run.stdout_size_bytes > 0 && <span>로그 {formatBytes(run.stdout_size_bytes)}</span>}
        {run.input_tokens || run.output_tokens ? <span>토큰 {formatTokens((run.input_tokens ?? 0) + (run.output_tokens ?? 0))}</span> : null}
        {run.total_cost_micros ? <span>비용 {formatCostMicros(run.total_cost_micros)}</span> : null}
        {run.model_resolved ? <span>모델 {run.model_resolved}</span> : null}
        {run.agent_instructions_version ? <span>instructions v{run.agent_instructions_version}</span> : null}
        {run.parent_run_id ? <span>parent {run.parent_run_id.slice(0, 8)}</span> : null}
        {run.chain_depth ? <span>chain depth {run.chain_depth}</span> : null}
        {run.max_attempts && run.max_attempts > 1 ? <span>attempt {run.attempt ?? 1}/{run.max_attempts}</span> : null}
        {run.next_retry_at ? <span>다음 재시도 <DateTimeText value={run.next_retry_at} mode="both" /></span> : null}
        {run.log_url && (
          <a className="inline-link" href={run.log_url}>
            전체 로그 보기
          </a>
        )}
      </div>
      {showError && (
        <details className="run-error-panel" open={run.status === 'failed'}>
          <summary>오류 메시지 보기</summary>
          <pre>{message}</pre>
        </details>
      )}
      <RunEventTimeline run={run} />
    </section>
  );
}

function RunEventTimeline({ run }: { run: Run }) {
  const events = useRunEventsQuery(run.id, run.status);

  return (
    <details className="run-event-details" open={run.status === 'running' || run.status === 'failed'}>
      <summary>이벤트 타임라인 {events.data?.length ? `(${events.data.length})` : ''}</summary>
      {events.isError ? <MutationErrorAlert error={events.error} title="이벤트 로드 실패" /> : null}
      <ol className="run-event-timeline">
        {events.data?.map((event) => (
          <RunEventItem key={event.id} event={event} />
        ))}
      </ol>
      {!events.isLoading && !events.data?.length ? <p className="muted-copy">기록된 이벤트가 없습니다.</p> : null}
    </details>
  );
}

function RunEventItem({ event }: { event: RunEvent }) {
  return (
    <li className={`run-event-item run-event-${event.severity}`}>
      <span className="run-event-dot" aria-hidden="true" />
      <div>
        <div className="run-event-header">
          <strong>{getRunEventLabel(event.event_type)}</strong>
          <DateTimeText value={event.created_at} mode="both" />
        </div>
        {event.message ? <p>{event.message}</p> : null}
        {event.details && Object.keys(event.details).length > 0 ? <code>{formatDetails(event.details)}</code> : null}
      </div>
    </li>
  );
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

function formatDetails(details: Record<string, unknown>) {
  return Object.entries(details)
    .map(([key, value]) => `${key}: ${String(value)}`)
    .join(' · ');
}

function formatTokens(value: number) {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`;
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)}k`;
  }
  return String(value);
}

function formatCostMicros(value: number) {
  return `$${(value / 1_000_000).toFixed(4)}`;
}

function formatBytes(value: number) {
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function confirmTitle(action: ConfirmAction | null) {
  const labels: Record<ConfirmAction, string> = {
    rerun: '이슈를 재실행할까요?',
    cancelRun: '현재 실행을 취소할까요?',
    markDone: '이슈를 완료 처리할까요?',
    cancelIssue: '이슈를 취소할까요?',
    reopen: '이슈를 다시 열까요?'
  };
  return action ? labels[action] : '작업 확인';
}

function confirmDescription(action: ConfirmAction | null, title?: string) {
  const target = title ? `“${title}”` : '선택한 이슈';
  const descriptions: Record<ConfirmAction, string> = {
    rerun: `${target}의 새 run을 큐에 등록합니다.`,
    cancelRun: `${target}의 queued/running run에 취소 신호를 보냅니다.`,
    markDone: `${target}을 완료 상태로 전환합니다. active run이 있으면 서버가 차단합니다.`,
    cancelIssue: `${target}을 취소 상태로 닫습니다. queued run은 함께 취소됩니다.`,
    reopen: `${target}을 open 상태로 되돌립니다.`
  };
  return action ? descriptions[action] : undefined;
}

function confirmLabel(action: ConfirmAction | null) {
  const labels: Record<ConfirmAction, string> = {
    rerun: '재실행',
    cancelRun: '실행 취소',
    markDone: '완료 처리',
    cancelIssue: '이슈 취소',
    reopen: '다시 열기'
  };
  return action ? labels[action] : '확인';
}

function statusToast(status: IssueStatus) {
  const labels: Record<IssueStatus, string> = {
    open: '이슈를 다시 열었습니다.',
    done: '이슈를 완료 처리했습니다.',
    cancelled: '이슈를 취소했습니다.'
  };
  return labels[status];
}

function errorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return '요청 처리 중 오류가 발생했습니다.';
}
