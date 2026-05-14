import { useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { useAgentsQuery, useIssuesQuery, type ExecutionStatus, type Issue, type IssueStatus } from '../api/queries';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { CreateIssueDialog } from '../components/CreateIssueDialog';
import { MutationErrorAlert } from '../components/MutationErrorAlert';
import { PageHeader } from '../components/PageHeader';
import { StatusPill } from '../components/StatusPill';
import { useToast } from '../components/ToastProvider';

type ViewMode = 'board' | 'list';
type StatusFilter = 'all' | IssueStatus;
type ExecutionFilter = 'all' | ExecutionStatus;
type PendingAction =
  | { type: 'status'; issue: Issue; status: IssueStatus }
  | { type: 'cancelExecution'; issue: Issue };

const statuses: Array<{ value: IssueStatus; label: string; hint: string }> = [
  { value: 'open', label: '진행', hint: 'Open' },
  { value: 'done', label: '완료', hint: 'Done' },
  { value: 'cancelled', label: '취소', hint: 'Cancelled' }
];
const statusFilterOptions: Array<{ value: StatusFilter; label: string }> = [
  { value: 'all', label: '전체' },
  { value: 'open', label: '진행' },
  { value: 'done', label: '완료' },
  { value: 'cancelled', label: '취소됨' }
];
const executionFilterOptions: Array<{ value: ExecutionFilter; label: string }> = [
  { value: 'all', label: '실행 전체' },
  { value: 'idle', label: '대기 없음' },
  { value: 'queued', label: '대기' },
  { value: 'running', label: '실행 중' },
  { value: 'done', label: '완료' },
  { value: 'failed', label: '실패' },
  { value: 'cancelled', label: '취소' }
];

function IssueCard({
  issue,
  slug,
  onRequestStatus,
  onRequestCancelExecution,
  disabled
}: {
  issue: Issue;
  slug: string | undefined;
  onRequestStatus: (issue: Issue, status: IssueStatus) => void;
  onRequestCancelExecution: (issue: Issue) => void;
  disabled: boolean;
}) {
  const busy = issue.execution_status === 'queued' || issue.execution_status === 'running';
  return (
    <article className="kanban-card">
      <Link className="kanban-card-link" to={`/w/${slug}/issues/${issue.identifier}`}>
        <span className="issue-id">{issue.identifier}</span>
        <strong>{issue.title}</strong>
        <div className="card-status-row">
          <StatusPill kind="execution" status={issue.execution_status} pulse={busy} />
          <StatusPill kind="issue" status={issue.status} />
        </div>
        <small>
          @{issue.last_run_agent_name || issue.assignee_agent_name || '메인 에이전트'} · 댓글 {issue.comment_count}
        </small>
      </Link>
      <div className="card-actions">
        {issue.status !== 'open' && (
          <button className="button micro secondary" type="button" onClick={() => onRequestStatus(issue, 'open')} disabled={disabled}>
            다시 열기
          </button>
        )}
        {issue.status === 'open' && (
          <button className="button micro secondary" type="button" onClick={() => onRequestStatus(issue, 'done')} disabled={busy || disabled}>
            완료
          </button>
        )}
        {busy ? (
          <button className="button micro danger" type="button" onClick={() => onRequestCancelExecution(issue)} disabled={disabled}>
            {issue.execution_status === 'queued' ? '대기 취소' : '실행 취소'}
          </button>
        ) : issue.status !== 'cancelled' ? (
          <button className="button micro danger" type="button" onClick={() => onRequestStatus(issue, 'cancelled')} disabled={disabled}>
            이슈 취소
          </button>
        ) : null}
      </div>
    </article>
  );
}

export function BoardPage() {
  const { slug } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const toast = useToast();
  const [viewMode, setViewMode] = useState<ViewMode>((searchParams.get('view') as ViewMode) === 'list' ? 'list' : 'board');
  const [dialogStatus, setDialogStatus] = useState<IssueStatus | null>(null);
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null);

  const statusFilter = validStatusFilter(searchParams.get('status'));
  const executionFilter = validExecutionFilter(searchParams.get('execution'));
  const agentFilter = searchParams.get('agent') ?? '';
  const query = searchParams.get('q') ?? '';
  const issues = useIssuesQuery(slug, {
    status: statusFilter,
    execution: executionFilter,
    assignee: agentFilter,
    q: query,
    limit: 200
  });
  const agents = useAgentsQuery(slug);

  const invalidateIssues = () => queryClient.invalidateQueries({ queryKey: ['issues', slug] });
  const updateStatus = useMutation({
    mutationFn: ({ issue, status }: { issue: Issue; status: IssueStatus }) => apiClient.put(`/issues/${issue.id}`, { status }),
    onSuccess: (_, variables) => {
      invalidateIssues();
      toast.success(statusToast(variables.status), { description: variables.issue.identifier });
    },
    onError: (error) => toast.error('상태 변경 실패', { description: errorMessage(error) }),
    onSettled: () => setPendingAction(null)
  });
  const cancelExecution = useMutation({
    mutationFn: (issue: Issue) => apiClient.post(`/issues/${issue.id}/cancel`),
    onSuccess: (_, issue) => {
      invalidateIssues();
      toast.success('실행 취소를 요청했습니다.', { description: issue.identifier });
    },
    onError: (error) => toast.error('실행 취소 실패', { description: errorMessage(error) }),
    onSettled: () => setPendingAction(null)
  });

  const allIssues = issues.data ?? [];
  const counts = useMemo(() => {
    return statuses.reduce<Record<IssueStatus, number>>(
      (acc, status) => {
        acc[status.value] = allIssues.filter((issue) => issue.status === status.value).length;
        return acc;
      },
      { open: 0, done: 0, cancelled: 0 }
    );
  }, [allIssues]);
  const grouped = useMemo(() => {
    return statuses.reduce<Record<IssueStatus, Issue[]>>(
      (acc, status) => {
        acc[status.value] = allIssues.filter((issue) => issue.status === status.value);
        return acc;
      },
      { open: [], done: [], cancelled: [] }
    );
  }, [allIssues]);

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    if (!value || value === 'all') {
      next.delete(key);
    } else {
      next.set(key, value);
    }
    if (key !== 'view') {
      next.delete('cursor');
    }
    setSearchParams(next, { replace: true });
  };
  const setSearchQuery = (value: string) => setParam('q', value);
  const setView = (value: ViewMode) => {
    setViewMode(value);
    setParam('view', value === 'board' ? '' : 'list');
  };
  const executePendingAction = () => {
    if (!pendingAction) {
      return;
    }
    if (pendingAction.type === 'status') {
      updateStatus.mutate({ issue: pendingAction.issue, status: pendingAction.status });
      return;
    }
    cancelExecution.mutate(pendingAction.issue);
  };
  const anyMutationPending = updateStatus.isPending || cancelExecution.isPending;

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="이슈 보드"
        description="기존 이슈, 완료 이슈, 취소 이슈를 보드/리스트로 전환하며 추적합니다."
      />

      <div className="board-toolbar panel">
        <div className="toolbar-main">
          <div>
            <h2>전체 이슈</h2>
            <p>open {counts.open} · done {counts.done} · cancelled {counts.cancelled}</p>
          </div>
          <button className="button" type="button" onClick={() => setDialogStatus('open')}>
            새 이슈
          </button>
        </div>
        <div className="toolbar-controls board-filter-grid">
          <div className="segmented" role="tablist" aria-label="이슈 상태 필터">
            {statusFilterOptions.map((option) => (
              <button key={option.value} type="button" className={statusFilter === option.value ? 'active' : ''} onClick={() => setParam('status', option.value)}>
                {option.label}
              </button>
            ))}
          </div>
          <select className="toolbar-select" value={executionFilter} onChange={(event) => setParam('execution', event.target.value)} aria-label="실행 상태 필터">
            {executionFilterOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <select className="toolbar-select" value={agentFilter} onChange={(event) => setParam('agent', event.target.value)} aria-label="담당 에이전트 필터">
            <option value="">담당 전체</option>
            {(agents.data ?? []).map((agent) => (
              <option key={agent.id} value={agent.id}>
                @{agent.name}
              </option>
            ))}
          </select>
          <div className="segmented" role="tablist" aria-label="보기 방식">
            <button type="button" className={viewMode === 'board' ? 'active' : ''} onClick={() => setView('board')}>
              보드
            </button>
            <button type="button" className={viewMode === 'list' ? 'active' : ''} onClick={() => setView('list')}>
              리스트
            </button>
          </div>
          <input className="toolbar-search" placeholder="이슈 검색" value={query} onChange={(event) => setSearchQuery(event.target.value)} />
        </div>
        <ActiveFilterSummary status={statusFilter} execution={executionFilter} agentId={agentFilter} q={query} agents={agents.data ?? []} onClear={() => setSearchParams(new URLSearchParams(), { replace: true })} />
      </div>

      {updateStatus.isError ? <MutationErrorAlert error={updateStatus.error} title="상태 변경 실패" /> : null}
      {cancelExecution.isError ? <MutationErrorAlert error={cancelExecution.error} title="실행 취소 실패" /> : null}

      {viewMode === 'board' ? (
        <div className="kanban-board">
          {statuses.map((status) => (
            <section className={`kanban-column ${status.value}`} key={status.value}>
              <header className="kanban-column-header">
                <div>
                  <h2>{status.label}</h2>
                  <small>{status.hint}</small>
                </div>
                <div className="column-actions">
                  <span className="badge">{grouped[status.value].length}</span>
                  <button className="icon-button" type="button" onClick={() => setDialogStatus(status.value)} aria-label={`${status.label} 컬럼에서 새 이슈`}>
                    +
                  </button>
                </div>
              </header>
              <div className="kanban-column-body">
                {grouped[status.value].map((issue) => (
                  <IssueCard
                    key={issue.id}
                    issue={issue}
                    slug={slug}
                    onRequestStatus={(target, nextStatus) => setPendingAction({ type: 'status', issue: target, status: nextStatus })}
                    onRequestCancelExecution={(target) => setPendingAction({ type: 'cancelExecution', issue: target })}
                    disabled={anyMutationPending}
                  />
                ))}
                {!grouped[status.value].length && <p className="column-empty">표시할 이슈가 없습니다.</p>}
              </div>
            </section>
          ))}
        </div>
      ) : (
        <article className="panel table-panel">
          <div className="data-table issue-table">
            <div className="data-row data-head">
              <span>이슈</span>
              <span>상태</span>
              <span>실행</span>
              <span>담당</span>
              <span>댓글</span>
            </div>
            {allIssues.map((issue) => {
              const busy = issue.execution_status === 'queued' || issue.execution_status === 'running';
              return (
                <Link className="data-row" key={issue.id} to={`/w/${slug}/issues/${issue.identifier}`}>
                  <span>
                    <span className="issue-id">{issue.identifier}</span>
                    <strong>{issue.title}</strong>
                  </span>
                  <span><StatusPill kind="issue" status={issue.status} /></span>
                  <span><StatusPill kind="execution" status={issue.execution_status} pulse={busy} /></span>
                  <span>@{issue.last_run_agent_name || issue.assignee_agent_name || '메인 에이전트'}</span>
                  <span>{issue.comment_count}</span>
                </Link>
              );
            })}
          </div>
          {!issues.isLoading && !allIssues.length && <p>조건에 맞는 이슈가 없습니다.</p>}
        </article>
      )}

      {!issues.isLoading && !allIssues.length && (
        <article className="panel empty-state">
          <h2>이슈 없음</h2>
          <p>{issues.isError ? '이슈 목록을 불러오지 못했습니다.' : '새 이슈 버튼으로 첫 작업을 생성하세요.'}</p>
        </article>
      )}

      <CreateIssueDialog open={Boolean(dialogStatus)} slug={slug} statusHint={dialogStatus ?? 'open'} onClose={() => setDialogStatus(null)} />
      <ConfirmDialog
        open={Boolean(pendingAction)}
        title={pendingTitle(pendingAction)}
        description={pendingDescription(pendingAction)}
        confirmLabel={pendingConfirmLabel(pendingAction)}
        tone={pendingAction?.type === 'cancelExecution' || pendingAction?.status === 'cancelled' ? 'danger' : 'default'}
        pending={anyMutationPending}
        onConfirm={executePendingAction}
        onClose={() => setPendingAction(null)}
      />
    </section>
  );
}

