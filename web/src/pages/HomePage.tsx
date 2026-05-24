import { useMemo, useState } from 'react';
import { Link, useOutletContext } from 'react-router-dom';
import { AuthTokenPanel, isUnauthorizedError } from '../components/AuthTokenPanel';
import { PageHeader } from '../components/PageHeader';
import { TeamPulseWidget } from '../components/TeamPulseWidget';
import { useHealthQuery, useIssuesQuery } from '../api/queries';
import type { DashboardOutletContext } from '../layouts/DashboardLayout';

export function HomePage() {
  const health = useHealthQuery();
  const { currentWorkspace, workspaces, isWorkspaceLoading, workspaceError, openCreateWorkspace } = useOutletContext<DashboardOutletContext>();
  const currentWorkspaceIssues = useIssuesQuery(currentWorkspace?.slug);
  const totalOpenIssues = workspaces.reduce((sum, workspace) => sum + (workspace.open_issue_count ?? 0), 0);
  const activeIssueCount =
    currentWorkspaceIssues.data?.filter((issue) => issue.execution_status === 'queued' || issue.execution_status === 'running').length ?? 0;
  const allIssues = currentWorkspaceIssues.data ?? [];
  const [expanded, setExpanded] = useState(false);
  const recentIssues = expanded ? allIssues : allIssues.slice(0, 5);
  const runtimeCount = health.data?.available_runtimes?.length ?? 0;
  const runtimeList = health.data?.available_runtimes ?? [];
  const serverState = health.isLoading ? 'syncing' : health.isError ? 'attention' : 'online';
  const serverLabel = health.isLoading ? '상태 확인 중' : health.isError ? '연결 확인 필요' : 'Server online';
  const workspaceName = currentWorkspace?.name ?? 'No workspace selected';
  const workspacePrefix = currentWorkspace?.identifier_prefix ?? 'MCP';

  const commandSurfaces = useMemo(
    () => [
      {
        label: 'workspace',
        command: currentWorkspace ? `cron-agent workspace use ${currentWorkspace.slug}` : 'cron-agent workspace create',
        meta: currentWorkspace ? `${currentWorkspace.open_issue_count ?? 0} open issues` : '보드 생성을 시작하세요',
      },
      {
        label: 'runtime',
        command: runtimeList.length > 0 ? `runtime probe --found ${runtimeList.join(', ')}` : 'runtime probe --pending',
        meta: runtimeList.length > 0 ? `${runtimeList.length} runtimes available` : 'PATH 탐지 대기',
      },
      {
        label: 'autopilot',
        command: activeIssueCount > 0 ? `agent runs watch --active ${activeIssueCount}` : 'agent runs queue --idle',
        meta: activeIssueCount > 0 ? 'queued/running activity' : '현재 실행 대기 없음',
      },
    ],
    [activeIssueCount, currentWorkspace, runtimeList],
  );

  return (
    <section className="page-stack home-shell">
      <PageHeader
        eyebrow="CoreMCP demo"
        title="운영 현황"
        description="워크스페이스, MCP 서버 상태, agent 실행 흐름을 한 화면에서 점검합니다."
      />

      <section className="mcp-hero" aria-labelledby="mcp-hero-title">
        <div className="mcp-hero-copy">
          <div className="hero-kicker-row">
            <span className="issue-id">{workspacePrefix}</span>
            <span className={`server-pill server-pill--${serverState}`}>
              <span aria-hidden="true" />
              {serverLabel}
            </span>
          </div>
          <h2 id="mcp-hero-title">{currentWorkspace ? `${currentWorkspace.name} MCP control plane` : '새 워크스페이스로 MCP 보드를 시작하세요'}</h2>
          <p>
            {currentWorkspace
              ? `열린 이슈 ${currentWorkspace.open_issue_count ?? 0}개, 에이전트 ${currentWorkspace.agent_count ?? 0}개, 런타임 ${runtimeCount}개를 연결해 작업 흐름을 관측합니다.`
              : 'Cron Agent Dashboard는 로컬 작업, 런타임, 오토파일럿을 하나의 데모형 대시보드로 묶어 보여줍니다.'}
          </p>
          <div className="hero-actions" aria-label="주요 작업">
            {currentWorkspace ? (
              <Link className="button button-hero" to={`/w/${currentWorkspace.slug}/board`}>
                보드 열기
              </Link>
            ) : (
              <button className="button button-hero" type="button" onClick={openCreateWorkspace}>
                워크스페이스 생성
              </button>
            )}
            <Link className="button secondary" to="/settings">
              Settings
            </Link>
          </div>
        </div>

        <div className="mcp-live-card" aria-label="Live MCP server preview">
          <div className="live-card-topline">
            <span>mcp://local-dashboard</span>
            <strong>{serverState}</strong>
          </div>
          <div className="server-orbit" aria-hidden="true">
            <span />
            <span />
            <span />
          </div>
          <dl className="server-readout">
            <div>
              <dt>workspace</dt>
              <dd>{workspaceName}</dd>
            </div>
            <div>
              <dt>uptime</dt>
              <dd>{health.data?.uptime_seconds ? `${health.data.uptime_seconds}s` : 'self-check'}</dd>
            </div>
            <div>
              <dt>runtimes</dt>
              <dd>{runtimeList.join(' / ') || 'detecting'}</dd>
            </div>
          </dl>
        </div>
      </section>

      <div className="mcp-status-grid" aria-label="Dashboard metrics">
        <article className="metric-card dashboard-metric status-card">
          <span>API 상태</span>
          <strong>{health.isLoading ? '확인 중' : health.isError ? '주의' : '정상'}</strong>
          <p>{health.data?.uptime_seconds ? `uptime ${health.data.uptime_seconds}s` : 'startup self-check 기반'}</p>
        </article>
        <article className="metric-card dashboard-metric status-card">
          <span>워크스페이스</span>
          <strong>{workspaces.length}개</strong>
          <p>{isWorkspaceLoading ? '목록 확인 중' : `열린 이슈 ${totalOpenIssues}개`}</p>
        </article>
        <article className="metric-card dashboard-metric status-card">
          <span>실행 대기/진행</span>
          <strong>{activeIssueCount}개</strong>
          <p>{currentWorkspace ? `${currentWorkspace.name} 기준` : '워크스페이스 생성 후 표시'}</p>
        </article>
        <article className="metric-card dashboard-metric status-card">
          <span>런타임</span>
          <strong>{runtimeCount}개</strong>
          <p>{runtimeList.join(', ') || 'PATH 탐지 대기'}</p>
        </article>
      </div>
      {isUnauthorizedError(workspaceError) && <AuthTokenPanel error={workspaceError} />}

      <div className="mcp-dashboard-grid">
        <article className="panel dashboard-preview-panel">
          <div className="section-heading">
            <div>
              <h2>Dashboard preview</h2>
              <p>선택된 워크스페이스의 이슈 스트림과 실행 상태를 데모 콘솔처럼 보여줍니다.</p>
            </div>
            {currentWorkspace && (
              <Link className="inline-link" to={`/w/${currentWorkspace.slug}/board`}>
                전체 보기
              </Link>
            )}
          </div>

          <div className="preview-console">
            <div className="console-titlebar" aria-hidden="true">
              <span />
              <span />
              <span />
              <strong>{workspacePrefix.toLowerCase()}-board</strong>
            </div>
            {recentIssues.length > 0 ? (
              <div className="dashboard-issue-list">
                {recentIssues.map((issue) => (
                  <Link className="dashboard-issue-row" key={issue.id} to={`/w/${currentWorkspace?.slug}/issues/${issue.identifier}`}>
                    <span className="issue-id">{issue.identifier}</span>
                    <strong>{issue.title}</strong>
                    <span>
                      {issue.status} · {issue.execution_status} · @{issue.last_run_agent_name || issue.assignee_agent_name || '-'}
                    </span>
                  </Link>
                ))}
                {allIssues.length > 5 && (
                  <button className="inline-link" type="button" onClick={() => setExpanded((v) => !v)}>
                    {expanded ? '접기' : `더 보기 (+${allIssues.length - 5})`}
                  </button>
                )}
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
          </div>
        </article>

        <aside className="mcp-side-rail">
          <article className="panel command-surface-panel">
            <div className="section-heading">
              <div>
                <h2>Command surfaces</h2>
                <p>CLI와 서버 상태를 한 번에 읽을 수 있는 운영 표면입니다.</p>
              </div>
            </div>
            <div className="command-list">
              {commandSurfaces.map((item) => (
                <div className="command-row" key={item.label}>
                  <span>{item.label}</span>
                  <code>{item.command}</code>
                  <small>{item.meta}</small>
                </div>
              ))}
            </div>
          </article>

          <TeamPulseWidget slug={currentWorkspace?.slug} />

          <article className="panel quick-actions-panel">
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
        </aside>
      </div>
    </section>
  );
}
