import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import { useAgentsQuery, type IssueStatus } from '../api/queries';
import { Modal } from './Modal';

export type CreateIssuePrefill = {
  title?: string;
  body?: string;
};

type CreateIssueDialogProps = {
  open: boolean;
  slug: string | undefined;
  statusHint?: IssueStatus;
  prefill?: CreateIssuePrefill;
  onClose: () => void;
};

export function CreateIssueDialog({ open, slug, statusHint = 'open', prefill, onClose }: CreateIssueDialogProps) {
  const queryClient = useQueryClient();
  const agents = useAgentsQuery(slug);
  const [form, setForm] = useState({ title: '', body: '', assignee_agent_id: '' });
  // Apply prefill the moment the dialog opens so the operator sees the
  // sample issue's title/body filled in without having to copy-paste.
  useEffect(() => {
    if (!open) return;
    if (prefill) {
      setForm({ title: prefill.title ?? '', body: prefill.body ?? '', assignee_agent_id: '' });
    }
  }, [open, prefill]);
  const createIssue = useMutation({
    mutationFn: () => apiClient.post(`/workspaces/${slug}/issues`, form),
    onSuccess: () => {
      setForm({ title: '', body: '', assignee_agent_id: '' });
      queryClient.invalidateQueries({ queryKey: ['issues', slug] });
      onClose();
    }
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    createIssue.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="새 이슈"
      description={
        statusHint === 'open'
          ? '작업 요청을 생성하면 담당 에이전트 실행이 자동으로 queued 됩니다.'
          : '완료/취소 컬럼에서도 새 이슈는 안전하게 open 상태로 생성됩니다.'
      }
      footer={
        <>
          <button className="button secondary" type="button" onClick={onClose}>
            취소
          </button>
          <button className="button" type="submit" form="create-issue-form" disabled={createIssue.isPending || !slug}>
            {createIssue.isPending ? '생성 중' : '이슈 생성'}
          </button>
        </>
      }
    >
      <form id="create-issue-form" className="form-grid" onSubmit={onSubmit}>
        <label className="field-label">
          제목
          <input autoFocus placeholder="제목" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required />
        </label>
        <label className="field-label">
          담당 에이전트
          <select value={form.assignee_agent_id} onChange={(e) => setForm({ ...form, assignee_agent_id: e.target.value })}>
            <option value="">메인 에이전트 자동 선택</option>
            {(agents.data ?? []).map((agent) => (
              <option key={agent.id} value={agent.id}>
                @{agent.name} · {agent.runtime}
              </option>
            ))}
          </select>
        </label>
        <label className="field-label">
          본문
          <textarea placeholder="본문" value={form.body} onChange={(e) => setForm({ ...form, body: e.target.value })} />
        </label>
        {createIssue.isError && <p className="error-text">이슈 생성에 실패했습니다.</p>}
      </form>
    </Modal>
  );
}
