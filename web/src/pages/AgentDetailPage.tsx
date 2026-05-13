import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { useAgentQuery } from '../api/queries';
import { ModelSelect } from '../components/ModelSelect';
import { PageHeader } from '../components/PageHeader';

export function AgentDetailPage() {
  const { id, slug } = useParams();
  const navigate = useNavigate();
  const agent = useAgentQuery(id);
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ name: '', runtime: 'codex', model: '', instructions: '' });
  useEffect(() => {
    if (agent.data) {
      setForm({
        name: agent.data.name,
        runtime: agent.data.runtime,
        model: agent.data.model ?? '',
        instructions: agent.data.instructions
      });
    }
  }, [agent.data]);
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['agent', id] });
    queryClient.invalidateQueries({ queryKey: ['agents', slug] });
  };
  const update = useMutation({ mutationFn: () => apiClient.put(`/agents/${id}`, form), onSuccess: invalidate });
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
        <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        <select value={form.runtime} onChange={(e) => setForm({ ...form, runtime: e.target.value })}>
          <option value="codex">codex</option>
          <option value="claude">claude</option>
          <option value="gemini">gemini</option>
        </select>
        <ModelSelect runtime={form.runtime} value={form.model} onChange={(model) => setForm({ ...form, model })} disabled={!agent.data} />
        <textarea value={form.instructions} onChange={(e) => setForm({ ...form, instructions: e.target.value })} required />
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
