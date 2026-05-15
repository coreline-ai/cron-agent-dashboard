import { FormEvent, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import { ModelSelect } from './ModelSelect';
import { Modal } from './Modal';

type CreateAgentDialogProps = {
  open: boolean;
  slug: string | undefined;
  onClose: () => void;
};

export function CreateAgentDialog({ open, slug, onClose }: CreateAgentDialogProps) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: '', runtime: 'codex', model: '', summary: '', tags: '', instructions: '' });
  const createAgent = useMutation({
    mutationFn: () => apiClient.post(`/workspaces/${slug}/agents`, form),
    onSuccess: () => {
      setForm({ name: '', runtime: 'codex', model: '', summary: '', tags: '', instructions: '' });
      queryClient.invalidateQueries({ queryKey: ['agents', slug] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      onClose();
    }
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    createAgent.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="에이전트 추가"
      description="CLI runtime과 기본 지시문을 지정해 워크스페이스에 새 작업자를 추가합니다."
      footer={
        <>
          <button className="button secondary" type="button" onClick={onClose}>
            취소
          </button>
          <button className="button" type="submit" form="create-agent-form" disabled={createAgent.isPending || !slug}>
            {createAgent.isPending ? '추가 중' : '에이전트 추가'}
          </button>
        </>
      }
    >
      <form id="create-agent-form" className="form-grid" onSubmit={onSubmit}>
        <label className="field-label">
          이름
          <input autoFocus placeholder="이름" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label className="field-label">
          Runtime
          <select value={form.runtime} onChange={(e) => setForm({ ...form, runtime: e.target.value })}>
            <option value="codex">codex</option>
            <option value="claude">claude</option>
            <option value="gemini">gemini</option>
          </select>
        </label>
        <ModelSelect runtime={form.runtime} value={form.model} onChange={(model) => setForm({ ...form, model })} />
        <label className="field-label">
          한 줄 요약
          <input placeholder="예: 긴 리서치를 요약하는 작성자" value={form.summary} onChange={(e) => setForm({ ...form, summary: e.target.value })} />
        </label>
        <label className="field-label">
          태그
          <input placeholder="research,writing,korean" value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} />
        </label>
        <label className="field-label">
          지시문
          <textarea placeholder="지시문" value={form.instructions} onChange={(e) => setForm({ ...form, instructions: e.target.value })} required />
        </label>
        {createAgent.isError && <p className="error-text">에이전트 추가에 실패했습니다.</p>}
      </form>
    </Modal>
  );
}
