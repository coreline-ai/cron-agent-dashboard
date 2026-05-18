import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { type Skill, useAgentInstructionVersionsQuery, useAgentQuery, useAgentSkillsQuery, useWorkspaceSkillsQuery } from '../api/queries';
import { DateTimeText } from '../components/DateTimeText';
import { ModelSelect } from '../components/ModelSelect';
import { MutationErrorAlert } from '../components/MutationErrorAlert';
import { PageHeader } from '../components/PageHeader';

function retryPolicyFromJSON(value?: string) {
  const fallback = { max_attempts: '1', backoff_seconds: '', retry_on_timeout: true, retry_on_executor_error: true };
  if (!value) {
    return fallback;
  }
  try {
    const parsed = JSON.parse(value) as { max_attempts?: number; backoff_seconds?: number[]; retry_on?: string[] };
    const retryOn = parsed.retry_on ?? ['timeout', 'executor_error'];
    return {
      max_attempts: String(parsed.max_attempts ?? 1),
      backoff_seconds: parsed.backoff_seconds?.join(',') ?? '',
      retry_on_timeout: retryOn.includes('timeout'),
      retry_on_executor_error: retryOn.includes('executor_error')
    };
  } catch {
    return fallback;
  }
}

function retryPolicyJSON(form: { max_attempts: string; backoff_seconds: string; retry_on_timeout: boolean; retry_on_executor_error: boolean }) {
  const retry_on: string[] = [];
  if (form.retry_on_timeout) retry_on.push('timeout');
  if (form.retry_on_executor_error) retry_on.push('executor_error');
  const backoff_seconds = form.backoff_seconds
    .split(',')
    .map((value) => Number(value.trim()))
    .filter((value) => Number.isFinite(value) && value > 0);
  return JSON.stringify({
    max_attempts: Number(form.max_attempts) || 1,
    ...(backoff_seconds.length ? { backoff_seconds } : {}),
    retry_on
  });
}

