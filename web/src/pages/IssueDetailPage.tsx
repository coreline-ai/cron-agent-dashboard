import { FormEvent, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { MarkdownText } from '../components/MarkdownText';
import { PageHeader } from '../components/PageHeader';
import { useCommentsQuery, useRunsQuery, useWorkspaceIssueQuery } from '../api/queries';
import type { Run } from '../api/queries';

export function IssueDetailPage() {
  const { identifier, slug } = useParams();
  const issue = useWorkspaceIssueQuery(slug, identifier);
  const comments = useCommentsQuery(issue.data?.id, issue.data?.execution_status);
  const runs = useRunsQuery(issue.data?.id, issue.data?.execution_status);
  const queryClient = useQueryClient();
  const [content, setContent] = useState('');
  const invalidateIssue = () => {
    queryClient.invalidateQueries({ queryKey: ['issue', slug, identifier] });
    queryClient.invalidateQueries({ queryKey: ['comments', issue.data?.id] });
    queryClient.invalidateQueries({ queryKey: ['runs', issue.data?.id] });
    queryClient.invalidateQueries({ queryKey: ['issues', slug] });
  };
  const addComment = useMutation({
    mutationFn: () => apiClient.post(`/issues/${issue.data?.id}/comments`, { content }),
    onSuccess: () => {
      setContent('');
      invalidateIssue();
    }
  });
  const rerun = useMutation({ mutationFn: () => apiClient.post(`/issues/${issue.data?.id}/rerun`), onSuccess: invalidateIssue });
  const cancelRun = useMutation({ mutationFn: () => apiClient.post(`/issues/${issue.data?.id}/cancel`), onSuccess: invalidateIssue });
  const markDone = useMutation({
    mutationFn: () => apiClient.put(`/issues/${issue.data?.id}`, { status: 'done' }),
    onSuccess: invalidateIssue
  });
  const cancelIssue = useMutation({
    mutationFn: () => apiClient.put(`/issues/${issue.data?.id}`, { status: 'cancelled' }),
    onSuccess: invalidateIssue
  });
  const onCommentSubmit = (event: FormEvent) => {
    event.preventDefault();
    addComment.mutate();
  };
  const busy = issue.data?.execution_status === 'queued' || issue.data?.execution_status === 'running';

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
      />
      <article className="panel">
        <h2>본문</h2>
        <MarkdownText value={issue.data?.body || '(본문 없음)'} />
        <div className="button-row">
          <button className="button secondary" type="button" onClick={() => rerun.mutate()} disabled={!issue.data || busy || rerun.isPending}>
            재실행
          </button>
          <button className="button secondary" type="button" onClick={() => cancelRun.mutate()} disabled={!issue.data || !busy || cancelRun.isPending}>
            {issue.data?.execution_status === 'queued' ? '대기 취소' : '실행 취소'}
          </button>
          <button className="button secondary" type="button" onClick={() => markDone.mutate()} disabled={!issue.data || busy}>
            완료 처리
          </button>
          <button className="button danger" type="button" onClick={() => cancelIssue.mutate()} disabled={!issue.data || busy}>
            이슈 취소
          </button>
        </div>
      </article>
      <article className="panel">
        <h2>댓글 스레드</h2>
        {comments.data?.map((comment) => (
          <div className="comment-block" key={comment.id}>
            <strong>{comment.author_agent_name || comment.author_type}</strong>
            <MarkdownText value={comment.content} />
            {comment.log_url && (
              <a className="inline-link" href={comment.log_url}>
                로그 보기
              </a>
            )}
          </div>
        ))}
        {!comments.isLoading && !comments.data?.length && <p>아직 댓글이 없습니다.</p>}
        <form className="form-grid" onSubmit={onCommentSubmit}>
          <textarea placeholder="@AgentName 멘션으로 위임할 수 있습니다." value={content} onChange={(e) => setContent(e.target.value)} required />
          <button className="button" type="submit" disabled={!issue.data || addComment.isPending}>
            {addComment.isPending ? '등록 중' : '댓글 등록'}
          </button>
          {addComment.isError && <p className="error-text">댓글 등록에 실패했습니다.</p>}
        </form>
      </article>
      <article className="panel">
        <h2>Run 이력</h2>
        <div className="run-history-list">
          {runs.data?.map((run) => (
            <RunHistoryCard key={run.id} run={run} />
          ))}
        </div>
        {!runs.isLoading && !runs.data?.length && <p>아직 실행 이력이 없습니다.</p>}
      </article>
    </section>
  );
}

function RunHistoryCard({ run }: { run: Run }) {
  const message = normalizeRunMessage(run.error_message);
  const showError = run.status !== 'done' && Boolean(message);

  return (
    <section className={`run-history-card run-status-${run.status}`}>
      <header className="run-history-header">
        <span className={`status-pill status-${run.status}`}>{runStatusLabel(run.status)}</span>
        <strong>@{run.agent_name || '-'}</strong>
        <span>{triggerLabel(run.trigger_type)}</span>
      </header>
      <div className="run-history-meta">
        <span>실행 시각: {formatDateTime(run.finished_at || run.started_at || run.enqueued_at)}</span>
        {typeof run.exit_code === 'number' && <span>exit {run.exit_code}</span>}
        {typeof run.stdout_size_bytes === 'number' && run.stdout_size_bytes > 0 && <span>로그 {formatBytes(run.stdout_size_bytes)}</span>}
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
    </section>
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

function runStatusLabel(status: string) {
  const labels: Record<string, string> = {
    queued: '대기',
    running: '실행 중',
    done: '완료',
    failed: '실패',
    cancelled: '취소'
  };
  return labels[status] ?? status;
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

function formatDateTime(value?: string) {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat('ko-KR', {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(date);
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
