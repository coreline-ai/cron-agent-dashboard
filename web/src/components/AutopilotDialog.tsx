import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import { useAgentsQuery, type AutopilotRule } from '../api/queries';
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
  rule?: AutopilotRule | null;
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

const cronPresets = [
  { label: '매일 오전 9시', value: '0 9 * * *' },
  { label: '평일 오전 9시', value: '0 9 * * 1-5' },
  { label: '매주 월요일', value: '0 10 * * 1' },
  { label: '매월 1일', value: '0 9 1 * *' }
];

export function AutopilotDialog({ open, slug, template, rule, onClose }: AutopilotDialogProps) {
  const queryClient = useQueryClient();
  const agents = useAgentsQuery(slug);
  const [form, setForm] = useState(emptyForm);
  const isEdit = Boolean(rule);
  const formID = isEdit ? 'edit-autopilot-form' : 'create-autopilot-form';

  useEffect(() => {
    if (!open) {
      return;
    }
    if (rule) {
      setForm({
        name: rule.name,
        cron_expr: rule.cron_expr,
        issue_title_template: rule.issue_title_template,
        issue_body_template: rule.issue_body_template ?? '',
        assignee_agent_id: rule.assignee_agent_id ?? '',
        enabled: rule.enabled
      });
      return;
    }
    setForm({ ...emptyForm, ...(template ?? {}) });
  }, [open, template, rule]);

  const saveRule = useMutation({
    mutationFn: () =>
      isEdit && rule
        ? apiClient.put(`/autopilot/${rule.id}`, form)
        : apiClient.post(`/workspaces/${slug}/autopilot`, form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['autopilot', slug] });
      onClose();
    }
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    saveRule.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isEdit ? '오토파일럿 편집' : '오토파일럿 추가'}
      description="cron 일정에 따라 이슈를 생성하거나 에이전트 작업을 예약합니다. 편집해도 실패 이력은 유지됩니다."
      footer={
        <>
          <button className="button secondary" type="button" onClick={onClose}>
            취소
          </button>
          <button className="button" type="submit" form={formID} disabled={saveRule.isPending || !slug}>
            {saveRule.isPending ? '저장 중' : isEdit ? '변경 저장' : '오토파일럿 추가'}
          </button>
        </>
      }
    >
      <form id={formID} className="form-grid" onSubmit={onSubmit}>
        <label className="field-label">
          이름
          <input autoFocus placeholder="이름" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label className="field-label">
          Cron
          <input placeholder="cron" value={form.cron_expr} onChange={(e) => setForm({ ...form, cron_expr: e.target.value })} required />
        </label>
        <div className="field-label">
          Cron 프리셋
          <div className="cron-preset-grid">
            {cronPresets.map((preset) => (
              <button
                key={preset.value}
                type="button"
                className={form.cron_expr === preset.value ? 'active' : ''}
                onClick={() => setForm({ ...form, cron_expr: preset.value })}
              >
                <span>{preset.label}</span>
                <small>{preset.value}</small>
              </button>
            ))}
          </div>
        </div>
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
        {saveRule.isError && <p className="error-text">오토파일럿 저장에 실패했습니다.</p>}
      </form>
    </Modal>
  );
}