export function AgentDetailPage() {
  const { id, slug } = useParams();
  const navigate = useNavigate();
  const agent = useAgentQuery(id);
  const instructionVersions = useAgentInstructionVersionsQuery(id);
  const workspaceSkills = useWorkspaceSkillsQuery(slug);
  const agentSkills = useAgentSkillsQuery(id);
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: '', runtime: 'codex', model: '', summary: '', tags: '', instructions: '', timeout_seconds_override: '', max_attempts: '1', backoff_seconds: '', retry_on_timeout: true, retry_on_executor_error: true });
  const [skillForm, setSkillForm] = useState({ name: '', description: '', triggers: '', content: '' });
  const [assignmentForm, setAssignmentForm] = useState({ skill_id: '', activation_mode: 'trigger', priority: '100' });
  const [dirty, setDirty] = useState(false);
  const [loadedAgentID, setLoadedAgentID] = useState<string | undefined>();
  useEffect(() => {
    if (!agent.data) {
      return;
    }
    const agentChanged = loadedAgentID !== agent.data.id;
    if (!agentChanged && dirty) {
      return;
    }
    setForm({
      name: agent.data.name,
      runtime: agent.data.runtime,
      model: agent.data.model ?? '',
      summary: agent.data.summary ?? '',
      tags: agent.data.tags ?? '',
      instructions: agent.data.instructions,
      timeout_seconds_override: agent.data.timeout_seconds_override ? String(agent.data.timeout_seconds_override) : '',
      ...retryPolicyFromJSON(agent.data.retry_policy_json)
    });
    setLoadedAgentID(agent.data.id);
    setDirty(false);
  }, [agent.data, dirty, loadedAgentID]);
  const updateForm = (patch: Partial<typeof form>) => {
    setDirty(true);
    setForm((current) => ({ ...current, ...patch }));
  };
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['agent', id] });
    queryClient.invalidateQueries({ queryKey: ['agent-instructions', id] });
    queryClient.invalidateQueries({ queryKey: ['agents', slug] });
  };
  const invalidateSkills = () => {
    queryClient.invalidateQueries({ queryKey: ['workspace-skills', slug] });
    queryClient.invalidateQueries({ queryKey: ['agent-skills', id] });
  };
  const update = useMutation({
    mutationFn: () =>
      apiClient.put(`/agents/${id}`, {
        name: form.name,
        runtime: form.runtime,
        model: form.model,
        summary: form.summary,
        tags: form.tags,
        instructions: form.instructions,
        timeout_seconds_override: form.timeout_seconds_override.trim() ? Number(form.timeout_seconds_override) : null,
        retry_policy_json: retryPolicyJSON(form)
      }),
    onSuccess: () => {
      setDirty(false);
      invalidate();
    }
  });
  const promote = useMutation({ mutationFn: () => apiClient.post(`/agents/${id}/promote`), onSuccess: invalidate });
  const remove = useMutation({
    mutationFn: () => apiClient.delete(`/agents/${id}`),
    onSuccess: () => navigate(`/w/${slug}/agents`)
  });
  const createSkill = useMutation({
    mutationFn: () =>
      apiClient.post<{ skill: Skill }>(`/workspaces/${slug}/skills`, {
        name: skillForm.name,
        description: skillForm.description,
        triggers: skillForm.triggers
          .split(',')
          .map((value) => value.trim())
          .filter(Boolean),
        content: skillForm.content,
        source_type: 'manual',
        trust_level: 'local'
      }),
    onSuccess: ({ skill }) => {
      setSkillForm({ name: '', description: '', triggers: '', content: '' });
      setAssignmentForm((current) => ({ ...current, skill_id: current.skill_id || skill.id }));
      invalidateSkills();
    }
  });
  const assignSkill = useMutation({
    mutationFn: () =>
      apiClient.post(`/agents/${id}/skills`, {
        skill_id: assignmentForm.skill_id,
        activation_mode: assignmentForm.activation_mode,
        priority: Number(assignmentForm.priority) || 100
      }),
    onSuccess: invalidateSkills
  });
  const unassignSkill = useMutation({
    mutationFn: (skillID: string) => apiClient.delete(`/agents/${id}/skills/${skillID}`),
    onSuccess: invalidateSkills
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    update.mutate();
  };
  const onCreateSkill = (event: FormEvent) => {
    event.preventDefault();
    createSkill.mutate();
  };
  const onAssignSkill = (event: FormEvent) => {
    event.preventDefault();
    assignSkill.mutate();
  };

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`${slug} / agents`}
        title={`@${agent.data?.name ?? id}`}
        description={`${agent.data?.is_main ? '메인 에이전트' : '추가 에이전트'} · instructions v${agent.data?.instructions_version ?? 1}`}
      />
      <form className="panel form-grid" onSubmit={onSubmit}>
        <h2>에이전트 설정</h2>
        <label className="field-label">
          이름
          <input value={form.name} onChange={(e) => updateForm({ name: e.target.value })} required />
        </label>
        <label className="field-label">
          런타임
          <select value={form.runtime} onChange={(e) => updateForm({ runtime: e.target.value })}>
            <option value="codex">codex</option>
            <option value="claude">claude</option>
            <option value="gemini">gemini</option>
          </select>
        </label>
        <ModelSelect runtime={form.runtime} value={form.model} onChange={(model) => updateForm({ model })} disabled={!agent.data} />
        <label className="field-label">
          한 줄 요약
          <input value={form.summary} onChange={(e) => updateForm({ summary: e.target.value })} placeholder="에이전트 역할 요약" />
        </label>
        <label className="field-label">
          태그
          <input value={form.tags} onChange={(e) => updateForm({ tags: e.target.value })} placeholder="research,writing,korean" />
        </label>
        <label className="field-label">
          실행 timeout override (초)
          <input
            min="1"
            max="86400"
            placeholder="비우면 워크스페이스 기본값 사용"
            type="number"
            value={form.timeout_seconds_override}
            onChange={(e) => updateForm({ timeout_seconds_override: e.target.value })}
          />
        </label>
        <label className="field-label">
          최대 재시도 횟수
          <input min="1" max="5" type="number" value={form.max_attempts} onChange={(e) => updateForm({ max_attempts: e.target.value })} />
          <span className="field-help">timeout / executor 오류만 자동 재시도합니다. exit_nonzero와 worker_panic은 재시도하지 않습니다.</span>
        </label>
        <label className="field-label">
          재시도 backoff 초
          <input placeholder="예: 10,60,300" value={form.backoff_seconds} onChange={(e) => updateForm({ backoff_seconds: e.target.value })} />
          <span className="field-help">비우면 기본값 10초 → 60초 → 5분을 사용합니다.</span>
        </label>
        <div className="checkbox-row">
          <label><input type="checkbox" checked={form.retry_on_timeout} onChange={(e) => updateForm({ retry_on_timeout: e.target.checked })} /> timeout 재시도</label>
          <label><input type="checkbox" checked={form.retry_on_executor_error} onChange={(e) => updateForm({ retry_on_executor_error: e.target.checked })} /> executor_error 재시도</label>
        </div>
        <label className="field-label">
          Instructions
          <textarea value={form.instructions} onChange={(e) => updateForm({ instructions: e.target.value })} required />
        </label>
        {update.isError && <p className="error-text">저장 실패: {update.error instanceof Error ? update.error.message : '알 수 없는 오류'}</p>}
        <div className="button-row">
          <button className="button" type="submit" disabled={update.isPending || !agent.data}>
            저장
          </button>
          <button className="button secondary" type="button" onClick={() => promote.mutate()} disabled={!agent.data || agent.data.is_main}>
            메인으로 승격
          </button>
          <button className="button danger" type="button" onClick={() => remove.mutate()} disabled={!agent.data || agent.data.is_main}>
            삭제
          </button>
        </div>
      </form>
      <section className="panel instruction-history-panel">
        <div>
          <h2>Instructions 버전 이력</h2>
          <p className="muted-copy">에이전트 지시문이 변경될 때마다 버전을 남겨, 과거 run이 어떤 지시문 기준으로 실행됐는지 추적합니다.</p>
        </div>
        {instructionVersions.isError ? <MutationErrorAlert error={instructionVersions.error} title="Instructions 이력 로드 실패" /> : null}
        {instructionVersions.isLoading ? <p className="muted-copy">버전 이력을 불러오는 중입니다.</p> : null}
        <div className="instruction-history-list">
          {instructionVersions.data?.map((version) => (
            <details key={version.id} className="instruction-history-card">
              <summary>
                <strong>v{version.version}</strong>
                <span>
                  <DateTimeText value={version.created_at} mode="both" />
                </span>
              </summary>
              <pre>{version.instructions}</pre>
            </details>
          ))}
        </div>
        {!instructionVersions.isLoading && !instructionVersions.data?.length ? <p className="muted-copy">아직 저장된 instructions 버전이 없습니다.</p> : null}
      </section>
      <section className="panel instruction-history-panel">
        <div>
          <h2>Agent Skills</h2>
          <p className="muted-copy">SKILL.md 호환 지침을 워크스페이스 registry에 저장하고, 이 에이전트에 always / trigger / manual 방식으로 할당합니다. 스크립트는 실행하지 않고 prompt context로만 주입됩니다.</p>
        </div>
        {workspaceSkills.isError ? <MutationErrorAlert error={workspaceSkills.error} title="Skill 목록 로드 실패" /> : null}
        {agentSkills.isError ? <MutationErrorAlert error={agentSkills.error} title="에이전트 Skill 로드 실패" /> : null}
        {createSkill.isError ? <MutationErrorAlert error={createSkill.error} title="Skill 생성 실패" /> : null}
        {assignSkill.isError ? <MutationErrorAlert error={assignSkill.error} title="Skill 할당 실패" /> : null}
        {unassignSkill.isError ? <MutationErrorAlert error={unassignSkill.error} title="Skill 해제 실패" /> : null}

        <div className="instruction-history-list">
          {agentSkills.data?.map((assignment) => (
            <div key={assignment.skill_id} className="instruction-history-card">
              <div className="button-row split-row">
                <div>
                  <strong>{assignment.skill?.name ?? assignment.skill_id}</strong>
                  <p className="muted-copy">
                    {assignment.activation_mode} · priority {assignment.priority} · {assignment.enabled ? 'enabled' : 'disabled'}
                  </p>
                  <p className="muted-copy">{assignment.skill?.description}</p>
                </div>
                <button className="button secondary" type="button" onClick={() => unassignSkill.mutate(assignment.skill_id)} disabled={unassignSkill.isPending}>
                  해제
                </button>
              </div>
            </div>
          ))}
          {!agentSkills.isLoading && !agentSkills.data?.length ? <p className="muted-copy">아직 할당된 skill이 없습니다.</p> : null}
        </div>

        <form className="form-grid compact-form" onSubmit={onAssignSkill}>
          <h3>기존 Skill 할당</h3>
          <label className="field-label">
            Skill
            <select value={assignmentForm.skill_id} onChange={(e) => setAssignmentForm((current) => ({ ...current, skill_id: e.target.value }))} required>
              <option value="">선택</option>
              {workspaceSkills.data?.map((skill) => (
                <option key={skill.id} value={skill.id}>
                  {skill.name}
                </option>
              ))}
            </select>
          </label>
          <label className="field-label">
            Activation
            <select value={assignmentForm.activation_mode} onChange={(e) => setAssignmentForm((current) => ({ ...current, activation_mode: e.target.value }))}>
              <option value="trigger">trigger — 내용과 trigger keyword가 맞을 때</option>
              <option value="always">always — 모든 run에 포함</option>
              <option value="manual">manual — #skills: name 지정 시</option>
            </select>
          </label>
          <label className="field-label">
            Priority
            <input type="number" value={assignmentForm.priority} onChange={(e) => setAssignmentForm((current) => ({ ...current, priority: e.target.value }))} />
          </label>
          <button className="button" type="submit" disabled={!assignmentForm.skill_id || assignSkill.isPending}>
            할당
          </button>
        </form>

        <form className="form-grid compact-form" onSubmit={onCreateSkill}>
          <h3>새 Skill 등록</h3>
          <label className="field-label">
            이름
            <input value={skillForm.name} onChange={(e) => setSkillForm((current) => ({ ...current, name: e.target.value }))} placeholder="reddit-ai-brief" required />
          </label>
          <label className="field-label">
            설명
            <input value={skillForm.description} onChange={(e) => setSkillForm((current) => ({ ...current, description: e.target.value }))} placeholder="Reddit AI 논의 요약 작성 방식" required />
          </label>
          <label className="field-label">
            Triggers
            <input value={skillForm.triggers} onChange={(e) => setSkillForm((current) => ({ ...current, triggers: e.target.value }))} placeholder="reddit, ai, community" />
          </label>
          <label className="field-label">
            Skill content
            <textarea value={skillForm.content} onChange={(e) => setSkillForm((current) => ({ ...current, content: e.target.value }))} placeholder="이 skill의 재사용 지침을 작성하세요." required />
          </label>
          <button className="button" type="submit" disabled={createSkill.isPending}>
            Skill 등록
          </button>
        </form>
      </section>
    </section>
  );
}
