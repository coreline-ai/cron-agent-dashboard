import { useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { useIssuesQuery, type Issue, type IssueStatus } from '../api/queries';
import { CreateIssueDialog } from '../components/CreateIssueDialog';
import { PageHeader } from '../components/PageHeader';

type ViewMode = 'board' | 'list';
type StatusFilter = 'all' | IssueStatus;

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

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function executionLabel(status: string) {
  const labels: Record<string, string> = {
    idle: '대기 없음',
    queued: 'Queued',
    running: 'Running',
    done: 'Done',
    failed: 'Failed',
    cancelled: 'Cancelled'
  };
  return labels[status] ?? status;
}

function IssueCard({ issue, slug, onStatusChange, isUpdating }: { issue: Issue; slug: string | undefined; onStatusChange: (issue: Issue, status: IssueStatus) => void; isUpdating: boolean }) {
  const busy = issue.execution_status === 'queued' || issue.execution_status === 'running';
  return (
    <article className="kanban-card">
      <Link className="kanban-card-link" to={`/w/${slug}/issues/${issue.identifier}`}>
        <span className="issue-id">{issue.identifier}</span>
        <strong>{issue.title}</strong>
        <small>
          {executionLabel(issue.execution_status)} · @{issue.last_run_agent_name || issue.assignee_agent_name || '-'} · 댓글 {issue.comment_count}
        </small>
      </Link>
      <div className="card-actions">
        {issue.status !== 'open' && (
          <button className="button micro secondary" type="button" onClick={() => onStatusChange(issue, 'open')} disabled={isUpdating}>
            다시 열기
          </button>
        )}
        {issue.status === 'open' && (
          <button className="button micro secondary" type="button" onClick={() => onStatusChange(issue, 'done')} disabled={busy || isUpdating}>
            완료
          </button>
        )}
        {issue.status !== 'cancelled' && (
          <button className="button micro danger" type="button" onClick={() => onStatusChange(issue, 'cancelled')} disabled={busy || isUpdating}>
            취소
          </button>
        )}
      </div>
    </article>
  );
}

export function BoardPage() {
  const { slug } = useParams();
  const issues = useIssuesQuery(slug);
  const queryClient = useQueryClient();
  const [viewMode, setViewMode] = useState<ViewMode>('board');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [query, setQuery] = useState('');
  const [dialogStatus, setDialogStatus] = useState<IssueStatus | null>(null);
  const updateStatus = useMutation({
    mutationFn: ({ issue, status }: { issue: Issue; status: IssueStatus }) => apiClient.put(`/issues/${issue.id}`, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['issues', slug] });
    }
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
  const filteredIssues = useMemo(() => {
    const q = query.trim().toLowerCase();
    return allIssues.filter((issue) => {
      if (statusFilter !== 'all' && issue.status !== statusFilter) {
        return false;
      }
      if (!q) {
        return true;
      }
      return [issue.identifier, issue.title, issue.body, issue.assignee_agent_name, issue.last_run_agent_name]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(q));
    });
  }, [allIssues, query, statusFilter]);
  const grouped = useMemo(() => {
    return statuses.reduce<Record<IssueStatus, Issue[]>>(
      (acc, status) => {
        acc[status.value] = filteredIssues.filter((issue) => issue.status === status.value);
        return acc;
      },
      { open: [], done: [], cancelled: [] }
    );
  }, [filteredIssues]);

  const onStatusChange = (issue: Issue, status: IssueStatus) => updateStatus.mutate({ issue, status });

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
        <div className="toolbar-controls">
          <div className="segmented" role="tablist" aria-label="이슈 상태 필터">
            {statusFilterOptions.map((option) => (
              <button
                key={option.value}
                type="button"
                className={statusFilter === option.value ? 'active' : ''}
                onClick={() => setStatusFilter(option.value)}
              >
                {option.label}
              </button>
            ))}
          </div>
          <div className="segmented" role="tablist" aria-label="보기 방식">
            <button type="button" className={viewMode === 'board' ? 'active' : ''} onClick={() => setViewMode('board')}>
              보드
            </button>
            <button type="button" className={viewMode === 'list' ? 'active' : ''} onClick={() => setViewMode('list')}>
              리스트
            </button>
          </div>
          <input className="toolbar-search" placeholder="이슈 검색" value={query} onChange={(event) => setQuery(event.target.value)} />
        </div>
      </div>

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
                    onStatusChange={onStatusChange}
                    isUpdating={updateStatus.isPending}
                  />
                ))}
                {!grouped[status.value].length && <p className="column-empty">표시할 이슈가 없습니다.</p>}
              </div>
            </section>
          ))}
        </div>
      ) : (
        <article className="panel table-panel">
          <div className="data-table">
            <div className="data-row data-head">
              <span>이슈</span>
              <span>상태</span>
              <span>실행</span>
              <span>담당</span>
              <span>댓글</span>
            </div>
            {filteredIssues.map((issue) => (
              <Link className="data-row" key={issue.id} to={`/w/${slug}/issues/${issue.identifier}`}>
                <span>
                  <span className="issue-id">{issue.identifier}</span>
                  <strong>{issue.title}</strong>
                </span>
                <span>{statusLabel(issue.status)}</span>
                <span>{executionLabel(issue.execution_status)}</span>
                <span>@{issue.last_run_agent_name || issue.assignee_agent_name || '-'}</span>
                <span>{issue.comment_count}</span>
              </Link>
            ))}
          </div>
          {!issues.isLoading && !filteredIssues.length && <p>조건에 맞는 이슈가 없습니다.</p>}
        </article>
      )}

      {!issues.isLoading && !allIssues.length && (
        <article className="panel empty-state">
          <h2>이슈 없음</h2>
          <p>{issues.isError ? '이슈 목록을 불러오지 못했습니다.' : '새 이슈 버튼으로 첫 작업을 생성하세요.'}</p>
        </article>
      )}

      <CreateIssueDialog open={Boolean(dialogStatus)} slug={slug} statusHint={dialogStatus ?? 'open'} onClose={() => setDialogStatus(null)} />
    </section>
  );
}
