import { Link, useParams } from 'react-router-dom';
import { PageHeader } from '../components/PageHeader';

const agents = [
  { id: 'newslead', name: 'NewsLead', role: '뉴스 수집과 큐레이션' },
  { id: 'publisher', name: 'Publisher', role: '발행용 문장 다듬기' }
];

export function AgentsPage() {
  const { slug } = useParams();

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="에이전트"
        description="작업을 수행할 CLI 에이전트와 기본 역할을 관리합니다."
      />
      <div className="status-grid">
        {agents.map((agent) => (
          <Link className="metric-card link-card" key={agent.id} to={`/w/${slug}/agents/${agent.id}`}>
            <span>@{agent.name}</span>
            <strong>{agent.role}</strong>
            <p>상세 설정 보기</p>
          </Link>
        ))}
      </div>
    </section>
  );
}
