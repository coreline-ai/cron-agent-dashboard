import { Link, useParams } from 'react-router-dom';
import { PageHeader } from '../components/PageHeader';

const sampleIssues = [
  { id: 'NEWS-15', title: '오늘 뉴스 정리', status: '실행 중', agent: 'NewsLead' },
  { id: 'NEWS-14', title: '주말 모아보기', status: '완료', agent: 'Publisher' },
  { id: 'NEWS-13', title: '어제 실패한 작업', status: '실패', agent: 'NewsLead' }
];

export function BoardPage() {
  const { slug } = useParams();

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="이슈 보드"
        description="작업 이슈와 담당 에이전트 상태를 한 화면에서 추적합니다."
      />
      <div className="issue-list">
        {sampleIssues.map((issue) => (
          <Link className="issue-card" key={issue.id} to={`/w/${slug}/issues/${issue.id}`}>
            <span className="issue-id">{issue.id}</span>
            <strong>{issue.title}</strong>
            <span>{issue.status} · @{issue.agent}</span>
          </Link>
        ))}
      </div>
    </section>
  );
}
