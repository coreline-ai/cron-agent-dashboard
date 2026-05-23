import { Navigate, Route, Routes } from 'react-router-dom';
import { AppErrorBoundary } from './components/AppErrorBoundary';
import { ToastProvider } from './components/ToastProvider';
import { DashboardLayout } from './layouts/DashboardLayout';
import { AgentDetailPage } from './pages/AgentDetailPage';
import { AgentsPage } from './pages/AgentsPage';
import { AutopilotPage } from './pages/AutopilotPage';
import { BoardPage } from './pages/BoardPage';
import { HomePage } from './pages/HomePage';
import { IssueDetailPage } from './pages/IssueDetailPage';
import { SettingsPage } from './pages/SettingsPage';
import { WorkspaceChainsPage } from './pages/WorkspaceChainsPage';
import { WorkspaceRunsPage } from './pages/WorkspaceRunsPage';

export function App() {
  return (
    <ToastProvider>
      <AppErrorBoundary>
        <Routes>
        <Route element={<DashboardLayout />}>
          <Route path="/" element={<HomePage />} />
          <Route path="/w/:slug/board" element={<BoardPage />} />
          <Route path="/w/:slug/issues/:identifier" element={<IssueDetailPage />} />
          <Route path="/w/:slug/agents" element={<AgentsPage />} />
          <Route path="/w/:slug/agents/:id" element={<AgentDetailPage />} />
          <Route path="/w/:slug/autopilot" element={<AutopilotPage />} />
          <Route path="/w/:slug/chains" element={<WorkspaceChainsPage />} />
          <Route path="/w/:slug/runs" element={<WorkspaceRunsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AppErrorBoundary>
    </ToastProvider>
  );
}
