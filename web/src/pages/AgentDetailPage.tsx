import { useParams } from 'react-router-dom';
import { PageHeader } from '../components/PageHeader';

export function AgentDetailPage() {
  const { id, slug } = useParams();

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`${slug} / agents`}
        title={`@${id}`}
        description="에이전트 실행 명령, 작업 디렉터리, 최근 실행 상태를 표시할 예정입니다."
      />
      <article className="panel">
        <h2>설정 스켈레톤</h2>
        <dl className="detail-list">
          <dt>실행기</dt>
          <dd>codex / claude / gemini</dd>
          <dt>상태</dt>
          <dd>대기 중</dd>
        </dl>
      </article>
    </section>
  );
}
