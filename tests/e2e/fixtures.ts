import type { APIRequestContext } from '@playwright/test';
import { expect } from '@playwright/test';

export type WorkspaceFixtureOptions = {
  slugPrefix?: string;
  identifierPrefix?: string;
  mainAgentName?: string;
  mainAgentRuntime?: string;
  mainAgentInstructions?: string;
};

export type WorkspaceFixture = {
  slug: string;
  workspaceId: string;
  mainAgentId: string;
  mainAgentName: string;
  identifierPrefix: string;
};

export function uniqueSuffix() {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

export async function createWorkspaceFixture(
  request: APIRequestContext,
  opts: WorkspaceFixtureOptions = {}
): Promise<WorkspaceFixture> {
  const suffix = uniqueSuffix();
  const slug = `${opts.slugPrefix ?? 'e2e'}-${suffix}`;
  const identifierPrefix = opts.identifierPrefix ?? 'TST';
  const mainAgentName = opts.mainAgentName ?? 'MainAgent';

  const response = await request.post('/api/workspaces', {
    data: {
      name: `E2E ${suffix}`,
      slug,
      identifier_prefix: identifierPrefix,
      main_agent: {
        name: mainAgentName,
        runtime: opts.mainAgentRuntime ?? 'missing-runtime',
        instructions:
          opts.mainAgentInstructions ??
          'E2E fixture agent. Workers will fail fast because the runtime is intentionally missing.'
      }
    }
  });
  expect(response.ok(), `workspace create failed: ${response.status()}`).toBeTruthy();
  const body = await response.json();
  return {
    slug,
    workspaceId: body.id,
    mainAgentId: body.main_agent_id ?? body.main_agent?.id,
    mainAgentName,
    identifierPrefix
  };
}

export async function seedSecondAgent(
  request: APIRequestContext,
  workspaceSlug: string,
  name: string,
  runtime: string = 'missing-runtime'
) {
  const response = await request.post(`/api/workspaces/${workspaceSlug}/agents`, {
    data: {
      name,
      runtime,
      instructions: `Secondary agent ${name} used by E2E fixtures.`
    }
  });
  expect(response.ok(), `agent create failed: ${response.status()}`).toBeTruthy();
  return response.json();
}

export async function deleteWorkspace(request: APIRequestContext, workspaceSlug: string) {
  const response = await request.delete(`/api/workspaces/${workspaceSlug}`);
  return response.ok();
}

async function cancelActiveIssues(request: APIRequestContext, workspaceSlug: string) {
  const listed = await request.get(`/api/workspaces/${workspaceSlug}/issues?limit=200`);
  if (!listed.ok()) {
    return;
  }
  const payload = await listed.json();
  const issues: Array<{ id: string; execution_status?: string }> = Array.isArray(payload)
    ? payload
    : payload.issues ?? [];
  for (const issue of issues) {
    if (issue.execution_status === 'queued' || issue.execution_status === 'running') {
      await request.post(`/api/issues/${issue.id}/cancel`);
    }
  }
}

export async function clearAllWorkspaces(request: APIRequestContext) {
  for (let attempt = 0; attempt < 5; attempt += 1) {
    const listed = await request.get('/api/workspaces');
    if (!listed.ok()) {
      return;
    }
    const payload = await listed.json();
    const items: Array<{ slug: string }> = Array.isArray(payload) ? payload : payload.workspaces ?? [];
    if (!items.length) {
      return;
    }
    let progressed = false;
    for (const ws of items) {
      const res = await request.delete(`/api/workspaces/${ws.slug}`);
      if (res.ok()) {
        progressed = true;
        continue;
      }
      // workspace had active runs — cancel them and retry on next loop
      await cancelActiveIssues(request, ws.slug);
    }
    if (!progressed) {
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
}
