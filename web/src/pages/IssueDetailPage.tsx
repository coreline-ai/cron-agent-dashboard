import { useParams } from 'react-router-dom';
import { MarkdownText } from '../components/MarkdownText';
import { PageHeader } from '../components/PageHeader';

const sampleComment = `@Writer 아래 요약을 바탕으로 블로그 초안을 작성해줘.\n\n- 원문 링크 수집\n- 핵심 문장 3개 정리\n- 한국어 톤으로 발행 준비`;

export function IssueDetailPage() {
  const { identifier, slug } = useParams();

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`${slug} / ${identifier}`}
        title="이슈 상세"
        description="실행 로그, 댓글, 멘션 위임 흐름을 안전한 텍스트 렌더링으로 표시합니다."
      />
      <article className="panel">
        <h2>댓글 스레드</h2>
        <MarkdownText value={sampleComment} />
      </article>
    </section>
  );
}
