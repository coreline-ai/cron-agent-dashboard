import { useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { apiClient } from '../api/client';
import { AutopilotDialog, type AutopilotTemplate } from '../components/AutopilotDialog';
import { PageHeader } from '../components/PageHeader';
import type { AutopilotRule } from '../api/queries';
import { useAutopilotRulesQuery } from '../api/queries';

type RuleFilter = 'all' | 'enabled' | 'paused';

const filters: Array<{ value: RuleFilter; label: string }> = [
  { value: 'all', label: '전체' },
  { value: 'enabled', label: 'ON' },
  { value: 'paused', label: 'OFF' }
];

const templates: Array<AutopilotTemplate & { summary: string }> = [
  {
    name: '매일 AI 뉴스 브리핑',
    cron_expr: '0 9 * * *',
    issue_title_template: '{{date}} AI 뉴스 브리핑',
    issue_body_template: '오늘의 AI 주요 뉴스를 5개로 요약하고, 출처 링크와 영향도를 정리하세요.',
    summary: '매일 오전 9시 AI 뉴스 정리 이슈 생성'
  },
  {
    name: '주간 작업 회고',
    cron_expr: '0 17 * * 5',
    issue_title_template: '{{date}} 주간 작업 회고',
    issue_body_template: '이번 주 완료/진행/실패 작업을 요약하고 다음 액션을 제안하세요.',
    summary: '매주 금요일 작업 흐름 요약'
  },
  {
    name: '문서 갭 점검',
    cron_expr: '0 10 * * 1',
    issue_title_template: '{{date}} 문서 갭 점검',
    issue_body_template: '최근 변경 사항과 문서의 불일치를 찾아 업데이트 후보를 제안하세요.',
    summary: '매주 월요일 문서 정합성 확인'
  }
];

export function AutopilotPage() {
  const { slug } = useParams();
  const rules = useAutopilotRulesQuery(slug);
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState<RuleFilter>('all');
  const [query, setQuery] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState<AutopilotTemplate | null>(null);
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['autopilot', slug] });
  const triggerRule = useMutation({ mutationFn: (id: string) => apiClient.post(`/autopilot/${id}/trigger`), onSuccess: invalidate });
  const toggleRule = useMutation({
    mutationFn: (rule: AutopilotRule) =>
      apiClient.put(`/autopilot/${rule.id}`, {
        name: rule.name,
        cron_expr: rule.cron_expr,
        issue_title_template: rule.issue_title_template,
        issue_body_template: rule.issue_body_template ?? '',
        assignee_agent_id: rule.assignee_agent_id ?? '',
        enabled: !rule.enabled
      }),
    onSuccess: invalidate
  });
  const deleteRule = useMutation({ mutationFn: (id: string) => apiClient.delete(`/autopilot/${id}`), onSuccess: invalidate });

  const visibleRules = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (rules.data ?? []).filter((rule) => {
      if (filter === 'enabled' && !rule.enabled) {
        return false;
      }
      if (filter === 'paused' && rule.enabled) {
        return false;
      }
      if (!q) {
        return true;
      }
      return [rule.name, rule.cron_expr, rule.issue_title_template, rule.assignee_agent_name]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(q));
    });
  }, [filter, query, rules.data]);
  const counts = useMemo(() => {
    const xs = rules.data ?? [];
    return { all: xs.length, enabled: xs.filter((rule) => rule.enabled).length, paused: xs.filter((rule) => !rule.enabled).length };
  }, [rules.data]);

  const openCreate = (template?: AutopilotTemplate) => {
    setSelectedTemplate(template ?? null);
    setCreateOpen(true);
  };
  const closeCreate = () => {
    setCreateOpen(false);
    setSelectedTemplate(null);
  };

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="오토파일럿"
        description="cron 기반 정기 이슈 생성 규칙을 테이블과 템플릿으로 관리합니다."
      />

      <div className="board-toolbar panel">
        <div className="toolbar-main">
          <div>
            <h2>자동화 규칙</h2>
            <p>
              전체 {counts.all} · ON {counts.enabled} · OFF {counts.paused}
            </p>
          </div>
          <button className="button" type="button" onClick={() => openCreate()}>
            규칙 추가
          </button>
        </div>
        <div className="toolbar-controls">
          <div className="segmented" role="tablist" aria-label="오토파일럿 필터">
            {filters.map((item) => (
              <button key={item.value} type="button" className={filter === item.value ? 'active' : ''} onClick={() => setFilter(item.value)}>
                {item.label}
              </button>
            ))}
          </div>
          <input className="toolbar-search" placeholder="규칙 검색" value={query} onChange={(event) => setQuery(event.target.value)} />
        </div>
      </div>

      {!rules.isLoading && !rules.data?.length && (
        <article className="panel template-panel">
          <div className="section-heading">
            <h2>자주 쓰는 템플릿</h2>
            <span className="badge">start</span>
          </div>
          <div className="template-grid">
            {templates.map((template) => (
              <button className="template-card" type="button" key={template.name} onClick={() => openCreate(template)}>
                <strong>{template.name}</strong>
                <span>{template.summary}</span>
                <small>{template.cron_expr}</small>
              </button>
            ))}
          </div>
          <button className="button secondary" type="button" onClick={() => openCreate()}>
            빈 규칙으로 시작
          </button>
        </article>
      )}

      <article className="panel table-panel">
        <div className="data-table autopilot-table">
          <div className="data-row data-head">
            <span>Rule</span>
            <span>Cron</span>
            <span>Agent</span>
            <span>Next</span>
            <span>Last</span>
            <span>Actions</span>
          </div>
          {visibleRules.map((rule) => (
            <div className="data-row" key={rule.id}>
              <span>
                <strong>{rule.name}</strong>
                <small>{rule.issue_title_template}</small>
              </span>
              <span>{rule.cron_expr}</span>
              <span>{rule.assignee_agent_name ? `@${rule.assignee_agent_name}` : '메인 에이전트'}</span>
              <span>{formatDateTime(rule.next_run_at)}</span>
              <span>{formatDateTime(rule.last_run_at)}</span>
              <span className="row-actions">
                <span className="badge">{rule.enabled ? 'ON' : 'OFF'}</span>
                <button className="button micro secondary" type="button" onClick={() => toggleRule.mutate(rule)}>
                  {rule.enabled ? '끄기' : '켜기'}
                </button>
                <button className="button micro secondary" type="button" onClick={() => triggerRule.mutate(rule.id)}>
                  지금 실행
                </button>
                <button className="button micro danger" type="button" onClick={() => deleteRule.mutate(rule.id)}>
                  삭제
                </button>
              </span>
            </div>
          ))}
        </div>
        {!rules.isLoading && !visibleRules.length && Boolean(rules.data?.length) && <p>조건에 맞는 규칙이 없습니다.</p>}
      </article>

      <AutopilotDialog open={createOpen} slug={slug} template={selectedTemplate} onClose={closeCreate} />
    </section>
  );
}

function formatDateTime(value?: string) {
  if (!value) {
    return '예약 없음';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat('ko-KR', {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(date);
}
