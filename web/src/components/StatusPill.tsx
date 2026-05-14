export type StatusPillKind = 'issue' | 'execution' | 'run' | 'rule' | 'runtime';

export type StatusPillProps = {
  kind: StatusPillKind;
  status: string;
  pulse?: boolean;
};

const STATUS_LABELS: Record<StatusPillKind, Record<string, string>> = {
  issue: {
    open: '열림',
    done: '완료',
    cancelled: '취소'
  },
  execution: {
    idle: '대기 없음',
    queued: '대기',
    running: '실행 중',
    done: '완료',
    failed: '실패',
    cancelled: '취소'
  },
  run: {
    queued: '대기',
    running: '실행 중',
    done: '완료',
    failed: '실패',
    cancelled: '취소'
  },
  rule: {
    enabled: '활성',
    disabled: '비활성',
    active: '활성',
    inactive: '비활성',
    on: '켜짐',
    off: '꺼짐'
  },
  runtime: {
    available: '사용 가능',
    unavailable: '사용 불가',
    ready: '준비됨',
    ok: '정상',
    error: '오류',
    missing: '없음',
    unknown: '알 수 없음'
  }
};

export function StatusPill({ kind, status, pulse = false }: StatusPillProps) {
  const normalizedStatus = normalizeStatus(status);
  const safeStatus = safeClassSegment(normalizedStatus || 'unknown');
  const label = getStatusPillLabel(kind, status);
  const className = [
    'status-pill',
    `status-pill--${kind}`,
    `status-pill--${safeStatus}`,
    `status-${safeStatus}`,
    pulse ? 'status-pill--pulse' : ''
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <span className={className} data-kind={kind} data-status={normalizedStatus || 'unknown'} title={label}>
      {label}
    </span>
  );
}

export function getStatusPillLabel(kind: StatusPillKind, status: string) {
  const normalizedStatus = normalizeStatus(status);
  return STATUS_LABELS[kind][normalizedStatus] ?? fallbackLabel(status);
}

function normalizeStatus(status: string) {
  return status.trim().toLowerCase().replace(/[\s_]+/g, '-');
}

function safeClassSegment(value: string) {
  return value.replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '') || 'unknown';
}

function fallbackLabel(status: string) {
  const trimmedStatus = status.trim();
  if (!trimmedStatus) {
    return '알 수 없음';
  }
  return trimmedStatus.replace(/[\s_-]+/g, ' ');
}
