import { useEffect, useMemo, useState } from 'react';
import { NavLink, Outlet, useLocation, useNavigate, useParams } from 'react-router-dom';
import { BrandMark } from '../components/BrandMark';
import { CreateWorkspaceDialog } from '../components/CreateWorkspaceDialog';
import { WorkspaceSwitcher } from '../components/WorkspaceSwitcher';
import { useWorkspacesQuery, type WorkspaceSummary } from '../api/queries';

const productNav = [{ to: '/', label: '대시보드', hint: 'Overview' }];
const systemNav = [{ to: '/settings', label: '설정', hint: 'Local' }];
const themeStorageKey = 'corn-agent-dashboard-theme';
const workspaceStorageKey = 'corn-agent-dashboard-last-workspace';
type Theme = 'light' | 'dark';
type NavItem = { to: string; label: string; hint: string; disabled?: boolean };

export type DashboardOutletContext = {
  currentWorkspace?: WorkspaceSummary;
  workspaces: WorkspaceSummary[];
  isWorkspaceLoading: boolean;
  workspaceError: unknown;
  openCreateWorkspace: () => void;
};

function NavGroup({ title, items }: { title: string; items: NavItem[] }) {
  return (
    <div className="nav-group">
      <p className="nav-group-label">{title}</p>
      <nav className="nav-list" aria-label={title}>
        {items.map((item) =>
          item.disabled ? (
            <span className="nav-disabled" key={item.label} aria-disabled="true">
              <span>{item.label}</span>
              <small>{item.hint}</small>
            </span>
          ) : (
            <NavLink key={item.to} to={item.to} end={item.to === '/'}>
              <span>{item.label}</span>
              <small>{item.hint}</small>
            </NavLink>
          )
        )}
      </nav>
    </div>
  );
}

export function DashboardLayout() {
  const { slug } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const workspacesQuery = useWorkspacesQuery();
  const workspaces = workspacesQuery.data ?? [];
  const [createWorkspaceOpen, setCreateWorkspaceOpen] = useState(false);
  const [lastWorkspaceSlug, setLastWorkspaceSlug] = useState(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return window.localStorage.getItem(workspaceStorageKey) ?? '';
  });
  const [theme, setTheme] = useState<Theme>(() => {
    if (typeof window === 'undefined') {
      return 'dark';
    }
    return window.localStorage.getItem(themeStorageKey) === 'light' ? 'light' : 'dark';
  });

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem(themeStorageKey, theme);
  }, [theme]);

  useEffect(() => {
    if (!slug) {
      return;
    }
    setLastWorkspaceSlug(slug);
    window.localStorage.setItem(workspaceStorageKey, slug);
  }, [slug]);

  const currentWorkspace = useMemo(() => {
    if (!workspaces.length) {
      return undefined;
    }
    return workspaces.find((workspace) => workspace.slug === slug) ?? workspaces.find((workspace) => workspace.slug === lastWorkspaceSlug) ?? workspaces[0];
  }, [lastWorkspaceSlug, slug, workspaces]);
  const currentSlug = currentWorkspace?.slug;
  const selectWorkspace = (nextSlug: string) => {
    setLastWorkspaceSlug(nextSlug);
    window.localStorage.setItem(workspaceStorageKey, nextSlug);
    const target = workspaceScopedPath(location.pathname, nextSlug);
    if (target && target !== location.pathname) {
      navigate(target);
    }
  };
  const openCreateWorkspace = () => setCreateWorkspaceOpen(true);
  const onWorkspaceCreated = (workspace: WorkspaceSummary) => {
    setCreateWorkspaceOpen(false);
    selectWorkspace(workspace.slug);
    navigate(`/w/${workspace.slug}/board`);
  };
  const workspaceNav: NavItem[] = currentSlug
    ? [
        { to: `/w/${currentSlug}/board`, label: '이슈 보드', hint: currentWorkspace?.identifier_prefix ?? 'Issues' },
        { to: `/w/${currentSlug}/agents`, label: '에이전트', hint: `${currentWorkspace?.agent_count ?? 0} agents` },
        { to: `/w/${currentSlug}/autopilot`, label: '오토파일럿', hint: 'Cron' }
      ]
    : [
        { to: '#', label: '이슈 보드', hint: '워크스페이스 필요', disabled: true },
        { to: '#', label: '에이전트', hint: '워크스페이스 필요', disabled: true },
        { to: '#', label: '오토파일럿', hint: '워크스페이스 필요', disabled: true }
      ];
  const nextTheme = theme === 'dark' ? 'light' : 'dark';

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="주요 메뉴">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            <BrandMark />
          </span>
          <div>
            <strong>Corn Agent</strong>
            <small>{currentWorkspace?.name ?? 'Local dashboard'}</small>
          </div>
        </div>
        <WorkspaceSwitcher
          workspaces={workspaces}
          currentWorkspace={currentWorkspace}
          isLoading={workspacesQuery.isLoading}
          onSelect={selectWorkspace}
          onCreate={openCreateWorkspace}
        />
        <NavGroup title="Product" items={productNav} />
        <NavGroup title="Workspace" items={workspaceNav} />
        <NavGroup title="System" items={systemNav} />
        <div className="sidebar-footer">
          <div className="sidebar-status">
            <span className="status-dot" aria-hidden="true" />
            <span>{workspacesQuery.isLoading ? 'Loading workspace' : currentSlug ? `/${currentSlug}` : 'No workspace'}</span>
          </div>
          <button className="theme-toggle" type="button" onClick={() => setTheme(nextTheme)} aria-label={`${nextTheme} 테마로 전환`}>
            {theme === 'dark' ? 'Light' : 'Dark'}
          </button>
        </div>
      </aside>
      <main className="content">
        <Outlet
          context={
            {
              currentWorkspace,
              workspaces,
              isWorkspaceLoading: workspacesQuery.isLoading,
              workspaceError: workspacesQuery.error,
              openCreateWorkspace
            } satisfies DashboardOutletContext
          }
        />
      </main>
      <CreateWorkspaceDialog open={createWorkspaceOpen} onClose={() => setCreateWorkspaceOpen(false)} onCreated={onWorkspaceCreated} />
    </div>
  );
}

function workspaceScopedPath(pathname: string, nextSlug: string) {
  if (!pathname.startsWith('/w/')) {
    return '';
  }
  const parts = pathname.split('/').filter(Boolean);
  const section = parts[2];
  if (section === 'agents') {
    return `/w/${nextSlug}/agents`;
  }
  if (section === 'autopilot') {
    return `/w/${nextSlug}/autopilot`;
  }
  return `/w/${nextSlug}/board`;
}
