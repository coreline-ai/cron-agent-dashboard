import { useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { ApiError, apiClient } from '../api/client';
import { AutopilotDialog, type AutopilotTemplate } from '../components/AutopilotDialog';
import { PageHeader } from '../components/PageHeader';
import type { AutopilotRule, AutopilotTriggerResponse } from '../api/queries';
import { useAutopilotRulesQuery } from '../api/queries';

type RuleFilter = 'all' | 'enabled' | 'paused';
type TriggerNotice = {
  tone: 'success' | 'error';
  message: string;
  issueHref?: string;
};

const filters: Array<{ value: RuleFilter; label: string }> = [
  { value: 'all', label: '전체' },
  { value: 'enabled', label: 'ON' },
  { value: 'paused', label: 'OFF/일시정지' }
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
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState<AutopilotTemplate | null>(null);
  const [editingRule, setEditingRule] = useState<AutopilotRule | null>(null);
  const [triggerNotice, setTriggerNotice] = useState<TriggerNotice | null>(null);
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['autopilot', slug] });
  const refreshRules = () => {
    void rules.refetch();
  };
  const triggerRule = useMutation({
    mutationFn: (rule: AutopilotRule) => apiClient.post<AutopilotTriggerResponse>(`/autopilot/${rule.id}/trigger`),
    onSuccess: (data, rule) => {
      const issue = data.issue ?? data.trigger_result.issue;
      setTriggerNotice({
        tone: 'success',
        message: `${data.rule.name || rule.name} 실행 이슈를 생성했습니다.`,
        issueHref: issue ? `/w/${slug}/issues/${issue.identifier || issue.id}` : undefined
      });
    },
    onError: (error, rule) => {
      setTriggerNotice({
        tone: 'error',
        message: `${rule.name} 실행 실패: ${apiErrorMessage(error)}`
      });
    },
    onSettled: invalidate
  });
  const toggleRule = useMutation({
    mutationFn: (rule: AutopilotRule) =>
      apiClient.put(`/autopilot/${rule.id}`, {
        name: rule.name,
        cron_expr: rule.cron_expr,
        issue_title_template: rule.issue_title_template,
        issue_body_template: rule.issue_body_template ?? '',
        assignee_agent_id: rule.assignee_agent_id ?? '',
        enabled: !rule.enabled,
        snooze_until: rule.snooze_until ?? ''
      }),
    onSuccess: invalidate
  });
  const deleteRule = useMutation({ mutationFn: (id: string) => apiClient.delete(`/autopilot/${id}`), onSuccess: invalidate });

  const visibleRules = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (rules.data ?? []).filter((rule) => {
      const snoozed = isRuleSnoozed(rule);
      if (filter === 'enabled' && (!rule.enabled || snoozed)) {
        return false;
      }
      if (filter === 'paused' && rule.enabled && !snoozed) {
        return false;
      }
      if (!q) {
        return true;
      }
      return [rule.name, rule.cron_expr, rule.issue_title_template, rule.assignee_agent_name, rule.last_error]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(q));
    });
  }, [filter, query, rules.data]);
  const counts = useMemo(() => {
    const xs = rules.data ?? [];
    return {
      all: xs.length,
      enabled: xs.filter((rule) => rule.enabled && !isRuleSnoozed(rule)).length,
      paused: xs.filter((rule) => !rule.enabled || isRuleSnoozed(rule)).length,
      failed: xs.filter((rule) => (rule.consecutive_failures ?? 0) > 0).length
    };
  }, [rules.data]);

  const openCreate = (template?: AutopilotTemplate) => {
    setEditingRule(null);
    setSelectedTemplate(template ?? null);
    setDialogOpen(true);
  };
  const openEdit = (rule: AutopilotRule) => {
    setSelectedTemplate(null);
    setEditingRule(rule);
    setDialogOpen(true);
  };
  const closeDialog = () => {
    setDialogOpen(false);
    setSelectedTemplate(null);
    setEditingRule(null);
  };

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow={`워크스페이스 / ${slug}`}
        title="오토파일럿"
        description="cron 기반 정기 이슈 생성 규칙을 테이블과 템플릿으로 관리합니다."
        actions={
          <button className="button micro secondary" type="button" onClick={refreshRules} disabled={rules.isFetching}>
            {rules.isFetching ? '새로고침 중' : '새로고침'}
          </button>
        }
      />

      <div className="board-toolbar panel">
        <div className="toolbar-main">
          <div>
            <h2>자동화 규칙</h2>
            <p>
              전체 {counts.all} · ON {counts.enabled} · OFF {counts.paused} · 실패 {counts.failed}
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

      {triggerNotice && (
        <div className={`autopilot-notice ${triggerNotice.tone}`} role="status">
          <span>{triggerNotice.message}</span>
          {triggerNotice.issueHref && (
            <Link className="button micro secondary" to={triggerNotice.issueHref}>
              생성 이슈 보기
            </Link>
          )}
          <button className="button micro secondary" type="button" onClick={() => setTriggerNotice(null)}>
            닫기
          </button>
        </div>
      )}

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
          {visibleRules.map((rule) => {
            const triggerBusy = triggerRule.isPending && triggerRule.variables?.id === rule.id;
            const snoozed = isRuleSnoozed(rule);
            return (
              <div className="data-row" key={rule.id}>
                <span>
                  <strong>{rule.name}</strong>
                  <small>{rule.issue_title_template}</small>
                  {rule.last_error && (
                    <small className="autopilot-error" title={rule.last_error}>
                      마지막 오류: {rule.last_error}
                    </small>
                  )}
                </span>
                <span>{rule.cron_expr}</span>
                <span>{rule.assignee_agent_name ? `@${rule.assignee_agent_name}` : '메인 에이전트'}</span>
                <span>
                  {snoozed ? <small className="badge warning">{formatSnooze(rule.snooze_until)}</small> : null}
                  <span>{formatDateTime(rule.next_run_at)}</span>
                </span>
                <span>{formatDateTime(rule.last_run_at)}</span>
                <span className="row-actions">
                  <span className={snoozed ? 'badge warning' : rule.enabled ? 'badge' : 'badge muted'}>{snoozed ? '일시정지' : rule.enabled ? 'ON' : 'OFF'}</span>
                  {(rule.consecutive_failures ?? 0) > 0 && <span className="badge danger">실패 {rule.consecutive_failures}</span>}
                  <button className="button micro secondary" type="button" onClick={() => openEdit(rule)}>
                    편집
                  </button>
                  <button className="button micro secondary" type="button" onClick={() => toggleRule.mutate(rule)}>
                    {rule.enabled ? '끄기' : '켜기'}
                  </button>
                  <button className="button micro secondary" type="button" onClick={() => triggerRule.mutate(rule)} disabled={triggerBusy || snoozed}>
                    {snoozed ? '정지 중' : triggerBusy ? '실행 중' : '지금 실행'}
                  </button>
                  {rule.last_triggered_issue_id && (
                    <Link className="button micro secondary" to={`/w/${slug}/issues/${rule.last_triggered_issue_id}`}>
                      마지막 이슈
                    </Link>
                  )}
                  <button className="button micro danger" type="button" onClick={() => deleteRule.mutate(rule.id)}>
                    삭제
                  </button>
                </span>
              </div>
            );
          })}
        </div>
        {!rules.isLoading && !visibleRules.length && Boolean(rules.data?.length) && <p>조건에 맞는 규칙이 없습니다.</p>}
      </article>

      <AutopilotDialog open={dialogOpen} slug={slug} template={selectedTemplate} rule={editingRule} onClose={closeDialog} />
    </section>
  );
}

function isRuleSnoozed(rule: AutopilotRule) {
  if (!rule.snooze_until) {
    return false;
  }
  const date = new Date(rule.snooze_until);
  return !Number.isNaN(date.getTime()) && date.getTime() > Date.now();
}

function formatSnooze(value?: string) {
  if (!value) {
    return '일시정지';
  }
  return `정지 해제 ${formatDateTime(value)}`;
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

function apiErrorMessage(error: unknown) {
  if (error instanceof ApiError) {
    return error.body?.error?.message ?? error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return '알 수 없는 오류';
}
