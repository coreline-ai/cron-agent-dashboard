import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { useAgentQuery } from '../api/queries';
import { ModelSelect } from '../components/ModelSelect';
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
  const retry_on = [];
  if (form.retry_on_timeout) retry_on.push('timeout');
  if (form.retry_on_executor_error) retry_on.push('executor_error');
  const backoff_seconds = form.backoff_seconds
    .split(',')
    .map((value) => Number(value.trim()))
    .filter((value) => Number.isFinite(value) && value > 0);
  return JSON.stringify({
    max_attempts: Number(form.max_attempts) || 1,
    ...(backoff_seconds.length ? { backoff_seconds } : {}),
    ...(retry_on.length ? { retry_on } : {})
  });
}

export function AgentDetailPage() {
  const { id, slug } = useParams();
  const navigate = useNavigate();
  const agent = useAgentQuery(id);
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: '', runtime: 'codex', model: '', summary: '', tags: '', instructions: '', timeout_seconds_override: '', max_attempts: '1', backoff_seconds: '', retry_on_timeout: true, retry_on_executor_error: true });
  useEffect(() => {
    if (agent.data) {
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
    }
  }, [agent.data]);
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['agent', id] });
    queryClient.invalidateQueries({ queryKey: ['agents', slug] });
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
    onSuccess: invalidate
  });
  const promote = useMutation({ mutationFn: () => apiClient.post(`/agents/${id}/promote`), onSuccess: invalidate });
  const remove = useMutation({
    mutationFn: () => apiClient.delete(`/agents/${id}`),
    onSuccess: () => navigate(`/w/${slug}/agents`)
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    update.mutate();
  };

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`${slug} / agents`}
        title={`@${agent.data?.name ?? id}`}
        description={agent.data?.is_main ? '메인 에이전트' : '추가 에이전트'}
      />
      <form className="panel form-grid" onSubmit={onSubmit}>
        <h2>에이전트 설정</h2>
        <label className="field-label">
          이름
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label className="field-label">
          런타임
          <select value={form.runtime} onChange={(e) => setForm({ ...form, runtime: e.target.value })}>
            <option value="codex">codex</option>
            <option value="claude">claude</option>
            <option value="gemini">gemini</option>
          </select>
        </label>
        <ModelSelect runtime={form.runtime} value={form.model} onChange={(model) => setForm({ ...form, model })} disabled={!agent.data} />
        <label className="field-label">
          한 줄 요약
          <input value={form.summary} onChange={(e) => setForm({ ...form, summary: e.target.value })} placeholder="에이전트 역할 요약" />
        </label>
        <label className="field-label">
          태그
          <input value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} placeholder="research,writing,korean" />
        </label>
        <label className="field-label">
          실행 timeout override (초)
          <input
            min="1"
            max="86400"
            placeholder="비우면 워크스페이스 기본값 사용"
            type="number"
            value={form.timeout_seconds_override}
            onChange={(e) => setForm({ ...form, timeout_seconds_override: e.target.value })}
          />
        </label>
        <label className="field-label">
          최대 재시도 횟수
          <input min="1" max="5" type="number" value={form.max_attempts} onChange={(e) => setForm({ ...form, max_attempts: e.target.value })} />
          <span className="field-help">timeout / executor 오류만 자동 재시도합니다. exit_nonzero와 worker_panic은 재시도하지 않습니다.</span>
        </label>
        <label className="field-label">
          재시도 backoff 초
          <input placeholder="예: 10,60,300" value={form.backoff_seconds} onChange={(e) => setForm({ ...form, backoff_seconds: e.target.value })} />
          <span className="field-help">비우면 기본값 10초 → 60초 → 5분을 사용합니다.</span>
        </label>
        <div className="checkbox-row">
          <label><input type="checkbox" checked={form.retry_on_timeout} onChange={(e) => setForm({ ...form, retry_on_timeout: e.target.checked })} /> timeout 재시도</label>
          <label><input type="checkbox" checked={form.retry_on_executor_error} onChange={(e) => setForm({ ...form, retry_on_executor_error: e.target.checked })} /> executor_error 재시도</label>
        </div>
        <label className="field-label">
          Instructions
          <textarea value={form.instructions} onChange={(e) => setForm({ ...form, instructions: e.target.value })} required />
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
    </section>
  );
}
