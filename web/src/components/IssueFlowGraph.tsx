import { Background, Controls, MarkerType, ReactFlow, type Edge, type Node, Position } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { Issue, Run } from '../api/queries';
import { getTriggerLabel } from '../lib/runLabels';

export type IssueFlowGraphProps = {
  issue: Issue;
  subIssues: Issue[];
  runs: Run[];
  mainAgentName?: string;
};

export type BuildIssueFlowGraphOptions = {
  mainAgentName?: string;
};

export function IssueFlowGraph({ issue, subIssues, runs, mainAgentName }: IssueFlowGraphProps) {
  const graph = buildIssueFlowGraph(issue, subIssues, runs, { mainAgentName });

  if (graph.nodes.length <= 1) {
    return <p className="muted-copy">아직 그래프로 표시할 하위 이슈나 run chain이 없습니다.</p>;
  }

  return (
    <div className="issue-flow-graph-wrap" aria-label="이슈 흐름 그래프">
      <p className="issue-flow-graph__legend">
        run 노드의 <code>d=N</code>은 <code>chain_depth</code>입니다. main agent 재진입은 depth를 증가시키지 않으며 <code>max_depth</code> 가드에서도 제외됩니다.
      </p>
      <div className="issue-flow-graph">
        <ReactFlow
          nodes={graph.nodes}
          edges={graph.edges}
          fitView
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
          zoomOnScroll={false}
          panOnScroll
        >
          <Background />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
    </div>
  );
}

function normalizeAgentName(name?: string): string {
  return (name ?? '').trim().toLowerCase();
}

export function buildIssueFlowGraph(
  issue: Issue,
  subIssues: Issue[],
  runs: Run[],
  options?: BuildIssueFlowGraphOptions
): { nodes: Node[]; edges: Edge[] } {
  const mainAgentKey = normalizeAgentName(options?.mainAgentName);
  const nodes: Node[] = [
    graphNode(`issue-${issue.id}`, `${issue.identifier}\n${issue.title}`, 0, 0, 'issue')
  ];
  const edges: Edge[] = [];

  subIssues.forEach((subIssue, index) => {
    const x = (index - (subIssues.length - 1) / 2) * 220;
    const id = `subissue-${subIssue.id}`;
    nodes.push(graphNode(id, `${subIssue.identifier}\n${subIssue.title}`, x, 150, `issue-${subIssue.status}`));
    edges.push(graphEdge(`issue-${issue.id}`, id, `edge-sub-${subIssue.id}`));
  });

  const sortedRuns = [...runs].sort((a, b) => (a.enqueued_at ?? '').localeCompare(b.enqueued_at ?? ''));
  sortedRuns.forEach((run, index) => {
    const id = `run-${run.id}`;
    const x = (index - (sortedRuns.length - 1) / 2) * 220;
    const y = subIssues.length > 0 ? 330 : 160;
    const depth = typeof run.chain_depth === 'number' ? run.chain_depth : undefined;
    const isHub = mainAgentKey !== '' && normalizeAgentName(run.agent_name) === mainAgentKey;
    const depthSuffix = depth !== undefined ? ` · d=${depth}` : '';
    const hubSuffix = isHub ? ' · hub' : '';
    const label = `@${run.agent_name || '-'}${hubSuffix}\n${run.status} · ${getTriggerLabel(run.trigger_type)}${depthSuffix}`;
    const kind = isHub ? `run-${run.status} run-hub` : `run-${run.status}`;
    nodes.push(graphNode(id, label, x, y, kind));
    const parent = run.parent_run_id ? `run-${run.parent_run_id}` : `issue-${issue.id}`;
    edges.push(graphEdge(parent, id, `edge-run-${run.id}`));
  });

  return { nodes, edges };
}

function graphNode(id: string, label: string, x: number, y: number, kind: string): Node {
  return {
    id,
    data: { label },
    position: { x, y },
    sourcePosition: Position.Bottom,
    targetPosition: Position.Top,
    className: `flow-node flow-node-${kind}`
  };
}

function graphEdge(source: string, target: string, id: string): Edge {
  return {
    id,
    source,
    target,
    markerEnd: { type: MarkerType.ArrowClosed },
    animated: target.startsWith('run-')
  };
}

export default IssueFlowGraph;
