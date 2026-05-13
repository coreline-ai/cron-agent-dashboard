import { useQuery } from '@tanstack/react-query';
import { apiClient } from './client';

export type WorkspaceSummary = {
  id: string;
  slug: string;
  name: string;
  description: string;
  identifier_prefix: string;
  agent_count?: number;
  open_issue_count?: number;
};

export type IssueStatus = 'open' | 'done' | 'cancelled';
export type ExecutionStatus = 'idle' | 'queued' | 'running' | 'done' | 'failed' | 'cancelled';

export type Agent = {
  id: string;
  name: string;
  runtime: string;
  model?: string;
  instructions: string;
  is_main: boolean;
};

export type Issue = {
  id: string;
  identifier: string;
  title: string;
  body: string;
  status: IssueStatus;
  execution_status: ExecutionStatus;
  assignee_agent_name?: string;
  last_run_agent_name?: string;
  comment_count: number;
};

export type Comment = {
  id: string;
  author_type: string;
  author_agent_name?: string;
  run_id?: string;
  content: string;
  log_url?: string;
  created_at: string;
};

export type Run = {
  id: string;
  agent_name?: string;
  status: string;
  trigger_type: string;
  log_url?: string;
  error_message?: string;
  exit_code?: number | null;
  stdout_size_bytes?: number;
  enqueued_at?: string;
  started_at?: string;
  finished_at?: string;
};

export type AutopilotRule = {
  id: string;
  name: string;
  cron_expr: string;
  issue_title_template: string;
  issue_body_template: string;
  assignee_agent_id?: string;
  assignee_agent_name?: string;
  enabled: boolean;
  last_run_at?: string;
  next_run_at?: string;
};

export type HealthResponse = {
  status: string;
  version?: string;
  uptime_seconds?: number;
  db_ok?: boolean;
  available_runtimes?: string[];
};

export type SettingsResponse = {
  version: string;
  data_dir: string;
  worker_pool_size: number;
  auth_mode: string;
  timezone: string;
  available_runtimes: Array<{
    name: string;
    version: string;
    path: string;
  }>;
};

async function fetchHealth(): Promise<HealthResponse> {
  const response = await fetch('/healthz');
  if (!response.ok) {
    throw new Error(`health check failed (${response.status})`);
  }
  return (await response.json()) as HealthResponse;
}

export function useHealthQuery() {
  return useQuery({
    queryKey: ['health'],
    queryFn: fetchHealth
  });
}

export function useWorkspaceQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['workspace', slug],
    enabled: Boolean(slug),
    queryFn: async () => (await apiClient.get<{ workspace: WorkspaceSummary }>(`/workspaces/${slug}`)).workspace
  });
}

export function useSettingsQuery() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: () => apiClient.get<SettingsResponse>('/settings')
  });
}

export function useWorkspacesQuery() {
  return useQuery({
    queryKey: ['workspaces'],
    queryFn: async () => (await apiClient.get<{ workspaces: WorkspaceSummary[] | null }>('/workspaces')).workspaces ?? []
  });
}

export function useAgentsQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['agents', slug],
    enabled: Boolean(slug),
    queryFn: async () => (await apiClient.get<{ agents: Agent[] | null }>(`/workspaces/${slug}/agents`)).agents ?? []
  });
}

export function useAgentQuery(id: string | undefined) {
  return useQuery({
    queryKey: ['agent', id],
    enabled: Boolean(id),
    queryFn: async () => (await apiClient.get<{ agent: Agent }>(`/agents/${id}`)).agent
  });
}

export function useIssuesQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['issues', slug],
    enabled: Boolean(slug),
    refetchInterval: 5_000,
    queryFn: async () => (await apiClient.get<{ issues: Issue[] | null }>(`/workspaces/${slug}/issues`)).issues ?? []
  });
}

export function useWorkspaceIssueQuery(slug: string | undefined, identifier: string | undefined) {
  return useQuery({
    queryKey: ['issue', slug, identifier],
    enabled: Boolean(slug && identifier),
    refetchInterval: (query) => {
      const issue = query.state.data as Issue | undefined;
      return issue?.execution_status === 'queued' || issue?.execution_status === 'running' ? 3_000 : false;
    },
    queryFn: async () =>
      (await apiClient.get<{ issue: Issue }>(`/workspaces/${slug}/issues/${identifier}`)).issue
  });
}

export function useCommentsQuery(issueId: string | undefined, executionStatus: string | undefined) {
  return useQuery({
    queryKey: ['comments', issueId],
    enabled: Boolean(issueId),
    refetchInterval: executionStatus === 'queued' || executionStatus === 'running' ? 3_000 : false,
    queryFn: async () => (await apiClient.get<{ comments: Comment[] }>(`/issues/${issueId}/comments`)).comments
  });
}

export function useRunsQuery(issueId: string | undefined, executionStatus: string | undefined) {
  return useQuery({
    queryKey: ['runs', issueId],
    enabled: Boolean(issueId),
    refetchInterval: executionStatus === 'queued' || executionStatus === 'running' ? 3_000 : false,
    queryFn: async () => (await apiClient.get<{ runs: Run[] }>(`/issues/${issueId}/runs`)).runs
  });
}

export function useAutopilotRulesQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['autopilot', slug],
    enabled: Boolean(slug),
    queryFn: async () => (await apiClient.get<{ rules: AutopilotRule[] | null }>(`/workspaces/${slug}/autopilot`)).rules ?? []
  });
}
