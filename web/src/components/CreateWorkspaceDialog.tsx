import { FormEvent, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import { useSettingsQuery, type WorkspaceSummary } from '../api/queries';
import { ModelSelect } from './ModelSelect';
import { Modal } from './Modal';
import { RuntimeBadge } from './RuntimeBadge';

type CreateWorkspaceResponse = {
  workspace: WorkspaceSummary;
};

type CreateWorkspaceDialogProps = {
  open: boolean;
  onClose: () => void;
  onCreated?: (workspace: WorkspaceSummary) => void;
};

const initialForm = {
  name: '',
  slug: '',
  identifier_prefix: 'TASK',
  main_agent_name: 'Codex',
  runtime: 'codex',
  model: '',
  instructions: '작업을 수행하고 결과를 한국어 markdown으로 요약하세요.',
  auto_chain_enabled: false
};

export function CreateWorkspaceDialog({ open, onClose, onCreated }: CreateWorkspaceDialogProps) {
  const queryClient = useQueryClient();
  const settings = useSettingsQuery();
  const [form, setForm] = useState(initialForm);
  const createWorkspace = useMutation({
    mutationFn: () =>
      apiClient.post<CreateWorkspaceResponse>('/workspaces', {
        name: form.name,
        slug: form.slug,
        identifier_prefix: form.identifier_prefix,
        auto_chain_enabled: form.auto_chain_enabled,
        main_agent: {
          name: form.main_agent_name,
          runtime: form.runtime,
          model: form.model,
          instructions: form.instructions
        }
      }),
    onSuccess: (data) => {
      setForm(initialForm);
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      onClose();
      onCreated?.(data.workspace);
    }
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    createWorkspace.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="워크스페이스 생성"
      description="이슈 보드, 에이전트, 오토파일럿을 묶는 독립 작업 공간을 만듭니다."
      footer={
        <>
          <button className="button secondary" type="button" onClick={onClose}>
            취소
          </button>
          <button className="button" type="submit" form="create-workspace-form" disabled={createWorkspace.isPending}>
            {createWorkspace.isPending ? '생성 중' : '워크스페이스 생성'}
          </button>
        </>
      }
    >
      <form id="create-workspace-form" className="form-grid" onSubmit={onSubmit}>
        <label className="field-label">
          이름
          <input autoFocus placeholder="이름" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label className="field-label">
          Slug
          <input placeholder="slug (예: ai-news)" value={form.slug} onChange={(e) => setForm({ ...form, slug: e.target.value })} required />
        </label>
        <label className="field-label">
          이슈 Prefix
          <input
            placeholder="Prefix"
            value={form.identifier_prefix}
            onChange={(e) => setForm({ ...form, identifier_prefix: e.target.value.toUpperCase() })}
            required
          />
        </label>
        <label className="field-label">
          메인 에이전트 이름
          <input
            placeholder="메인 에이전트 이름"
            value={form.main_agent_name}
            onChange={(e) => setForm({ ...form, main_agent_name: e.target.value })}
            required
          />
        </label>
        <label className="field-label">
          Runtime
          <div className="runtime-select-row">
            <select value={form.runtime} onChange={(e) => setForm({ ...form, runtime: e.target.value })}>
              <option value="codex">codex</option>
              <option value="claude">claude</option>
              <option value="gemini">gemini</option>
            </select>
            <RuntimeBadge runtime={form.runtime} runtimes={settings.data?.available_runtimes} />
          </div>
        </label>
        <ModelSelect runtime={form.runtime} value={form.model} onChange={(model) => setForm({ ...form, model })} />
        <label className="field-label">
          에이전트 지시문
          <textarea
            placeholder="에이전트 지시문"
            value={form.instructions}
            onChange={(e) => setForm({ ...form, instructions: e.target.value })}
            required
          />
        </label>
        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={form.auto_chain_enabled}
            onChange={(e) => setForm({ ...form, auto_chain_enabled: e.target.checked })}
          />
          agent 결과 @mention 자동 체이닝 허용
        </label>
        <p className="form-helper">
          켜면 agent 결과 댓글의 첫 <code>@AgentName</code>이 자동으로 다음 run으로 dispatch됩니다. depth/24h run/24h cost/dry-run/main agent 재진입 면제 가드가 함께 적용됩니다. 기본값은 끔 — RFP·hub-PM 같이 한 이슈를 여러 agent가 순차 처리하는 워크플로우에 권장합니다. 사후에 Settings에서 변경할 수 있습니다.
        </p>
        {createWorkspace.isError && <p className="error-text">워크스페이스 생성에 실패했습니다.</p>}
      </form>
    </Modal>
  );
}
