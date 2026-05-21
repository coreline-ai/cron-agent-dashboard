import type { SettingsResponse } from '../api/queries';

type RuntimeInfo = SettingsResponse['available_runtimes'][number];

// KNOWN_RUNTIME_ISSUES surfaces operational warnings that the dashboard has
// observed but the runtime CLI itself reports as "available". When a known
// issue gets resolved by an adapter patch it should be removed here so the
// badge stops nagging operators. Currently empty — see dev-plan/
// implement_20260521_221108.md for the most recent cleanup (claude --print
// + stdin pipe hang resolved by passing --input-format text).
const KNOWN_RUNTIME_ISSUES: Record<string, string> = {};

export type RuntimeBadgeVariant = 'ok' | 'warn-known-issue' | 'warn-not-detected';

export type RuntimeClassification = {
  variant: RuntimeBadgeVariant;
  label: string;
  description: string;
};

export function classifyRuntime(runtime: string, runtimes: RuntimeInfo[] | undefined): RuntimeClassification {
  const trimmed = runtime.trim().toLowerCase();
  const info = runtimes?.find((r) => r.name.trim().toLowerCase() === trimmed);
  const knownIssue = KNOWN_RUNTIME_ISSUES[trimmed];

  if (!info || info.supported === false) {
    const detail = info?.warning?.trim();
    return {
      variant: 'warn-not-detected',
      label: '⚠ 미감지',
      description: detail || `${runtime} CLI가 PATH에서 감지되지 않습니다. 설치 후 서버를 재시작하세요.`
    };
  }
  if (knownIssue) {
    return { variant: 'warn-known-issue', label: '⚠ 운영 주의', description: knownIssue };
  }
  return {
    variant: 'ok',
    label: '✓ 권장',
    description: `${runtime} CLI 감지 — ${info.path}${info.version ? ` · ${info.version}` : ''}`
  };
}

export function RuntimeBadge({ runtime, runtimes }: { runtime: string; runtimes?: RuntimeInfo[] }) {
  const cls = classifyRuntime(runtime, runtimes);
  return (
    <span
      className={`runtime-badge runtime-badge--${cls.variant}`}
      data-runtime={runtime}
      data-variant={cls.variant}
      title={cls.description}
    >
      {cls.label}
    </span>
  );
}
