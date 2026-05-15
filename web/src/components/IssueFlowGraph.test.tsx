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
});
