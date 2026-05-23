import { useQuery } from '@tanstack/react-query';
import { apiClient } from './client';

export type WorkspaceSummary = {
  id: string;
  slug: string;
  name: string;
  description: string;
  identifier_prefix: string;
  working_dir?: string;
  output_dir?: string;
  default_timeout_seconds?: number;
  auto_chain_enabled?: boolean;
  auto_chain_max_depth?: number;
  auto_chain_daily_run_limit?: number;
  auto_chain_daily_cost_micros?: number;
  auto_chain_dry_run?: boolean;
  auto_close_on_run_done?: boolean;
  per_run_worktree?: boolean;
  agent_count?: number;
  open_issue_count?: number;
};

export type IssueStatus = 'open' | 'done' | 'cancelled';
export type ExecutionStatus = 'idle' | 'queued' | 'running' | 'done' | 'failed' | 'cancelled';
export type RunStatus = 'queued' | 'running' | 'done' | 'failed' | 'cancelled';
export type RunEventSeverity = 'debug' | 'info' | 'warn' | 'error';

export type Agent = {
  id: string;
  name: string;
  runtime: string;
  model?: string;
  instructions: string;
  instructions_version?: number;
  summary?: string;
  tags?: string;
  is_main: boolean;
  timeout_seconds_override?: number | null;
  retry_policy_json?: string;
};

export type AgentInstructionVersion = {
  id: string;
  agent_id: string;
  version: number;
  instructions: string;
  created_at: string;
};

export type Skill = {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  triggers: string[];
  content: string;
  source_type: string;
  source_url?: string;
  source_ref?: string;
  local_path?: string;
  content_hash: string;
  trust_level: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

export type AgentSkill = {
  agent_id: string;
  skill_id: string;
  activation_mode: 'always' | 'trigger' | 'manual';
  priority: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  skill?: Skill;
};

export type Issue = {
  id: string;
  workspace_id?: string;
  identifier: string;
  title: string;
  body: string;
  status: IssueStatus;
  execution_status: ExecutionStatus;
  assignee_agent_id?: string;
  assignee_agent_name?: string;
  parent_issue_id?: string;
  created_by?: string;
  autopilot_rule_id?: string;
  last_run_agent_id?: string;
  last_run_agent_name?: string;
  comment_count: number;
  created_at?: string;
  updated_at?: string;
};

export type Comment = {
  id: string;
  author_type: string;
  author_agent_name?: string;
  run_id?: string;
  content: string;
  truncated?: boolean;
  log_url?: string;
  created_at: string;
};

export type Run = {
  id: string;
  issue_id?: string;
  agent_id?: string;
  agent_name?: string;
  status: RunStatus | string;
  trigger_type: string;
  trigger_comment_id?: string;
  trigger_content_snapshot?: string;
  parent_run_id?: string;
  chain_id?: string;
  chain_depth?: number;
  agent_instructions_version?: number;
  log_url?: string;
  error_message?: string;
  exit_code?: number | null;
  stdout_size_bytes?: number;
  input_tokens?: number;
  output_tokens?: number;
  total_cost_micros?: number;
  model_resolved?: string;
  attempt?: number;
  max_attempts?: number;
  next_retry_at?: string;
  enqueued_at?: string;
  claimed_at?: string;
  claimed_by?: string;
  started_at?: string;
  heartbeat_at?: string;
  finished_at?: string;
  terminal_reason?: string;
  failure_kind?: string;
  cancel_reason?: string;
};

export type RunEvent = {
  id: string;
  run_id: string;
  issue_id: string;
  seq: number;
  event_type: string;
  severity: RunEventSeverity | string;
  message?: string;
  details?: Record<string, unknown>;
  created_at: string;
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
  snooze_until?: string;
  last_error?: string;
  consecutive_failures: number;
  last_triggered_issue_id?: string;
};

export type AutopilotTriggerResult = {
  ok: boolean;
  rule: AutopilotRule;
  issue?: Issue;
  run?: Run;
  error?: string;
};

export type AutopilotTriggerResponse = {
  trigger_result: AutopilotTriggerResult;
  rule: AutopilotRule;
  issue?: Issue;
  run?: Run;
};

export type HealthResponse = {
  status: string;
  version?: string;
  uptime_seconds?: number;
  db_ok?: boolean;
  available_runtimes?: string[];
};

export type UsageSummary = {
  since: string;
  run_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micros: number;
  measured_run_count: number;
};

export type SettingsResponse = {
  version: string;
  data_dir: string;
  worker_pool_size: number;
  auth_mode: string;
  timezone: string;
  usage_7d?: UsageSummary;
  migration_fail_count?: number;
  migration_failures?: Array<{
    id: number;
    version: number;
    name: string;
    error: string;
    failed_at: string;
  }>;
  maintenance?: {
    auto_backup: boolean;
    auto_backup_keep: number;
    auto_cleanup_log_days: number;
    interval_seconds: number;
    last_log_cleanup_at?: string;
    last_log_cleanup_files?: string;
    last_log_cleanup_bytes?: string;
    worktree_bytes?: string;
    worktree_dir_count?: string;
    worktree_pruned_last_pass?: string;
    worktree_measured_at?: string;
  };
  run_lifecycle?: {
    heartbeat_interval_seconds: number;
    stale_after_seconds: number;
    stale_scan_interval_seconds: number;
  };
  available_runtimes: Array<{
    name: string;
    version: string;
    path: string;
    supported?: boolean;
    warning?: string;
  }>;
};

export type IssuesQueryParams = {
  status?: IssueStatus | 'all';
  execution?: ExecutionStatus | 'all';
  assignee?: string;
  q?: string;
  limit?: number;
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

export function useUsageSummaryQuery(days: number) {
  return useQuery({
    queryKey: ['usage-summary', days],
    queryFn: async () => (await apiClient.get<{ usage: UsageSummary; days: number }>(`/usage/summary?days=${days}`)).usage
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

export function useAgentInstructionVersionsQuery(id: string | undefined) {
  return useQuery({
    queryKey: ['agent-instructions', id],
    enabled: Boolean(id),
    queryFn: async () => (await apiClient.get<{ versions: AgentInstructionVersion[] | null }>(`/agents/${id}/instructions`)).versions ?? []
  });
}

export type Attachment = {
  id: string;
  issue_id: string;
  comment_id?: string;
  uploaded_by: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  sha256: string;
  download_url: string;
  created_at: string;
};

export function useIssueAttachmentsQuery(issueID: string | undefined) {
  return useQuery({
    enabled: Boolean(issueID),
    queryKey: ['attachments', issueID],
    queryFn: async () =>
      (await apiClient.get<{ attachments: Attachment[] | null }>(`/issues/${issueID}/attachments`)).attachments ?? []
  });
}

export type Webhook = {
  id: string;
  workspace_id: string;
  url: string;
  has_secret: boolean;
  events: string[];
  enabled: boolean;
  mask_pii: boolean;
  failed_delivery_count: number;
  created_at: string;
  updated_at: string;
};

export type WebhookDelivery = {
  id: string;
  webhook_id: string;
  event_type: string;
  status: 'pending' | 'delivered' | 'failed';
  status_code: number;
  response_body?: string;
  error_message?: string;
  attempt: number;
  next_attempt_at: string;
  delivered_at?: string;
  created_at: string;
};

export function useWorkspaceWebhooksQuery(slug: string | undefined) {
  return useQuery({
    enabled: Boolean(slug),
    queryKey: ['webhooks', slug],
    queryFn: async () => (await apiClient.get<{ webhooks: Webhook[] | null }>(`/workspaces/${slug}/webhooks`)).webhooks ?? []
  });
}

export function useWebhookDeliveriesQuery(webhookID: string | undefined, limit = 5) {
  return useQuery({
    enabled: Boolean(webhookID),
    queryKey: ['webhook-deliveries', webhookID, limit],
    queryFn: async () =>
      (await apiClient.get<{ deliveries: WebhookDelivery[] | null }>(`/webhooks/${webhookID}/deliveries?limit=${limit}`)).deliveries ?? [],
    refetchInterval: 15_000
  });
}

export function useWorkspaceSkillsQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['workspace-skills', slug],
    enabled: Boolean(slug),
    queryFn: async () => (await apiClient.get<{ skills: Skill[] | null }>(`/workspaces/${slug}/skills`)).skills ?? []
  });
}

export type AgentActivity = {
  agent_id: string;
  agent_name: string;
  runtime: string;
  is_main: boolean;
  latest_run_id?: string;
  latest_run_status?: string;
  latest_run_finished_at?: string;
  latest_run_enqueued_at?: string;
  latest_issue_id?: string;
  latest_issue_identifier?: string;
};

export function useAgentActivityQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['agent-activity', slug],
    enabled: Boolean(slug),
    refetchInterval: 5_000,
    queryFn: async () => (await apiClient.get<{ activity: AgentActivity[] | null }>(`/workspaces/${slug}/agents/activity`)).activity ?? []
  });
}

