import { Link, useOutletContext } from 'react-router-dom';
import { AuthTokenPanel, isUnauthorizedError } from '../components/AuthTokenPanel';
import { PageHeader } from '../components/PageHeader';
import { useHealthQuery, useIssuesQuery } from '../api/queries';
import type { DashboardOutletContext } from '../layouts/DashboardLayout';

export function HomePage() {
  const health = useHealthQuery();
  const { currentWorkspace, workspaces, isWorkspaceLoading, workspaceError, openCreateWorkspace } = useOutletContext<DashboardOutletContext>();
  const currentWorkspaceIssues = useIssuesQuery(currentWorkspace?.slug);
  const totalOpenIssues = workspaces.reduce((sum, workspace) => sum + (workspace.open_issue_count ?? 0), 0);
  const activeIssueCount =
    currentWorkspaceIssues.data?.filter((issue) => issue.execution_status === 'queued' || issue.execution_status === 'running').length ?? 0;
  const recentIssues = currentWorkspaceIssues.data?.slice(0, 5) ?? [];
  const runtimeCount = health.data?.available_runtimes?.length ?? 0;

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="대시보드"
        title="운영 현황"
        description="로컬 워크스페이스, 실행 중인 agent 작업, 런타임 상태를 한 화면에서 확인합니다."
      />

      <div className="dashboard-hero">
        <div className="dashboard-hero-copy">
          <span className="issue-id">LOCAL</span>
          <h2>{currentWorkspace ? `${currentWorkspace.name} 작업 허브` : '새 워크스페이스를 만들고 시작하세요'}</h2>
          <p>
            {currentWorkspace
              ? `현재 선택된 워크스페이스입니다. 열린 이슈 ${currentWorkspace.open_issue_count ?? 0}개와 에이전트 ${
                  currentWorkspace.agent_count ?? 0
                }개를 추적 중입니다.`
              : 'Corn Agent Dashboard는 개인용 AI 작업 보드입니다. 워크스페이스를 만들면 이슈, 에이전트, 오토파일럿이 연결됩니다.'}
          </p>
        </div>
        <div className="button-row">
          {currentWorkspace ? (
            <Link className="button" to={`/w/${currentWorkspace.slug}/board`}>
              보드 열기
            </Link>
          ) : (
            <button className="button" type="button" onClick={openCreateWorkspace}>
              워크스페이스 생성
            </button>
          )}
          <button className="button secondary" type="button" onClick={openCreateWorkspace}>
            새 워크스페이스
          </button>
          <Link className="button secondary" to="/settings">
            설정 보기
          </Link>
        </div>
      </div>

      <div className="dashboard-metrics">
        <article className="metric-card dashboard-metric">
          <span>API 상태</span>
          <strong>{health.isLoading ? '확인 중' : health.isError ? '연결 대기' : '정상'}</strong>
          <p>{health.data?.uptime_seconds ? `uptime ${health.data.uptime_seconds}s` : 'startup self-check 기반'}</p>
        </article>
        <article className="metric-card dashboard-metric">
          <span>워크스페이스</span>
          <strong>{workspaces.length}개</strong>
          <p>{isWorkspaceLoading ? '목록 확인 중' : `열린 이슈 ${totalOpenIssues}개`}</p>
        </article>
        <article className="metric-card dashboard-metric">
          <span>실행 대기/진행</span>
          <strong>{activeIssueCount}개</strong>
          <p>{currentWorkspace ? `${currentWorkspace.name} 기준` : '워크스페이스 생성 후 표시'}</p>
        </article>
        <article className="metric-card dashboard-metric">
          <span>런타임</span>
          <strong>{runtimeCount}개</strong>
          <p>{health.data?.available_runtimes?.join(', ') || 'PATH 탐지 대기'}</p>
        </article>
      </div>
      {isUnauthorizedError(workspaceError) && <AuthTokenPanel error={workspaceError} />}

      <div className="dashboard-grid">
        <article className="panel dashboard-main">
          <div className="section-heading">
            <div>
              <h2>{currentWorkspace ? `${currentWorkspace.name} 최근 이슈` : '최근 이슈'}</h2>
              <p>사이드바의 워크스페이스 선택기를 바꾸면 이 영역도 함께 바뀝니다.</p>
            </div>
            {currentWorkspace && (
              <Link className="inline-link" to={`/w/${currentWorkspace.slug}/board`}>
                전체 보기
              </Link>
            )}
          </div>
          {recentIssues.length > 0 ? (
            <div className="dashboard-issue-list">
              {recentIssues.map((issue) => (
                <Link className="dashboard-issue-row" key={issue.id} to={`/w/${currentWorkspace?.slug}/issues/${issue.identifier}`}>
                  <span className="issue-id">{issue.identifier}</span>
                  <strong>{issue.title}</strong>
                  <span>{issue.status} · {issue.execution_status} · @{issue.last_run_agent_name || issue.assignee_agent_name || '-'}</span>
                </Link>
              ))}
            </div>
          ) : (
            <div className="empty-state">
              <h2>{currentWorkspaceIssues.isLoading ? '이슈를 불러오는 중' : '최근 이슈 없음'}</h2>
              <p>{currentWorkspace ? '보드에서 첫 이슈를 만들면 여기에 표시됩니다.' : '워크스페이스를 먼저 생성하세요.'}</p>
              {!currentWorkspace && (
                <button className="button" type="button" onClick={openCreateWorkspace}>
                  워크스페이스 생성
                </button>
              )}
            </div>
          )}
        </article>

        <aside className="dashboard-side">
          <article className="panel">
            <div className="section-heading">
              <h2>빠른 액션</h2>
            </div>
            <div className="quick-actions">
              {currentWorkspace ? (
                <>
                  <Link className="quick-action" to={`/w/${currentWorkspace.slug}/board`}>
                    <strong>이슈 보드</strong>
                    <span>작업 생성/상태 추적</span>
                  </Link>
                  <Link className="quick-action" to={`/w/${currentWorkspace.slug}/agents`}>
                    <strong>에이전트</strong>
                    <span>CLI runtime 역할 관리</span>
                  </Link>
                  <Link className="quick-action" to={`/w/${currentWorkspace.slug}/autopilot`}>
                    <strong>오토파일럿</strong>
                    <span>cron 정기 작업</span>
                  </Link>
                </>
              ) : (
                <button className="quick-action" type="button" onClick={openCreateWorkspace}>
                  <strong>워크스페이스 생성</strong>
                  <span>보드, 에이전트, 오토파일럿 시작</span>
                </button>
              )}
            </div>
          </article>

          <article className="panel workspace-summary-card">
            <div className="section-heading">
              <div>
                <h2>선택된 워크스페이스</h2>
                <p>워크스페이스 변경은 좌측 상단 선택기에서 합니다.</p>
              </div>
            </div>
            {currentWorkspace ? (
              <div className="workspace-summary">
                <span className="issue-id">{currentWorkspace.identifier_prefix}</span>
                <strong>{currentWorkspace.name}</strong>
                <p>{currentWorkspace.description || '설명 없음'}</p>
                <div className="workspace-summary-stats">
                  <span>open {currentWorkspace.open_issue_count ?? 0}</span>
                  <span>agents {currentWorkspace.agent_count ?? 0}</span>
                  <span>total {workspaces.length}</span>
                </div>
                <button className="button secondary" type="button" onClick={openCreateWorkspace}>
                  새 워크스페이스
                </button>
              </div>
            ) : (
              <div className="empty-state compact">
                <h2>워크스페이스 없음</h2>
                <p>좌측 선택기 또는 아래 버튼으로 새 워크스페이스를 만드세요.</p>
                <button className="button" type="button" onClick={openCreateWorkspace}>
                  워크스페이스 생성
                </button>
              </div>
            )}
          </article>
        </aside>
      </div>
    </section>
  );
}
