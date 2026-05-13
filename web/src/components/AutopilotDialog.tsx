import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import { useAgentsQuery } from '../api/queries';
import { Modal } from './Modal';

export type AutopilotTemplate = {
  name: string;
  cron_expr: string;
  issue_title_template: string;
  issue_body_template: string;
};

type AutopilotDialogProps = {
  open: boolean;
  slug: string | undefined;
  template?: AutopilotTemplate | null;
  onClose: () => void;
};

const emptyForm = {
  name: '',
  cron_expr: '0 9 * * *',
  issue_title_template: '{{date}} 정기 작업',
  issue_body_template: '',
  assignee_agent_id: '',
  enabled: true
};

export function AutopilotDialog({ open, slug, template, onClose }: AutopilotDialogProps) {
  const queryClient = useQueryClient();
  const agents = useAgentsQuery(slug);
  const [form, setForm] = useState(emptyForm);

  useEffect(() => {
    if (!open) {
      return;
    }
    setForm({ ...emptyForm, ...(template ?? {}) });
  }, [open, template]);

  const createRule = useMutation({
    mutationFn: () => apiClient.post(`/workspaces/${slug}/autopilot`, form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['autopilot', slug] });
      onClose();
    }
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    createRule.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="오토파일럿 추가"
      description="cron 일정에 따라 이슈를 생성하거나 에이전트 작업을 예약합니다."
      footer={
        <>
          <button className="button secondary" type="button" onClick={onClose}>
            취소
          </button>
          <button className="button" type="submit" form="create-autopilot-form" disabled={createRule.isPending || !slug}>
            {createRule.isPending ? '추가 중' : '오토파일럿 추가'}
          </button>
        </>
      }
    >
      <form id="create-autopilot-form" className="form-grid" onSubmit={onSubmit}>
        <label className="field-label">
          이름
          <input autoFocus placeholder="이름" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label className="field-label">
          Cron
          <input placeholder="cron" value={form.cron_expr} onChange={(e) => setForm({ ...form, cron_expr: e.target.value })} required />
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
          이슈 제목 템플릿
          <input
            placeholder="이슈 제목 템플릿"
            value={form.issue_title_template}
            onChange={(e) => setForm({ ...form, issue_title_template: e.target.value })}
            required
          />
        </label>
        <label className="field-label">
          이슈 본문 템플릿
          <textarea
            placeholder="이슈 본문 템플릿"
            value={form.issue_body_template}
            onChange={(e) => setForm({ ...form, issue_body_template: e.target.value })}
          />
        </label>
        <label className="check-row">
          <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> ON
        </label>
        {createRule.isError && <p className="error-text">오토파일럿 추가에 실패했습니다.</p>}
      </form>
    </Modal>
  );
}