export function useAgentSkillsQuery(id: string | undefined) {
  return useQuery({
    queryKey: ['agent-skills', id],
    enabled: Boolean(id),
    queryFn: async () => (await apiClient.get<{ skills: AgentSkill[] | null }>(`/agents/${id}/skills`)).skills ?? []
  });
}

export function useIssuesQuery(slug: string | undefined, params: IssuesQueryParams = {}) {
  const queryString = buildIssuesQuery(params);
  return useQuery({
    queryKey: ['issues', slug, queryString],
    enabled: Boolean(slug),
    refetchInterval: 5_000,
    queryFn: async () => (await apiClient.get<{ issues: Issue[] | null }>(`/workspaces/${slug}/issues${queryString}`)).issues ?? []
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


export function useSubIssuesQuery(issueId: string | undefined) {
  return useQuery({
    queryKey: ['subissues', issueId],
    enabled: Boolean(issueId),
    queryFn: async () => (await apiClient.get<{ issues: Issue[] | null }>(`/issues/${issueId}/subissues`)).issues ?? []
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

export function useRunEventsQuery(runId: string | undefined, executionStatus?: string) {
  return useQuery({
    queryKey: ['run-events', runId],
    enabled: Boolean(runId),
    refetchInterval: executionStatus === 'queued' || executionStatus === 'running' ? 3_000 : false,
    queryFn: async () => (await apiClient.get<{ events: RunEvent[] }>(`/runs/${runId}/events`)).events ?? []
  });
}

export function useAutopilotRulesQuery(slug: string | undefined) {
  return useQuery({
    queryKey: ['autopilot', slug],
    enabled: Boolean(slug),
    queryFn: async () => (await apiClient.get<{ rules: AutopilotRule[] | null }>(`/workspaces/${slug}/autopilot`)).rules ?? []
  });
}

function buildIssuesQuery(params: IssuesQueryParams) {
  const search = new URLSearchParams();
  if (params.status && params.status !== 'all') {
    search.set('status', params.status);
  }
  if (params.execution && params.execution !== 'all') {
    search.set('execution', params.execution);
  }
  if (params.assignee) {
    search.set('assignee', params.assignee);
  }
  const q = params.q?.trim();
  if (q) {
    search.set('q', q);
  }
  if (params.limit) {
    search.set('limit', String(params.limit));
  }
  const value = search.toString();
  return value ? `?${value}` : '';
}
