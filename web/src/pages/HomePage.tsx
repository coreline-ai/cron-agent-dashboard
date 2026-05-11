import { Link } from 'react-router-dom';
import { PageHeader } from '../components/PageHeader';
import { useHealthQuery } from '../api/queries';

export function HomePage() {
  const health = useHealthQuery();

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="개요"
        title="혼자 쓰는 AI 에이전트 작업 트래커"
        description="이슈를 만들고, CLI 에이전트 실행 결과를 댓글로 추적하며, 정기 작업은 오토파일럿으로 자동화합니다."
      />
      <div className="hero-card">
        <div>
          <h2>오늘의 워크스페이스</h2>
          <p>샘플 워크스페이스에서 7개 핵심 화면 라우팅을 확인할 수 있습니다.</p>
        </div>
        <Link className="button" to="/w/news/board">
          보드 열기
        </Link>
      </div>
      <div className="status-grid">
        <article className="metric-card">
          <span>API 상태</span>
          <strong>{health.isLoading ? '확인 중' : health.isError ? '연결 대기' : '정상'}</strong>
          <p>백엔드 준비 전에는 연결 대기로 표시됩니다.</p>
        </article>
        <article className="metric-card">
          <span>라우트</span>
          <strong>7개</strong>
          <p>보드, 이슈 상세, 에이전트, 오토파일럿, 설정.</p>
        </article>
      </div>
    </section>
  );
}
