import { NavLink, Outlet } from 'react-router-dom';

const demoSlug = 'news';

const navItems = [
  { to: '/', label: '홈' },
  { to: `/w/${demoSlug}/board`, label: '보드' },
  { to: `/w/${demoSlug}/agents`, label: '에이전트' },
  { to: `/w/${demoSlug}/autopilot`, label: '오토파일럿' },
  { to: '/settings', label: '설정' }
];

export function DashboardLayout() {
  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="주요 메뉴">
        <div className="brand">
          <span className="brand-mark">C</span>
          <div>
            <strong>Corn Agent</strong>
            <small>Dashboard</small>
          </div>
        </div>
        <nav className="nav-list">
          {navItems.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.to === '/'}>
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
