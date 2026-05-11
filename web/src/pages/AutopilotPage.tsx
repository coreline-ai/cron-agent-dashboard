import { useParams } from 'react-router-dom';
import { PageHeader } from '../components/PageHeader';

export function AutopilotPage() {
  const { slug } = useParams();

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="오토파일럿"
        description="cron 기반 정기 이슈 생성 규칙을 관리하는 화면입니다."
      />
      <article className="panel autopilot-row">
        <div>
          <strong>매일 09:00</strong>
          <p>NewsLead에게 오늘 뉴스 정리를 요청합니다.</p>
        </div>
        <span className="badge">ON</span>
      </article>
    </section>
  );
}
