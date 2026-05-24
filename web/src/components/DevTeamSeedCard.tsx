import { FormEvent, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { apiClient } from '../api/client';
import { useToast } from './ToastProvider';

// DevTeamSeedCard exposes the CLI `seed-dev-team` action on the Settings
// page so an operator can provision the 7-role hub-PM workspace from the
// web without dropping to a terminal. The card mounts above the existing
// workspace timeout rows because new dev-team workspaces typically need
// no further config — the seed already wires auto_chain, per_run_worktree,
// max_depth=8 etc.
type SeedResponse = {
  workspace: { id: string; slug: string; name: string };
  agents: Array<{ name: string; runtime: string }>;
  skills: string[];
  assignment_count: number;
  created_agent_count: number;
  already_had: boolean;
};

export function DevTeamSeedCard() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const toast = useToast();
  const [slug, setSlug] = useState('ai-dev-team');
  const [workingDir, setWorkingDir] = useState('');

  const seed = useMutation({
    mutationFn: () =>
      apiClient.post<SeedResponse>('/system/seed-dev-team', {
        slug: slug.trim(),
        working_dir: workingDir.trim()
      }),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['workspace', data.workspace.slug] });
      const verb = data.already_had ? '재사용' : '생성';
      toast.success(`${data.workspace.name} ${verb} 완료`, {
        description: `agents=${data.agents.length}, skills=${data.skills.length}, assignments=${data.assignment_count}`
      });
      navigate(`/w/${data.workspace.slug}/board`);
    },
    onError: (err: unknown) => {
      toast.error('AI Dev Team 생성 실패', {
        description: err instanceof Error ? err.message : '알 수 없는 오류'
      });
    }
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!slug.trim()) {
      return;
    }
    seed.mutate();
  };

  return (
    <article className="panel settings-card dev-team-seed-card">
      <div>
        <h2>AI Dev Team 워크스페이스</h2>
        <p>
          PM / Designer / Backend / Frontend / DB / QA / DevOps 7개 역할 에이전트와 8개 스킬을 한 번에 생성합니다.
          hub-PM 패턴(<code>auto_chain_max_depth=8</code>), per-run worktree, auto_close=off로 미리 설정됩니다.
          이미 같은 slug가 있으면 새로 만들지 않고 그대로 재사용합니다.
        </p>
      </div>
      <form className="form-grid dev-team-seed-card__form" onSubmit={onSubmit}>
        <label className="field-label">
          Workspace slug
          <input
            type="text"
            required
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            placeholder="ai-dev-team"
          />
        </label>
        <label className="field-label">
          Working directory (비우면 서버 data_dir)
          <input
            type="text"
            value={workingDir}
            onChange={(e) => setWorkingDir(e.target.value)}
            placeholder="/Users/.../code/myapp"
          />
        </label>
        <button className="button" type="submit" disabled={seed.isPending || !slug.trim()}>
          {seed.isPending ? '생성 중' : 'AI Dev Team 생성'}
        </button>
      </form>
    </article>
  );
}
