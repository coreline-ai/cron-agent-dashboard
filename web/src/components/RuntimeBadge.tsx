import type { SettingsResponse } from '../api/queries';

type RuntimeInfo = SettingsResponse['available_runtimes'][number];

// KNOWN_RUNTIME_ISSUES surfaces operational warnings that the dashboard has
// observed but the runtime CLI itself reports as "available". The strings
// are operator-facing and are mirrored in dev-plan/implement_20260520_230031.md
// (claude --print + stdin pipe hang) and dev-plan/implement_20260521_211716.md
// (hub-PM workflow recommendations).
const KNOWN_RUNTIME_ISSUES: Record<string, string> = {
  claude:
    'claude --print를 비대화형 stdin pipe로 호출할 때 입력 대기로 hang하는 사례가 관측됐습니다. RFP/hub-PM 류 워크플로우는 codex 사용을 권장합니다.'
};

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
