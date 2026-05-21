import { describe, expect, it } from 'vitest';

import { buildIssueFlowGraph } from './IssueFlowGraph';
import type { Issue, Run } from '../api/queries';

const issue = { id: 'i1', identifier: 'ISS-1', title: 'Parent', body: '', status: 'open', execution_status: 'done', comment_count: 0 } as Issue;

describe('buildIssueFlowGraph', () => {
  it('connects sub-issues and parent/child runs into a lineage graph', () => {
    const subIssues = [
      { id: 's1', identifier: 'ISS-2', title: 'Sub', body: '', status: 'done', execution_status: 'done', comment_count: 0 }
    ] as Issue[];
    const runs = [
      { id: 'r1', status: 'done', trigger_type: 'issue_created', agent_name: 'Lead', enqueued_at: '2026-05-15T00:00:00Z' },
      { id: 'r2', status: 'queued', trigger_type: 'mention', agent_name: 'Writer', parent_run_id: 'r1', enqueued_at: '2026-05-15T00:01:00Z' }
    ] as Run[];

    const graph = buildIssueFlowGraph(issue, subIssues, runs);

    expect(graph.nodes.map((node) => node.id)).toEqual(['issue-i1', 'subissue-s1', 'run-r1', 'run-r2']);
    expect(graph.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ source: 'issue-i1', target: 'subissue-s1' }),
        expect.objectContaining({ source: 'issue-i1', target: 'run-r1' }),
        expect.objectContaining({ source: 'run-r1', target: 'run-r2' })
      ])
    );
  });

  it('annotates run labels with chain_depth and flags main-agent re-entry as a hub node', () => {
    const runs = [
      { id: 'r1', status: 'done', trigger_type: 'issue_created', agent_name: 'Lead', chain_depth: 0, enqueued_at: '2026-05-21T00:00:00Z' },
      { id: 'r2', status: 'done', trigger_type: 'mention', agent_name: 'Writer', parent_run_id: 'r1', chain_depth: 1, enqueued_at: '2026-05-21T00:01:00Z' },
      { id: 'r3', status: 'queued', trigger_type: 'mention', agent_name: 'Lead', parent_run_id: 'r2', chain_depth: 1, enqueued_at: '2026-05-21T00:02:00Z' }
    ] as Run[];

    const graph = buildIssueFlowGraph(issue, [], runs, { mainAgentName: 'Lead' });

    const leadInitial = graph.nodes.find((node) => node.id === 'run-r1');
    const writer = graph.nodes.find((node) => node.id === 'run-r2');
    const leadReentry = graph.nodes.find((node) => node.id === 'run-r3');

    expect(String(leadInitial?.data?.label)).toContain('d=0');
    expect(String(leadInitial?.data?.label)).toContain('hub');
    expect(leadInitial?.className).toContain('run-hub');

    expect(String(writer?.data?.label)).toContain('d=1');
    expect(writer?.className).not.toContain('run-hub');

    expect(String(leadReentry?.data?.label)).toContain('d=1');
    expect(leadReentry?.className).toContain('run-hub');
  });

  it('omits the hub annotation when mainAgentName is not supplied', () => {
    const runs = [
      { id: 'r1', status: 'done', trigger_type: 'issue_created', agent_name: 'Lead', chain_depth: 0, enqueued_at: '2026-05-21T00:00:00Z' }
    ] as Run[];

    const graph = buildIssueFlowGraph(issue, [], runs);

    const lead = graph.nodes.find((node) => node.id === 'run-r1');
    expect(String(lead?.data?.label)).not.toContain('hub');
    expect(lead?.className).not.toContain('run-hub');
  });
});