function ActiveFilterSummary({
  status,
  execution,
  agentId,
  q,
  agents,
  onClear
}: {
  status: StatusFilter;
  execution: ExecutionFilter;
  agentId: string;
  q: string;
  agents: Array<{ id: string; name: string }>;
  onClear: () => void;
}) {
  const agent = agents.find((item) => item.id === agentId);
  const chips = [
    status !== 'all' ? `상태: ${statusLabel(status)}` : '',
    execution !== 'all' ? `실행: ${executionLabel(execution)}` : '',
    agent ? `담당: @${agent.name}` : '',
    q.trim() ? `검색: ${q.trim()}` : ''
  ].filter(Boolean);

  if (!chips.length) {
    return <p className="filter-summary">필터 없음 · 전체 이슈를 표시합니다.</p>;
  }
  return (
    <div className="filter-summary active">
      {chips.map((chip) => (
        <span className="badge muted" key={chip}>{chip}</span>
      ))}
      <button className="button micro secondary" type="button" onClick={onClear}>
        필터 초기화
      </button>
    </div>
  );
}

function validStatusFilter(value: string | null): StatusFilter {
  return value === 'open' || value === 'done' || value === 'cancelled' ? value : 'all';
}

function validExecutionFilter(value: string | null): ExecutionFilter {
  return value === 'idle' || value === 'queued' || value === 'running' || value === 'done' || value === 'failed' || value === 'cancelled' ? value : 'all';
}

