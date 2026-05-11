import { useQuery } from '@tanstack/react-query';
import { apiClient } from './client';

export type WorkspaceSummary = {
  slug: string;
  name: string;
  issuePrefix: string;
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
    queryFn: () => apiClient.get<WorkspaceSummary>(`/workspaces/${slug}`)
  });
}

export function useSettingsQuery() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: () => apiClient.get<SettingsResponse>('/settings')
  });
}
