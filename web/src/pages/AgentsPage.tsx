import { useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { CreateAgentDialog } from '../components/CreateAgentDialog';
import { PageHeader } from '../components/PageHeader';
import { useAgentsQuery } from '../api/queries';

type AgentFilter = 'all' | 'main' | 'codex' | 'claude' | 'gemini';

const filters: Array<{ value: AgentFilter; label: string }> = [
  { value: 'all', label: '전체' },
  { value: 'main', label: 'Main' },
  { value: 'codex', label: 'codex' },
  { value: 'claude', label: 'claude' },
  { value: 'gemini', label: 'gemini' }
];

export function AgentsPage() {
  const { slug } = useParams();
  const agents = useAgentsQuery(slug);
  const [filter, setFilter] = useState<AgentFilter>('all');
  const [query, setQuery] = useState('');
  const [createOpen, setCreateOpen] = useState(false);

  const visibleAgents = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (agents.data ?? []).filter((agent) => {
      if (filter === 'main' && !agent.is_main) {
        return false;
      }
      if (filter !== 'all' && filter !== 'main' && agent.runtime !== filter) {
        return false;
      }
      if (!q) {
        return true;
      }
      return [agent.name, agent.runtime, agent.model || '기본', agent.instructions]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(q));
    });
  }, [agents.data, filter, query]);

  const counts = useMemo(() => {
    const allAgents = agents.data ?? [];
    return {
      all: allAgents.length,
      main: allAgents.filter((agent) => agent.is_main).length,
      codex: allAgents.filter((agent) => agent.runtime === 'codex').length,
      claude: allAgents.filter((agent) => agent.runtime === 'claude').length,
      gemini: allAgents.filter((agent) => agent.runtime === 'gemini').length
    };
  }, [agents.data]);

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="에이전트"
        description="CLI 에이전트를 테이블로 관리하고 runtime/역할 기준으로 빠르게 필터링합니다."
      />

      <div className="board-toolbar panel">
        <div className="toolbar-main">
          <div>
            <h2>에이전트 목록</h2>
            <p>
              전체 {counts.all} · main {counts.main} · codex {counts.codex} · claude {counts.claude} · gemini {counts.gemini}
            </p>
          </div>
          <button className="button" type="button" onClick={() => setCreateOpen(true)}>
            에이전트 추가
          </button>
        </div>
        <div className="toolbar-controls">
          <div className="segmented" role="tablist" aria-label="에이전트 필터">
            {filters.map((item) => (
              <button key={item.value} type="button" className={filter === item.value ? 'active' : ''} onClick={() => setFilter(item.value)}>
                {item.label}
              </button>
            ))}
          </div>
          <input className="toolbar-search" placeholder="에이전트 검색" value={query} onChange={(event) => setQuery(event.target.value)} />
        </div>
      </div>

      <article className="panel table-panel">
        <div className="data-table agent-table">
          <div className="data-row data-head">
            <span>Agent</span>
            <span>Runtime</span>
            <span>Model</span>
            <span>Role</span>
            <span>Instructions</span>
          </div>
          {visibleAgents.map((agent) => (
            <Link className="data-row" key={agent.id} to={`/w/${slug}/agents/${agent.id}`}>
              <span>
                <span className="agent-avatar">@</span>
                <strong>{agent.name}</strong>
              </span>
              <span>{agent.runtime}</span>
              <span>{agent.model || '기본'}</span>
              <span>{agent.is_main ? <span className="badge">main</span> : 'worker'}</span>
              <span className="truncate">{agent.instructions || '-'}</span>
            </Link>
          ))}
        </div>
        {!agents.isLoading && !visibleAgents.length && (
          <div className="empty-state compact">
            <h2>{agents.isError ? '에이전트 로드 실패' : '표시할 에이전트 없음'}</h2>
            <p>{agents.isError ? 'API 연결 또는 토큰을 확인하세요.' : '에이전트 추가 버튼으로 새 작업자를 등록하세요.'}</p>
          </div>
        )}
      </article>

      <CreateAgentDialog open={createOpen} slug={slug} onClose={() => setCreateOpen(false)} />
    </section>
  );
}
