import { FormEvent, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import {
  type Webhook,
  type WebhookDelivery,
  type WorkspaceSummary,
  useWebhookDeliveriesQuery,
  useWorkspaceWebhooksQuery
} from '../api/queries';

const ALL_EVENTS = ['run.completed', 'run.failed', 'issue.done', 'issue.cancelled'];

type WorkspaceWebhookSectionProps = {
  workspace: WorkspaceSummary;
};

export function WorkspaceWebhookSection({ workspace }: WorkspaceWebhookSectionProps) {
  const queryClient = useQueryClient();
  const webhooks = useWorkspaceWebhooksQuery(workspace.slug);
  const [url, setUrl] = useState('');
  const [secret, setSecret] = useState('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [maskPII, setMaskPII] = useState(false);
  const [error, setError] = useState('');

  const create = useMutation({
    mutationFn: () =>
      apiClient.post(`/workspaces/${workspace.slug}/webhooks`, {
        url,
        secret,
        events: Array.from(selected),
        mask_pii: maskPII
      }),
    onSuccess: () => {
      setUrl('');
      setSecret('');
      setSelected(new Set());
      setMaskPII(false);
      setError('');
      queryClient.invalidateQueries({ queryKey: ['webhooks', workspace.slug] });
    },
    onError: (err: unknown) => {
      setError(err instanceof Error ? err.message : '알 수 없는 오류');
    }
  });

  const remove = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/webhooks/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks', workspace.slug] });
    }
  });

  const toggleEvent = (event: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(event)) {
        next.delete(event);
      } else {
        next.add(event);
      }
      return next;
    });
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!url.trim()) {
      setError('URL이 필요합니다.');
      return;
    }
    create.mutate();
  };

  return (
    <section className="webhook-section">
      <header className="webhook-section__header">
        <strong>Webhook</strong>
        <span className="muted-copy">run/issue 종료 이벤트를 외부 URL로 전송 (HMAC-SHA256 서명, 1회 retry)</span>
      </header>

      <form className="webhook-section__form" onSubmit={onSubmit}>
        <label className="field-label">
          URL
          <input
            type="url"
            placeholder="https://hooks.example/incoming"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            required
          />
        </label>
        <label className="field-label">
          Secret (옵션, HMAC 키)
          <input
            type="password"
            placeholder="비우면 서명 헤더 없이 전송"
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
        </label>
        <div className="webhook-section__events">
          <span className="field-label-text">이벤트 (비우면 전체)</span>
          {ALL_EVENTS.map((event) => (
            <label key={event} className="webhook-section__event">
              <input
                type="checkbox"
                checked={selected.has(event)}
                onChange={() => toggleEvent(event)}
              />
              <code>{event}</code>
            </label>
          ))}
        </div>
        <label className="webhook-section__event">
          <input
            type="checkbox"
            checked={maskPII}
            onChange={(e) => setMaskPII(e.target.checked)}
          />
          <span>payload에서 이메일/전화 PII 마스킹</span>
        </label>
        <button className="button secondary" type="submit" disabled={create.isPending}>
          {create.isPending ? '추가 중' : 'Webhook 추가'}
        </button>
        {error && <p className="error-text">{error}</p>}
      </form>

      {(webhooks.data ?? []).length === 0 ? (
        <p className="muted-copy">등록된 webhook이 없습니다.</p>
      ) : (
        <ul className="webhook-section__list">
          {(webhooks.data ?? []).map((hook) => (
            <li key={hook.id}>
              <WebhookRow
                hook={hook}
                onDelete={() => {
                  if (window.confirm(`webhook ${hook.url}을(를) 삭제할까요?`)) {
                    remove.mutate(hook.id);
                  }
                }}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function WebhookRow({ hook, onDelete }: { hook: Webhook; onDelete: () => void }) {
  const queryClient = useQueryClient();
  const deliveries = useWebhookDeliveriesQuery(hook.id, 5);
  const redeliver = useMutation({
    mutationFn: (deliveryID: string) =>
      apiClient.post(`/webhooks/${hook.id}/deliveries/${deliveryID}/redeliver`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhook-deliveries', hook.id] });
      queryClient.invalidateQueries({ queryKey: ['webhooks'] });
    }
  });
  const redeliverAllFailed = useMutation({
    mutationFn: () => apiClient.post(`/webhooks/${hook.id}/deliveries/redeliver-failed`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhook-deliveries', hook.id] });
      queryClient.invalidateQueries({ queryKey: ['webhooks'] });
    }
  });
  return (
    <div className="webhook-row">
      <div className="webhook-row__head">
        <div className="webhook-row__url">
          <code>{hook.url}</code>
          {hook.has_secret && <span className="badge">서명 사용</span>}
          {hook.mask_pii && <span className="badge">PII 마스킹</span>}
          {hook.failed_delivery_count > 0 && (
            <span className="badge danger">dead-letter {hook.failed_delivery_count}건</span>
          )}
          {hook.enabled ? null : <span className="badge muted">비활성</span>}
        </div>
        <div className="webhook-row__actions">
          {hook.failed_delivery_count > 0 ? (
            <button
              type="button"
              className="button secondary ghost"
              onClick={() => {
                if (window.confirm(`${hook.failed_delivery_count}건의 dead-letter를 모두 재전송할까요?`)) {
                  redeliverAllFailed.mutate();
                }
              }}
              disabled={redeliverAllFailed.isPending}
            >
              {redeliverAllFailed.isPending ? '재전송 중' : '모두 재전송'}
            </button>
          ) : null}
          <button className="button danger ghost" type="button" onClick={onDelete}>
            삭제
          </button>
        </div>
      </div>
      <div className="webhook-row__events">
        {hook.events.length === 0 ? (
          <code>전체 이벤트</code>
        ) : (
          hook.events.map((e) => <code key={e}>{e}</code>)
        )}
      </div>
      <div className="webhook-row__deliveries">
        <span className="muted-copy">최근 전달</span>
        {(deliveries.data ?? []).length === 0 ? (
          <span className="muted-copy">기록 없음</span>
        ) : (
          <ul>
            {(deliveries.data ?? []).map((d) => (
              <li key={d.id}>
                <details className="webhook-delivery-row">
                  <summary>
                    <span data-status={d.status} className="webhook-delivery-status">
                      {labelForStatus(d)}
                    </span>
                    <code>{d.event_type}</code>
                    <span className="muted-copy">{d.created_at.slice(0, 19).replace('T', ' ')}</span>
                    {d.status === 'failed' ? (
                      <button
                        type="button"
                        className="button secondary ghost webhook-delivery-redeliver"
                        onClick={(e) => {
                          e.preventDefault();
                          redeliver.mutate(d.id);
                        }}
                        disabled={redeliver.isPending}
                      >
                        {redeliver.isPending ? '재전송 중' : '재전송'}
                      </button>
                    ) : null}
                  </summary>
                  <pre className="webhook-delivery-payload">{formatPayload(d.payload_json)}</pre>
                  {d.error_message ? <p className="error-text">{d.error_message}</p> : null}
                </details>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function labelForStatus(d: WebhookDelivery): string {
  if (d.status === 'delivered') return `200 OK (시도 ${d.attempt})`;
  if (d.status === 'failed') return `실패 ${d.status_code || ''} (시도 ${d.attempt})`.trim();
  return `대기 중 (시도 ${d.attempt})`;
}

function formatPayload(raw?: string): string {
  if (!raw) return '(payload 없음)';
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