function statusLabel(status: string) {
  const labels: Record<string, string> = { all: '전체', open: '진행', done: '완료', cancelled: '취소' };
  return labels[status] ?? status;
}

function executionLabel(status: string) {
  const labels: Record<string, string> = {
    all: '전체',
    idle: '대기 없음',
    queued: '대기',
    running: '실행 중',
    done: '완료',
    failed: '실패',
    cancelled: '취소'
  };
  return labels[status] ?? status;
}

function pendingTitle(action: PendingAction | null) {
  if (!action) {
    return '작업 확인';
  }
  if (action.type === 'cancelExecution') {
    return action.issue.execution_status === 'queued' ? '대기 run을 취소할까요?' : '실행 중인 run을 취소할까요?';
  }
  const titles: Record<IssueStatus, string> = {
    open: '이슈를 다시 열까요?',
    done: '이슈를 완료 처리할까요?',
    cancelled: '이슈를 취소할까요?'
  };
  return titles[action.status];
}

function pendingDescription(action: PendingAction | null) {
  if (!action) {
    return undefined;
  }
  const target = `“${action.issue.identifier} · ${action.issue.title}”`;
  if (action.type === 'cancelExecution') {
    return `${target}의 active run을 취소합니다. 이슈 자체는 open 상태로 유지됩니다.`;
  }
  if (action.status === 'done') {
    return `${target}을 완료 상태로 전환합니다. 서버는 queued/running run이 있으면 요청을 차단합니다.`;
  }
  if (action.status === 'cancelled') {
    return `${target}을 취소 상태로 닫습니다. queued run은 함께 취소됩니다.`;
  }
  return `${target}을 open 상태로 되돌립니다.`;
}

function pendingConfirmLabel(action: PendingAction | null) {
  if (!action) {
    return '확인';
  }
  if (action.type === 'cancelExecution') {
    return '실행 취소';
  }
  const labels: Record<IssueStatus, string> = { open: '다시 열기', done: '완료 처리', cancelled: '이슈 취소' };
  return labels[action.status];
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
