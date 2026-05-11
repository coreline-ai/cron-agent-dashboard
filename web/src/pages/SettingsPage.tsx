import { PageHeader } from '../components/PageHeader';
import { useSettingsQuery } from '../api/queries';

export function SettingsPage() {
  const settings = useSettingsQuery();
  const data = settings.data;

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="전역"
        title="설정"
        description="토큰 인증, 기본 타임존, 로컬 데이터 경로와 런타임 탐지 상태를 확인합니다."
      />
      <article className="panel">
        <h2>서버 설정</h2>
        <dl className="detail-list">
          <dt>상태</dt>
          <dd>{settings.isLoading ? '확인 중' : settings.isError ? 'API 연결 대기' : '연결됨'}</dd>
          <dt>버전</dt>
          <dd>{data?.version ?? '-'}</dd>
          <dt>데이터 디렉토리</dt>
          <dd>{data?.data_dir ?? '-'}</dd>
          <dt>테마</dt>
          <dd>다크 모드</dd>
          <dt>타임존</dt>
          <dd>{data?.timezone ?? 'Asia/Seoul'}</dd>
          <dt>Worker</dt>
          <dd>{data?.worker_pool_size ?? 3}</dd>
          <dt>인증</dt>
          <dd>{data?.auth_mode ?? 'none'}</dd>
          <dt>런타임</dt>
          <dd>
            {data?.available_runtimes?.length
              ? data.available_runtimes.map((runtime) => runtime.name).join(', ')
              : 'PATH에서 탐지된 런타임 없음'}
          </dd>
        </dl>
      </article>
    </section>
  );
}
